package alertnotify

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

const (
	weChatAPIURL = "https://qyapi.weixin.qq.com/cgi-bin/"
	// Alertmanager uses TLS from the start on this SMTP port.
	smtpsPort = "465"
)

// SendTest sends a firing alert straight to a normalized channel, without
// Alertmanager, so the channel editor can check settings before they are
// saved. It sends the way Alertmanager and the forwarder do, but from where it
// runs.
func SendTest(ctx context.Context, ch Channel, cluster string, labels, annotations map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	msg := webhookMessage{
		Version:           "4",
		Receiver:          ch.Name,
		Status:            "firing",
		GroupLabels:       labels,
		CommonLabels:      labels,
		CommonAnnotations: annotations,
		Alerts: []webhookAlert{{
			Status:      "firing",
			Labels:      labels,
			Annotations: annotations,
			StartsAt:    time.Now(),
		}},
	}
	f := &forwarder{cluster: cluster, client: &http.Client{Timeout: sendTimeout}}
	text := describe(msg, func(s string) string { return s })
	switch ch.Type {
	case ChannelEmail:
		return sendEmail(ctx, ch.Email, f.title(msg), text)
	case ChannelWeChat:
		return f.sendWeChat(ctx, ch.WeChat, f.title(msg)+"\n\n"+text)
	case ChannelWebhook:
		return f.post(ctx, ch.Webhook.URL, msg, nil)
	case ChannelFeishu:
		return f.sendFeishu(ctx, ch.Feishu, msg)
	case ChannelBark:
		return f.sendBark(ctx, ch.Bark, msg)
	}
	return fmt.Errorf("unknown channel type %q", ch.Type)
}

// sendWeChat sends a text message through a WeCom application the way
// Alertmanager does.
func (f *forwarder) sendWeChat(ctx context.Context, settings *WeChatSettings, content string) error {
	query := url.Values{"corpid": {settings.CorpID}, "corpsecret": {settings.APISecret}}
	var token struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := f.do(ctx, http.MethodGet, weChatAPIURL+"gettoken?"+query.Encode(), nil, &token); err != nil {
		return err
	}
	if token.AccessToken == "" {
		return fmt.Errorf("wechat returned no access token, code %d: %s", token.ErrCode, token.ErrMsg)
	}
	message := struct {
		ToUser  string            `json:"touser,omitempty"`
		ToParty string            `json:"toparty,omitempty"`
		ToTag   string            `json:"totag,omitempty"`
		AgentID string            `json:"agentid"`
		Type    string            `json:"msgtype"`
		Text    map[string]string `json:"text"`
	}{settings.ToUser, settings.ToParty, settings.ToTag, settings.AgentID, "text", map[string]string{"content": content}}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := f.post(ctx, weChatAPIURL+"message/send?access_token="+url.QueryEscape(token.AccessToken), message, &result); err != nil {
		return err
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wechat returned code %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// sendEmail sends a plain text email the way Alertmanager does: TLS from the
// start on port 465, STARTTLS only when required, and authentication when a
// username is set and the server asks for it.
func sendEmail(ctx context.Context, settings *EmailSettings, subject, text string) error {
	from, err := mail.ParseAddress(settings.From)
	if err != nil {
		return err
	}
	to, err := mail.ParseAddressList(settings.To)
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(settings.Smarthost)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if port == smtpsPort {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", settings.Smarthost)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", settings.Smarthost)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to the SMTP server: %w", err)
	}
	// The SMTP client takes no context, so the deadline bounds the session.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to start the SMTP session: %w", err)
	}
	defer func() { _ = client.Close() }()
	if settings.RequireTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("STARTTLS is on but %s does not offer it", settings.Smarthost)
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}
	if ok, mechs := client.Extension("AUTH"); ok && settings.Username != "" {
		auth, err := emailAuth(settings, host, mechs)
		if err != nil {
			return err
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("the SMTP server refused the sender: %w", err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr.Address); err != nil {
			return fmt.Errorf("the SMTP server refused the recipient %s: %w", addr.Address, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("the SMTP server refused the message: %w", err)
	}
	if err := writeEmail(w, from, to, host, subject, text); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("the SMTP server refused the message: %w", err)
	}
	return client.Quit()
}

func emailAuth(settings *EmailSettings, host, mechs string) (smtp.Auth, error) {
	for _, mech := range strings.Fields(mechs) {
		switch mech {
		case "PLAIN":
			return smtp.PlainAuth("", settings.Username, settings.Password, host), nil
		case "LOGIN":
			return &loginAuth{username: settings.Username, password: settings.Password}, nil
		}
	}
	return nil, fmt.Errorf("the SMTP server offers no supported authentication method: %s", mechs)
}

func writeEmail(w io.Writer, from *mail.Address, to []*mail.Address, host, subject, text string) error {
	recipients := make([]string, len(to))
	for i, addr := range to {
		recipients[i] = addr.String()
	}
	now := time.Now()
	_, err := fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-Id: <%d@%s>\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n",
		from, strings.Join(recipients, ", "), mime.QEncoding.Encode("utf-8", subject), now.Format(time.RFC1123Z), now.UnixNano(), host)
	if err != nil {
		return err
	}
	body := quotedprintable.NewWriter(w)
	if _, err := io.WriteString(body, text); err != nil {
		return err
	}
	return body.Close()
}

// loginAuth is the LOGIN mechanism, which Alertmanager supports and net/smtp
// lacks.
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(string(fromServer)) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected SMTP challenge %q", fromServer)
}
