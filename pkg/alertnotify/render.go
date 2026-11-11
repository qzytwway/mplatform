package alertnotify

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// TestChannelLabel routes a test alert straight to the channel whose ID
	// it holds.
	TestChannelLabel = "mplatform_test_channel"
	// OverrideLabel marks the alerts of rules the platform writes, so a muted
	// built-in alert still comes through from the platform's copy of it.
	OverrideLabel = "mplatform_override"
)

type amConfig struct {
	Global       map[string]string `yaml:"global"`
	Route        amRoute           `yaml:"route"`
	InhibitRules []amInhibitRule   `yaml:"inhibit_rules"`
	Receivers    []amReceiver      `yaml:"receivers"`
}

type amRoute struct {
	Receiver       string    `yaml:"receiver"`
	GroupBy        []string  `yaml:"group_by,omitempty"`
	Matchers       []string  `yaml:"matchers,omitempty"`
	Continue       bool      `yaml:"continue,omitempty"`
	GroupWait      string    `yaml:"group_wait,omitempty"`
	GroupInterval  string    `yaml:"group_interval,omitempty"`
	RepeatInterval string    `yaml:"repeat_interval,omitempty"`
	Routes         []amRoute `yaml:"routes,omitempty"`
}

type amInhibitRule struct {
	SourceMatchers []string `yaml:"source_matchers,omitempty"`
	TargetMatchers []string `yaml:"target_matchers"`
	Equal          []string `yaml:"equal,omitempty"`
}

type amReceiver struct {
	Name           string            `yaml:"name"`
	EmailConfigs   []amEmailConfig   `yaml:"email_configs,omitempty"`
	WeChatConfigs  []amWeChatConfig  `yaml:"wechat_configs,omitempty"`
	WebhookConfigs []amWebhookConfig `yaml:"webhook_configs,omitempty"`
}

type amEmailConfig struct {
	SendResolved bool   `yaml:"send_resolved"`
	To           string `yaml:"to"`
	From         string `yaml:"from"`
	Smarthost    string `yaml:"smarthost"`
	AuthUsername string `yaml:"auth_username,omitempty"`
	AuthPassword string `yaml:"auth_password,omitempty"`
	RequireTLS   bool   `yaml:"require_tls"`
}

type amWeChatConfig struct {
	SendResolved bool   `yaml:"send_resolved"`
	CorpID       string `yaml:"corp_id"`
	AgentID      string `yaml:"agent_id"`
	APISecret    string `yaml:"api_secret"`
	ToUser       string `yaml:"to_user,omitempty"`
	ToParty      string `yaml:"to_party,omitempty"`
	ToTag        string `yaml:"to_tag,omitempty"`
}

type amWebhookConfig struct {
	SendResolved bool   `yaml:"send_resolved"`
	URL          string `yaml:"url"`
}

// The inhibit rules kube-prometheus-stack ships: a critical alert mutes the
// warning and info ones of the same alert, and so on.
var defaultInhibitRules = []amInhibitRule{
	{SourceMatchers: []string{`severity="critical"`}, TargetMatchers: []string{`severity=~"warning|info"`}, Equal: []string{"namespace", "alertname"}},
	{SourceMatchers: []string{`severity="warning"`}, TargetMatchers: []string{`severity="info"`}, Equal: []string{"namespace", "alertname"}},
	{SourceMatchers: []string{`alertname="InfoInhibitor"`}, TargetMatchers: []string{`severity="info"`}, Equal: []string{"namespace"}},
	{TargetMatchers: []string{`alertname="InfoInhibitor"`}},
}

// Render builds the Alertmanager configuration of a normalized Config.
// forwarderURL is where Alertmanager reaches the forwarder.
func Render(cfg Config, forwarderURL string) ([]byte, error) {
	names := map[string]string{}
	for _, ch := range cfg.Channels {
		names[ch.ID] = ch.Name
	}
	root := amRoute{
		Receiver:       nullReceiver,
		GroupBy:        cfg.Routing.GroupBy,
		GroupWait:      cfg.Routing.GroupWait,
		GroupInterval:  cfg.Routing.GroupInterval,
		RepeatInterval: cfg.Routing.RepeatInterval,
	}
	if name, ok := names[cfg.Routing.Receiver]; ok {
		root.Receiver = name
	}
	// A test alert is its own group and goes out at once.
	for _, ch := range cfg.Channels {
		root.Routes = append(root.Routes, amRoute{
			Receiver:  ch.Name,
			GroupBy:   []string{"..."},
			Matchers:  []string{matcher(TestChannelLabel, "=", ch.ID)},
			GroupWait: "0s",
		})
	}
	// Watchdog always fires to show the alerting pipeline works.
	root.Routes = append(root.Routes, amRoute{Receiver: nullReceiver, Matchers: []string{matcher("alertname", "=", "Watchdog")}})
	if len(cfg.MutedAlerts) > 0 {
		root.Routes = append(root.Routes, amRoute{
			Receiver: nullReceiver,
			Matchers: []string{anyOf("alertname", cfg.MutedAlerts), matcher(OverrideLabel, "!=", "true")},
		})
	}
	for _, route := range cfg.Routing.Routes {
		matchers := routeMatchers(route)
		// A route with several receivers sends to each of them in turn.
		for i, id := range route.Receivers {
			root.Routes = append(root.Routes, amRoute{
				Receiver:       names[id],
				Matchers:       matchers,
				Continue:       route.Continue || i < len(route.Receivers)-1,
				GroupInterval:  route.GroupInterval,
				RepeatInterval: route.RepeatInterval,
			})
		}
	}

	receivers := []amReceiver{{Name: nullReceiver}}
	for _, ch := range cfg.Channels {
		receiver, err := renderReceiver(ch, forwarderURL)
		if err != nil {
			return nil, err
		}
		receivers = append(receivers, receiver)
	}
	return yaml.Marshal(amConfig{
		Global:       map[string]string{"resolve_timeout": "5m"},
		Route:        root,
		InhibitRules: defaultInhibitRules,
		Receivers:    receivers,
	})
}

func renderReceiver(ch Channel, forwarderURL string) (amReceiver, error) {
	receiver := amReceiver{Name: ch.Name}
	switch ch.Type {
	case ChannelEmail:
		receiver.EmailConfigs = []amEmailConfig{{
			SendResolved: ch.SendResolved,
			To:           ch.Email.To,
			From:         ch.Email.From,
			Smarthost:    ch.Email.Smarthost,
			AuthUsername: ch.Email.Username,
			AuthPassword: ch.Email.Password,
			RequireTLS:   ch.Email.RequireTLS,
		}}
	case ChannelWeChat:
		receiver.WeChatConfigs = []amWeChatConfig{{
			SendResolved: ch.SendResolved,
			CorpID:       ch.WeChat.CorpID,
			AgentID:      ch.WeChat.AgentID,
			APISecret:    ch.WeChat.APISecret,
			ToUser:       ch.WeChat.ToUser,
			ToParty:      ch.WeChat.ToParty,
			ToTag:        ch.WeChat.ToTag,
		}}
	case ChannelWebhook:
		receiver.WebhookConfigs = []amWebhookConfig{{SendResolved: ch.SendResolved, URL: ch.Webhook.URL}}
	case ChannelFeishu, ChannelBark:
		// The forwarder reads the target from the mounted configuration, so
		// the URL holds no secret.
		receiver.WebhookConfigs = []amWebhookConfig{{SendResolved: ch.SendResolved, URL: forwarderURL + "/channels/" + ch.ID}}
	default:
		return amReceiver{}, fmt.Errorf("unknown channel type %q", ch.Type)
	}
	return receiver, nil
}

func routeMatchers(route Route) []string {
	matchers := []string{}
	if m := anyOf("severity", route.Severities); m != "" {
		matchers = append(matchers, m)
	}
	if m := anyOf("namespace", route.Namespaces); m != "" {
		matchers = append(matchers, m)
	}
	for _, m := range route.Matchers {
		matchers = append(matchers, matcher(m.Name, m.Operator, m.Value))
	}
	return matchers
}

// anyOf matches a label against a list of literal values.
func anyOf(label string, values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return matcher(label, "=", values[0])
	}
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = regexp.QuoteMeta(value)
	}
	return matcher(label, "=~", strings.Join(quoted, "|"))
}

// matcher formats a matcher in Alertmanager's syntax. Values hold no control
// characters, so Go quoting only escapes quotes and backslashes.
func matcher(name, operator, value string) string {
	return name + operator + strconv.Quote(value)
}
