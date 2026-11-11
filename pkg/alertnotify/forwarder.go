package alertnotify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	maxPayloadBytes  = 1 << 20
	maxResponseBytes = 4096
	maxListedAlerts  = 10
	sendTimeout      = 15 * time.Second
	shutdownTimeout  = 5 * time.Second
	timeLayout       = "2006-01-02 15:04:05 MST"
)

// webhookMessage is what Alertmanager posts to a webhook receiver.
type webhookMessage struct {
	Status            string            `json:"status"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	Alerts            []webhookAlert    `json:"alerts"`
}

type webhookAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
}

type forwarder struct {
	configPath string
	cluster    string
	client     *http.Client
}

// RunForwarder serves the webhook Alertmanager sends Feishu and Bark channels
// to. Each request reads the channel from the mounted configuration, so
// changes apply without a restart.
func RunForwarder(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("mplatform alert-forwarder", flag.ContinueOnError)
	klog.InitFlags(flags)
	listen := flags.String("listen", ":8080", "Address to listen on")
	configPath := flags.String("config", "", "Path to the notification configuration")
	cluster := flags.String("cluster", "", "Cluster name shown in notifications")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	f := &forwarder{configPath: *configPath, cluster: *cluster, client: &http.Client{Timeout: sendTimeout}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /channels/{id}", f.handle)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	klog.Infof("Alert forwarder listening on %s", *listen)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (f *forwarder) handle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var msg webhookMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, maxPayloadBytes)).Decode(&msg); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	ch, err := f.channel(id)
	if err != nil {
		klog.Errorf("Failed to load channel %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch ch.Type {
	case ChannelFeishu:
		err = f.sendFeishu(r.Context(), ch.Feishu, msg)
	case ChannelBark:
		err = f.sendBark(r.Context(), ch.Bark, msg)
	default:
		err = fmt.Errorf("channel type %s is not forwarded", ch.Type)
	}
	if err != nil {
		klog.Errorf("Failed to send to channel %s (%s): %v", ch.Name, ch.Type, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *forwarder) channel(id string) (Channel, error) {
	data, err := os.ReadFile(f.configPath)
	if err != nil {
		return Channel{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Channel{}, err
	}
	ch, ok := cfg.Channel(id)
	if !ok {
		return Channel{}, fmt.Errorf("channel %s not found", id)
	}
	if (ch.Type == ChannelFeishu && ch.Feishu == nil) || (ch.Type == ChannelBark && ch.Bark == nil) {
		return Channel{}, fmt.Errorf("channel %s has no settings", id)
	}
	return ch, nil
}

func (f *forwarder) title(msg webhookMessage) string {
	name := msg.GroupLabels["alertname"]
	if name == "" {
		name = msg.CommonLabels["alertname"]
	}
	firing := 0
	for _, alert := range msg.Alerts {
		if alert.Status == "firing" {
			firing++
		}
	}
	title := "[RESOLVED]"
	if msg.Status == "firing" {
		title = fmt.Sprintf("[FIRING:%d]", firing)
	}
	if name != "" {
		title += " " + name
	}
	if f.cluster != "" {
		title = "[" + f.cluster + "] " + title
	}
	return title
}

// describe lists the alerts of a message, one paragraph each.
func describe(msg webhookMessage, bold func(string) string) string {
	var b strings.Builder
	for i, alert := range msg.Alerts {
		if i == maxListedAlerts {
			fmt.Fprintf(&b, "… %d more\n", len(msg.Alerts)-i)
			break
		}
		heading := []string{bold(alert.Labels["alertname"])}
		for _, label := range []string{"severity", "namespace", "pod"} {
			if value := alert.Labels[label]; value != "" {
				heading = append(heading, value)
			}
		}
		b.WriteString(strings.Join(heading, " · ") + "\n")
		for _, annotation := range []string{"summary", "description", "message"} {
			if text := alert.Annotations[annotation]; text != "" {
				b.WriteString(text + "\n")
				break
			}
		}
		if alert.Status == "resolved" {
			fmt.Fprintf(&b, "Resolved at %s\n", alert.EndsAt.Format(timeLayout))
		} else {
			fmt.Fprintf(&b, "Started at %s\n", alert.StartsAt.Format(timeLayout))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func cardColor(msg webhookMessage) string {
	if msg.Status != "firing" {
		return "green"
	}
	switch msg.CommonLabels["severity"] {
	case "critical":
		return "red"
	case "warning":
		return "orange"
	}
	return "blue"
}

// sendFeishu posts a card to a Feishu custom bot, signed when the bot checks
// signatures: HMAC-SHA256 keyed by "timestamp\nsecret" over no data.
func (f *forwarder) sendFeishu(ctx context.Context, settings *FeishuSettings, msg webhookMessage) error {
	body := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]string{"tag": "plain_text", "content": f.title(msg)},
				"template": cardColor(msg),
			},
			"elements": []any{map[string]string{
				"tag":     "markdown",
				"content": describe(msg, func(s string) string { return "**" + s + "**" }),
			}},
		},
	}
	if settings.Secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(timestamp+"\n"+settings.Secret))
		body["timestamp"] = timestamp
		body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := f.post(ctx, settings.URL, body, &result); err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu returned code %d: %s", result.Code, result.Msg)
	}
	return nil
}

func (f *forwarder) sendBark(ctx context.Context, settings *BarkSettings, msg webhookMessage) error {
	body := map[string]string{
		"device_key": settings.Key,
		"title":      f.title(msg),
		"body":       describe(msg, func(s string) string { return s }),
		"group":      "MPlatform",
	}
	var result struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err := f.post(ctx, settings.Server+"/push", body, &result); err != nil {
		return err
	}
	if result.Code != nil && *result.Code != http.StatusOK {
		return fmt.Errorf("bark returned code %d: %s", *result.Code, result.Message)
	}
	return nil
}

// post sends JSON and decodes a JSON reply when there is one. Errors leave out
// the target URL, which holds the bot token or device key.
func (f *forwarder) post(ctx context.Context, target string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid target URL")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := f.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("target returned %s: %s", resp.Status, strings.TrimSpace(string(reply)))
	}
	_ = json.Unmarshal(reply, out)
	return nil
}
