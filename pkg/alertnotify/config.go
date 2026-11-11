// Package alertnotify holds the notification settings MPlatform keeps for a
// cluster's Alertmanager: the channels alerts are sent to and how alerts are
// routed to them. It renders them as an Alertmanager configuration and runs
// the forwarder that sends to the channels Alertmanager has no notifier for.
package alertnotify

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	prommodel "github.com/prometheus/common/model"
)

// Channel types. Alertmanager sends to email, WeChat and webhook channels
// itself, and to Feishu and Bark through the forwarder.
const (
	ChannelEmail   = "email"
	ChannelWeChat  = "wechat"
	ChannelWebhook = "webhook"
	ChannelFeishu  = "feishu"
	ChannelBark    = "bark"
)

const (
	defaultBarkServer = "https://api.day.app"
	maxChannelName    = 64
	// nullReceiver drops the alerts routed to it.
	nullReceiver = "null"
)

var (
	labelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	channelIDPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)
	matchOperators   = []string{"=", "!=", "=~", "!~"}
)

// Config is what the platform stores for a cluster. Routes refer to channels
// by ID, so renaming a channel keeps its routes.
type Config struct {
	Channels []Channel `json:"channels"`
	Routing  Routing   `json:"routing"`
}

type Channel struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Type         string           `json:"type"`
	SendResolved bool             `json:"sendResolved"`
	Email        *EmailSettings   `json:"email,omitempty"`
	WeChat       *WeChatSettings  `json:"wechat,omitempty"`
	Webhook      *WebhookSettings `json:"webhook,omitempty"`
	Feishu       *FeishuSettings  `json:"feishu,omitempty"`
	Bark         *BarkSettings    `json:"bark,omitempty"`
}

type EmailSettings struct {
	// To is one or more addresses separated by commas.
	To         string `json:"to"`
	From       string `json:"from"`
	Smarthost  string `json:"smarthost"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RequireTLS bool   `json:"requireTLS"`
}

// WeChatSettings sends through a WeCom application.
type WeChatSettings struct {
	CorpID    string `json:"corpID"`
	AgentID   string `json:"agentID"`
	APISecret string `json:"apiSecret"`
	ToUser    string `json:"toUser"`
	ToParty   string `json:"toParty"`
	ToTag     string `json:"toTag"`
}

type WebhookSettings struct {
	URL string `json:"url"`
}

// FeishuSettings is a Feishu group's custom bot; Secret is its signing key
// when signature verification is on.
type FeishuSettings struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type BarkSettings struct {
	Server string `json:"server"`
	Key    string `json:"key"`
}

// Routing sends every alert to Receiver unless one of the routes matches it.
type Routing struct {
	// Receiver is a channel ID; empty drops unmatched alerts.
	Receiver       string   `json:"receiver"`
	GroupBy        []string `json:"groupBy"`
	GroupWait      string   `json:"groupWait"`
	GroupInterval  string   `json:"groupInterval"`
	RepeatInterval string   `json:"repeatInterval"`
	Routes         []Route  `json:"routes"`
}

// Route matches alerts by all of its conditions. A matched alert stops at the
// route unless Continue is set.
type Route struct {
	Receivers      []string  `json:"receivers"`
	Severities     []string  `json:"severities"`
	Namespaces     []string  `json:"namespaces"`
	Matchers       []Matcher `json:"matchers"`
	Continue       bool      `json:"continue"`
	GroupInterval  string    `json:"groupInterval"`
	RepeatInterval string    `json:"repeatInterval"`
}

type Matcher struct {
	Name     string `json:"name"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// DefaultConfig has no channels and the timing kube-prometheus-stack ships.
func DefaultConfig() Config {
	return Config{
		Channels: []Channel{},
		Routing: Routing{
			GroupBy:        []string{"namespace", "alertname"},
			GroupWait:      "30s",
			GroupInterval:  "5m",
			RepeatInterval: "12h",
			Routes:         []Route{},
		},
	}
}

// NeedsForwarder reports whether a channel is sent through the forwarder.
func (c Config) NeedsForwarder() bool {
	return slices.ContainsFunc(c.Channels, func(ch Channel) bool {
		return ch.Type == ChannelFeishu || ch.Type == ChannelBark
	})
}

func (c Config) Channel(id string) (Channel, bool) {
	for _, ch := range c.Channels {
		if ch.ID == id {
			return ch, true
		}
	}
	return Channel{}, false
}

func (ch Channel) clone() Channel {
	out := ch
	if ch.Email != nil {
		email := *ch.Email
		out.Email = &email
	}
	if ch.WeChat != nil {
		wechat := *ch.WeChat
		out.WeChat = &wechat
	}
	if ch.Webhook != nil {
		webhook := *ch.Webhook
		out.Webhook = &webhook
	}
	if ch.Feishu != nil {
		feishu := *ch.Feishu
		out.Feishu = &feishu
	}
	if ch.Bark != nil {
		bark := *ch.Bark
		out.Bark = &bark
	}
	return out
}

// secrets returns the sensitive settings of a channel by their JSON names.
// They are never sent back to the browser.
func (ch *Channel) secrets() map[string]*string {
	switch {
	case ch.Type == ChannelEmail && ch.Email != nil:
		return map[string]*string{"password": &ch.Email.Password}
	case ch.Type == ChannelWeChat && ch.WeChat != nil:
		return map[string]*string{"apiSecret": &ch.WeChat.APISecret}
	case ch.Type == ChannelWebhook && ch.Webhook != nil:
		return map[string]*string{"url": &ch.Webhook.URL}
	case ch.Type == ChannelFeishu && ch.Feishu != nil:
		return map[string]*string{"url": &ch.Feishu.URL, "secret": &ch.Feishu.Secret}
	case ch.Type == ChannelBark && ch.Bark != nil:
		return map[string]*string{"key": &ch.Bark.Key}
	}
	return nil
}

// MaskedChannel is a channel without its sensitive settings, and the names of
// the ones that are set.
type MaskedChannel struct {
	Channel
	ConfiguredSecrets []string `json:"configuredSecrets"`
}

func (c Config) MaskedChannels() []MaskedChannel {
	masked := make([]MaskedChannel, 0, len(c.Channels))
	for _, ch := range c.Channels {
		out := ch.clone()
		configured := []string{}
		for name, value := range out.secrets() {
			if *value != "" {
				configured = append(configured, name)
			}
			*value = ""
		}
		slices.Sort(configured)
		masked = append(masked, MaskedChannel{Channel: out, ConfiguredSecrets: configured})
	}
	return masked
}

// WithStoredSecrets fills the sensitive settings a request leaves empty from
// the stored channel with the same ID and type, so saving a channel keeps
// what the browser never received.
func (c Config) WithStoredSecrets(stored Config) Config {
	out := c
	out.Channels = make([]Channel, 0, len(c.Channels))
	for _, ch := range c.Channels {
		merged := ch.clone()
		if previous, ok := stored.Channel(ch.ID); ok && previous.Type == ch.Type {
			previousSecrets := previous.secrets()
			for name, value := range merged.secrets() {
				if *value == "" && previousSecrets[name] != nil {
					*value = *previousSecrets[name]
				}
			}
		}
		out.Channels = append(out.Channels, merged)
	}
	return out
}

// Normalize trims the configuration, gives new channels an ID, fills defaults
// and validates it.
func (c Config) Normalize() (Config, error) {
	out := Config{Channels: make([]Channel, 0, len(c.Channels))}
	names := map[string]bool{}
	ids := map[string]bool{}
	for i, ch := range c.Channels {
		normalized, err := normalizeChannel(ch)
		if err != nil {
			return Config{}, fmt.Errorf("channel %d: %w", i+1, err)
		}
		if names[normalized.Name] {
			return Config{}, fmt.Errorf("channel name %q is used more than once", normalized.Name)
		}
		if ids[normalized.ID] {
			return Config{}, fmt.Errorf("channel ID %q is used more than once", normalized.ID)
		}
		names[normalized.Name] = true
		ids[normalized.ID] = true
		out.Channels = append(out.Channels, normalized)
	}
	routing, err := normalizeRouting(c.Routing, ids)
	if err != nil {
		return Config{}, err
	}
	out.Routing = routing
	return out, nil
}

func normalizeChannel(ch Channel) (Channel, error) {
	out := Channel{
		ID:           strings.TrimSpace(ch.ID),
		Name:         strings.TrimSpace(ch.Name),
		Type:         ch.Type,
		SendResolved: ch.SendResolved,
	}
	if out.ID == "" {
		id := make([]byte, 6)
		if _, err := rand.Read(id); err != nil {
			return Channel{}, err
		}
		out.ID = hex.EncodeToString(id)
	} else if !channelIDPattern.MatchString(out.ID) {
		return Channel{}, fmt.Errorf("invalid channel ID %q", out.ID)
	}
	if out.Name == "" || len(out.Name) > maxChannelName || hasControlChars(out.Name) {
		return Channel{}, fmt.Errorf("name is required and must be at most %d characters", maxChannelName)
	}
	if out.Name == nullReceiver {
		return Channel{}, fmt.Errorf("name %q is reserved", nullReceiver)
	}
	var err error
	switch ch.Type {
	case ChannelEmail:
		out.Email, err = normalizeEmail(ch.Email)
	case ChannelWeChat:
		out.WeChat, err = normalizeWeChat(ch.WeChat)
	case ChannelWebhook:
		out.Webhook, err = normalizeWebhook(ch.Webhook)
	case ChannelFeishu:
		out.Feishu, err = normalizeFeishu(ch.Feishu)
	case ChannelBark:
		out.Bark, err = normalizeBark(ch.Bark)
	default:
		return Channel{}, fmt.Errorf("unknown channel type %q", ch.Type)
	}
	if err != nil {
		return Channel{}, fmt.Errorf("%s: %w", out.Name, err)
	}
	return out, nil
}

func hasControlChars(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func normalizeEmail(in *EmailSettings) (*EmailSettings, error) {
	if in == nil {
		return nil, errors.New("email settings are required")
	}
	out := &EmailSettings{
		To:         strings.TrimSpace(in.To),
		From:       strings.TrimSpace(in.From),
		Smarthost:  strings.TrimSpace(in.Smarthost),
		Username:   strings.TrimSpace(in.Username),
		Password:   in.Password,
		RequireTLS: in.RequireTLS,
	}
	if _, err := mail.ParseAddressList(out.To); err != nil {
		return nil, fmt.Errorf("invalid recipients: %w", err)
	}
	if _, err := mail.ParseAddress(out.From); err != nil {
		return nil, fmt.Errorf("invalid sender: %w", err)
	}
	host, port, err := net.SplitHostPort(out.Smarthost)
	if err == nil && host != "" {
		_, err = strconv.ParseUint(port, 10, 16)
	}
	if err != nil || host == "" {
		return nil, errors.New("SMTP server must look like smtp.example.com:465")
	}
	return out, nil
}

func normalizeWeChat(in *WeChatSettings) (*WeChatSettings, error) {
	if in == nil {
		return nil, errors.New("WeChat settings are required")
	}
	out := &WeChatSettings{
		CorpID:    strings.TrimSpace(in.CorpID),
		AgentID:   strings.TrimSpace(in.AgentID),
		APISecret: strings.TrimSpace(in.APISecret),
		ToUser:    strings.TrimSpace(in.ToUser),
		ToParty:   strings.TrimSpace(in.ToParty),
		ToTag:     strings.TrimSpace(in.ToTag),
	}
	if out.CorpID == "" || out.AgentID == "" || out.APISecret == "" {
		return nil, errors.New("corp ID, agent ID and secret are required")
	}
	if out.ToUser == "" && out.ToParty == "" && out.ToTag == "" {
		out.ToUser = "@all"
	}
	return out, nil
}

func normalizeWebhook(in *WebhookSettings) (*WebhookSettings, error) {
	if in == nil {
		return nil, errors.New("webhook settings are required")
	}
	out := &WebhookSettings{URL: strings.TrimSpace(in.URL)}
	if err := checkURL(out.URL); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeFeishu(in *FeishuSettings) (*FeishuSettings, error) {
	if in == nil {
		return nil, errors.New("feishu settings are required")
	}
	out := &FeishuSettings{URL: strings.TrimSpace(in.URL), Secret: strings.TrimSpace(in.Secret)}
	if err := checkURL(out.URL); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeBark(in *BarkSettings) (*BarkSettings, error) {
	if in == nil {
		return nil, errors.New("bark settings are required")
	}
	out := &BarkSettings{Server: strings.TrimRight(strings.TrimSpace(in.Server), "/"), Key: strings.TrimSpace(in.Key)}
	if out.Server == "" {
		out.Server = defaultBarkServer
	}
	if err := checkURL(out.Server); err != nil {
		return nil, err
	}
	if out.Key == "" {
		return nil, errors.New("device key is required")
	}
	return out, nil
}

func checkURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("a valid http or https URL is required")
	}
	return nil
}

func normalizeRouting(in Routing, channelIDs map[string]bool) (Routing, error) {
	defaults := DefaultConfig().Routing
	out := Routing{
		Receiver:       strings.TrimSpace(in.Receiver),
		GroupBy:        []string{},
		GroupWait:      orDefault(in.GroupWait, defaults.GroupWait),
		GroupInterval:  orDefault(in.GroupInterval, defaults.GroupInterval),
		RepeatInterval: orDefault(in.RepeatInterval, defaults.RepeatInterval),
		Routes:         make([]Route, 0, len(in.Routes)),
	}
	if out.Receiver != "" && !channelIDs[out.Receiver] {
		return Routing{}, errors.New("the default receiver is not a channel")
	}
	for _, label := range in.GroupBy {
		label = strings.TrimSpace(label)
		if label == "" || slices.Contains(out.GroupBy, label) {
			continue
		}
		if label != "..." && !labelNamePattern.MatchString(label) {
			return Routing{}, fmt.Errorf("invalid group by label %q", label)
		}
		out.GroupBy = append(out.GroupBy, label)
	}
	if len(out.GroupBy) == 0 {
		out.GroupBy = defaults.GroupBy
	}
	if len(out.GroupBy) > 1 && slices.Contains(out.GroupBy, "...") {
		return Routing{}, errors.New(`group by "..." groups by every label and cannot be combined with other labels`)
	}
	if err := checkDuration("group wait", out.GroupWait, true); err != nil {
		return Routing{}, err
	}
	if err := checkDuration("group interval", out.GroupInterval, false); err != nil {
		return Routing{}, err
	}
	if err := checkDuration("repeat interval", out.RepeatInterval, false); err != nil {
		return Routing{}, err
	}
	for i, route := range in.Routes {
		normalized, err := normalizeRoute(route, channelIDs)
		if err != nil {
			return Routing{}, fmt.Errorf("route %d: %w", i+1, err)
		}
		out.Routes = append(out.Routes, normalized)
	}
	return out, nil
}

func orDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func checkDuration(name, value string, allowZero bool) error {
	duration, err := prommodel.ParseDuration(value)
	if err != nil || (!allowZero && duration == 0) {
		return fmt.Errorf("%s must look like 30s, 5m or 12h", name)
	}
	return nil
}

func normalizeRoute(in Route, channelIDs map[string]bool) (Route, error) {
	out := Route{
		Receivers:      []string{},
		Continue:       in.Continue,
		Matchers:       make([]Matcher, 0, len(in.Matchers)),
		GroupInterval:  strings.TrimSpace(in.GroupInterval),
		RepeatInterval: strings.TrimSpace(in.RepeatInterval),
	}
	for _, id := range in.Receivers {
		if !channelIDs[id] {
			return Route{}, errors.New("a receiver is not a channel")
		}
		if !slices.Contains(out.Receivers, id) {
			out.Receivers = append(out.Receivers, id)
		}
	}
	if len(out.Receivers) == 0 {
		return Route{}, errors.New("at least one receiver is required")
	}
	var err error
	if out.Severities, err = labelValues("severity", in.Severities); err != nil {
		return Route{}, err
	}
	if out.Namespaces, err = labelValues("namespace", in.Namespaces); err != nil {
		return Route{}, err
	}
	for _, matcher := range in.Matchers {
		normalized := Matcher{
			Name:     strings.TrimSpace(matcher.Name),
			Operator: matcher.Operator,
			Value:    strings.TrimSpace(matcher.Value),
		}
		if !labelNamePattern.MatchString(normalized.Name) {
			return Route{}, fmt.Errorf("invalid label name %q", normalized.Name)
		}
		if !slices.Contains(matchOperators, normalized.Operator) {
			return Route{}, fmt.Errorf("invalid operator %q", normalized.Operator)
		}
		if hasControlChars(normalized.Value) {
			return Route{}, fmt.Errorf("invalid value for label %s", normalized.Name)
		}
		if strings.Contains(normalized.Operator, "~") {
			if _, err := regexp.Compile("^(?:" + normalized.Value + ")$"); err != nil {
				return Route{}, fmt.Errorf("invalid regular expression for label %s: %w", normalized.Name, err)
			}
		}
		out.Matchers = append(out.Matchers, normalized)
	}
	if out.GroupInterval != "" {
		if err := checkDuration("group interval", out.GroupInterval, false); err != nil {
			return Route{}, err
		}
	}
	if out.RepeatInterval != "" {
		if err := checkDuration("repeat interval", out.RepeatInterval, false); err != nil {
			return Route{}, err
		}
	}
	return out, nil
}

func labelValues(label string, values []string) ([]string, error) {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		if hasControlChars(value) {
			return nil, fmt.Errorf("invalid %s %q", label, value)
		}
		out = append(out, value)
	}
	return out, nil
}
