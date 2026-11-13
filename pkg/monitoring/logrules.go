package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/logging"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuleSourceLogs marks the rules vmalert evaluates against the cluster's
// VictoriaLogs.
const RuleSourceLogs = "logs"

const (
	// The log rules live in a ConfigMap next to VictoriaLogs: the platform's
	// rules, and the vmalert rule file rendered from them.
	logRulesConfigMap = "mplatform-log-alert-rules"
	logRulesKey       = "rules.json"
	vmalertRulesKey   = "rules.yaml"

	maxLogKeyword = 256
	maxThreshold  = 1000000
)

// logRuleWindows are the windows a rule counts matching logs over; vmalert
// evaluates a rule once per window.
var logRuleWindows = []string{"1m", "5m", "15m", "30m", "1h"}

// LogRule fires for each container whose logs contain Keyword at least
// Threshold times within Window.
type LogRule struct {
	Name        string `json:"name"`
	Keyword     string `json:"keyword"`
	Namespace   string `json:"namespace"`
	PodPrefix   string `json:"podPrefix"`
	Container   string `json:"container"`
	Window      string `json:"window"`
	Threshold   int    `json:"threshold"`
	Severity    string `json:"severity"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

func (r *LogRule) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Keyword = strings.TrimSpace(r.Keyword)
	r.Namespace = strings.TrimSpace(r.Namespace)
	r.PodPrefix = strings.TrimSpace(r.PodPrefix)
	r.Container = strings.TrimSpace(r.Container)
	r.Severity = strings.TrimSpace(r.Severity)
	r.Summary = strings.TrimSpace(r.Summary)
	r.Description = strings.TrimSpace(r.Description)
	if !alertNamePattern.MatchString(r.Name) {
		return fmt.Errorf("name must start with a letter or underscore and contain only letters, digits and underscores")
	}
	if r.Keyword == "" || len(r.Keyword) > maxLogKeyword {
		return fmt.Errorf("keyword is required and must be at most %d characters", maxLogKeyword)
	}
	if !slices.Contains(logRuleWindows, r.Window) {
		return fmt.Errorf("window must be one of %s", strings.Join(logRuleWindows, ", "))
	}
	if r.Threshold < 1 || r.Threshold > maxThreshold {
		return fmt.Errorf("threshold must be between 1 and %d", maxThreshold)
	}
	return nil
}

// LogsQL counts the matching logs per container and keeps the containers at
// the threshold, with the plain namespace, pod and container labels other
// alerts have. User input is quoted, so it cannot add pipes or filters.
func (r LogRule) LogsQL() string {
	filters := []string{}
	if r.Namespace != "" {
		filters = append(filters, logging.NamespaceField+":="+strconv.Quote(r.Namespace))
	}
	if r.PodPrefix != "" {
		filters = append(filters, logging.PodField+":="+strconv.Quote(r.PodPrefix)+"*")
	}
	if r.Container != "" {
		filters = append(filters, logging.ContainerField+":="+strconv.Quote(r.Container))
	}
	filters = append(filters, strconv.Quote(r.Keyword))
	return fmt.Sprintf("%s | stats by (%s, %s, %s) count() as hits | filter hits:>=%d | rename %s as namespace, %s as pod, %s as container",
		strings.Join(filters, " "),
		logging.NamespaceField, logging.PodField, logging.ContainerField,
		r.Threshold,
		logging.NamespaceField, logging.PodField, logging.ContainerField)
}

type vmalertGroup struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"`
	Interval string        `yaml:"interval"`
	Rules    []vmalertRule `yaml:"rules"`
}

type vmalertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// renderVMAlertRules gives each rule a group of its own, since vmalert
// queries the logs of the last group interval: the rule's window.
func renderVMAlertRules(rules []LogRule) (string, error) {
	groups := make([]vmalertGroup, 0, len(rules))
	for _, r := range rules {
		rule := vmalertRule{Alert: r.Name, Expr: r.LogsQL(), Labels: map[string]string{}, Annotations: map[string]string{}}
		if r.Severity != "" {
			rule.Labels["severity"] = r.Severity
		}
		if r.Summary != "" {
			rule.Annotations["summary"] = r.Summary
		}
		if r.Description != "" {
			rule.Annotations["description"] = r.Description
		}
		groups = append(groups, vmalertGroup{Name: r.Name, Type: "vlogs", Interval: r.Window, Rules: []vmalertRule{rule}})
	}
	data, err := yaml.Marshal(map[string]any{"groups": groups})
	if err != nil {
		return "", fmt.Errorf("failed to render vmalert rules: %w", err)
	}
	return string(data), nil
}

// LoadLogRules returns the cluster's log rules, none when it has no rule
// ConfigMap yet.
func LoadLogRules(ctx context.Context, cs *cluster.ClientSet) ([]LogRule, error) {
	cm, err := cs.K8sClient.ClientSet.CoreV1().ConfigMaps(logging.Namespace).Get(ctx, logRulesConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return []LogRule{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load log alert rules: %w", err)
	}
	rules := []LogRule{}
	if data := cm.Data[logRulesKey]; data != "" {
		if err := json.Unmarshal([]byte(data), &rules); err != nil {
			return nil, fmt.Errorf("failed to parse log alert rules: %w", err)
		}
	}
	return rules, nil
}

func saveLogRules(ctx context.Context, cs *cluster.ClientSet, rules []LogRule) error {
	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	rendered, err := renderVMAlertRules(rules)
	if err != nil {
		return err
	}
	configMaps := cs.K8sClient.ClientSet.CoreV1().ConfigMaps(logging.Namespace)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      logRulesConfigMap,
			Namespace: logging.Namespace,
			Labels:    map[string]string{managedByLabel: managedByPlatform},
		},
		Data: map[string]string{logRulesKey: string(data), vmalertRulesKey: rendered},
	}
	existing, err := configMaps.Get(ctx, logRulesConfigMap, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = configMaps.Create(ctx, cm, metav1.CreateOptions{})
	case err == nil:
		updated := existing.DeepCopy()
		updated.Labels = cm.Labels
		updated.Data = cm.Data
		_, err = configMaps.Update(ctx, updated, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to save log alert rules: %w", err)
	}
	return nil
}
