package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Sources of the alerting rules a cluster's Prometheus loads.
const (
	RuleSourceBuiltin = "builtin"
	RuleSourceCustom  = "custom"
	RuleSourceOther   = "other"
)

const (
	alertRulesResource = "alertrules"

	// The platform marks the PrometheusRules it creates, and names the
	// built-in alert a rule replaces.
	managedByLabel      = "app.kubernetes.io/managed-by"
	managedByPlatform   = "mplatform"
	overridesAnnotation = "mplatform.io/overrides"

	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	exprCheckTimeout = 10 * time.Second
	maxNamePrefix    = 50
)

var (
	prometheusRuleGVK     = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRule"}
	prometheusRuleListGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRuleList"}
	prometheusListGVK     = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusList"}

	alertNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	durationPattern  = regexp.MustCompile(`^([0-9]+(ms|s|m|h|d|w|y))+$`)
	nonNameChars     = regexp.MustCompile(`[^a-z0-9]+`)

	errNoPrometheus   = errors.New("no Prometheus managed by prometheus-operator in this cluster")
	errNoRuleSelector = errors.New("the cluster's Prometheus does not select any PrometheusRules")
	errNotHelmManaged = errors.New("the built-in rules of the cluster's Prometheus are not managed by a Helm release")
	errNotPlatform    = errors.New("only rules created in MPlatform can be changed here")
)

// RuleSource is where the alerting rules of a PrometheusRule come from: the
// release of the cluster's Prometheus (builtin), the platform (custom), or
// anything else.
type RuleSource struct {
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Overrides string `json:"overrides,omitempty"`
}

// ruleTarget is the Prometheus that loads the cluster's rules: where custom
// rules go, the labels it selects them by and the Helm release that ships
// its built-in rules.
type ruleTarget struct {
	namespace        string
	labels           map[string]string
	releaseName      string
	releaseNamespace string
}

func findRuleTarget(ctx context.Context, cs *cluster.ClientSet) (*ruleTarget, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(prometheusListGVK)
	if err := cs.K8sClient.List(ctx, list); err != nil {
		if apimeta.IsNoMatchError(err) {
			return nil, errNoPrometheus
		}
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, errNoPrometheus
	}
	prometheus := list.Items[0]
	if _, found, _ := unstructured.NestedFieldNoCopy(prometheus.Object, "spec", "ruleSelector"); !found {
		return nil, errNoRuleSelector
	}
	labels, _, err := unstructured.NestedStringMap(prometheus.Object, "spec", "ruleSelector", "matchLabels")
	if err != nil {
		return nil, err
	}
	annotations := prometheus.GetAnnotations()
	return &ruleTarget{
		namespace:        prometheus.GetNamespace(),
		labels:           labels,
		releaseName:      annotations[helmReleaseNameAnnotation],
		releaseNamespace: annotations[helmReleaseNamespaceAnnotation],
	}, nil
}

func (t *ruleTarget) sourceOf(rule *unstructured.Unstructured) string {
	if rule.GetLabels()[managedByLabel] == managedByPlatform {
		return RuleSourceCustom
	}
	annotations := rule.GetAnnotations()
	if t.releaseName != "" && annotations[helmReleaseNameAnnotation] == t.releaseName && annotations[helmReleaseNamespaceAnnotation] == t.releaseNamespace {
		return RuleSourceBuiltin
	}
	return RuleSourceOther
}

func listPrometheusRules(ctx context.Context, cs *cluster.ClientSet) ([]unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(prometheusRuleListGVK)
	if err := cs.K8sClient.List(ctx, list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// RuleSources maps the UID of every PrometheusRule to where it comes from.
// prometheus-operator names the rule files Prometheus reports after them.
func RuleSources(ctx context.Context, cs *cluster.ClientSet) (map[string]RuleSource, error) {
	target, err := findRuleTarget(ctx, cs)
	if err != nil {
		return nil, err
	}
	rules, err := listPrometheusRules(ctx, cs)
	if err != nil {
		return nil, err
	}
	sources := make(map[string]RuleSource, len(rules))
	for i := range rules {
		rule := &rules[i]
		sources[string(rule.GetUID())] = RuleSource{
			Source:    target.sourceOf(rule),
			Namespace: rule.GetNamespace(),
			Name:      rule.GetName(),
			Overrides: rule.GetAnnotations()[overridesAnnotation],
		}
	}
	return sources, nil
}

type ruleRequest struct {
	Name        string `json:"name"`
	Expr        string `json:"expr"`
	For         string `json:"for"`
	Severity    string `json:"severity"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

// ruleLocation is the position of a rule in a PrometheusRule.
type ruleLocation struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	Index     int    `json:"index"`
}

func (r *ruleRequest) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Expr = strings.TrimSpace(r.Expr)
	r.For = strings.TrimSpace(r.For)
	r.Severity = strings.TrimSpace(r.Severity)
	r.Summary = strings.TrimSpace(r.Summary)
	r.Description = strings.TrimSpace(r.Description)
	if !alertNamePattern.MatchString(r.Name) {
		return fmt.Errorf("name must start with a letter or underscore and contain only letters, digits and underscores")
	}
	if r.Expr == "" {
		return fmt.Errorf("expression is required")
	}
	if r.For != "" && !durationPattern.MatchString(r.For) {
		return fmt.Errorf("duration must look like 5m or 1h30m")
	}
	return nil
}

// apply writes the request into a rule and keeps its other labels and
// annotations, such as a built-in rule's runbook_url.
func (r *ruleRequest) apply(rule map[string]any) {
	rule["alert"] = r.Name
	rule["expr"] = r.Expr
	if r.For == "" {
		delete(rule, "for")
	} else {
		rule["for"] = r.For
	}
	setRuleField(rule, "labels", "severity", r.Severity)
	setRuleField(rule, "annotations", "summary", r.Summary)
	setRuleField(rule, "annotations", "description", r.Description)
}

func setRuleField(rule map[string]any, field, key, value string) {
	values, _ := rule[field].(map[string]any)
	if values == nil {
		values = map[string]any{}
	}
	if value == "" {
		delete(values, key)
	} else {
		values[key] = value
	}
	if len(values) == 0 {
		delete(rule, field)
	} else {
		rule[field] = values
	}
}

// checkExpr evaluates the expression once, so a syntax error is reported
// instead of Prometheus skipping the rule file. Other query failures are left
// to the operator's own validation.
func checkExpr(ctx context.Context, cs *cluster.ClientSet, expr string) error {
	if cs.PromClient == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, exprCheckTimeout)
	defer cancel()
	_, _, err := cs.PromClient.Query(ctx, expr, time.Now())
	var apiErr *v1.Error
	if errors.As(err, &apiErr) && apiErr.Type == v1.ErrBadData {
		return fmt.Errorf("invalid expression: %s", apiErr.Msg)
	}
	return nil
}

func newPrometheusRule(target *ruleTarget, alert string, rules []any) *unstructured.Unstructured {
	prefix := strings.Trim(nonNameChars.ReplaceAllString(strings.ToLower(alert), "-"), "-")
	if len(prefix) > maxNamePrefix {
		prefix = strings.TrimRight(prefix[:maxNamePrefix], "-")
	}
	labels := map[string]string{}
	for key, value := range target.labels {
		labels[key] = value
	}
	labels[managedByLabel] = managedByPlatform

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(prometheusRuleGVK)
	obj.SetNamespace(target.namespace)
	obj.SetGenerateName("mplatform-" + prefix + "-")
	obj.SetLabels(labels)
	obj.Object["spec"] = map[string]any{
		"groups": []any{map[string]any{"name": alert, "rules": rules}},
	}
	return obj
}

func getPlatformRule(ctx context.Context, cs *cluster.ClientSet, namespace, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(prometheusRuleGVK)
	if err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, err
	}
	if obj.GetLabels()[managedByLabel] != managedByPlatform {
		return nil, errNotPlatform
	}
	return obj, nil
}

// ruleAt returns the alerting rule at a position of a PrometheusRule, to be
// changed in place.
func ruleAt(obj *unstructured.Unstructured, group string, index int) map[string]any {
	groups, _, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "groups")
	items, _ := groups.([]any)
	for _, item := range items {
		ruleGroup, _ := item.(map[string]any)
		if ruleGroup["name"] != group {
			continue
		}
		rules, _ := ruleGroup["rules"].([]any)
		if index < 0 || index >= len(rules) {
			return nil
		}
		rule, _ := rules[index].(map[string]any)
		if _, ok := rule["alert"]; !ok {
			return nil
		}
		return rule
	}
	return nil
}

// findOverride returns the platform rule replacing a built-in alert.
func findOverride(rules []unstructured.Unstructured, alert string) *unstructured.Unstructured {
	for i := range rules {
		rule := &rules[i]
		if rule.GetLabels()[managedByLabel] == managedByPlatform && rule.GetAnnotations()[overridesAnnotation] == alert {
			return rule
		}
	}
	return nil
}

// builtinCopies copies every built-in rule named alert, since disabling an
// alert in the chart removes all of them; the rule at loc gets the request.
func builtinCopies(target *ruleTarget, rules []unstructured.Unstructured, alert string, loc ruleLocation, req *ruleRequest) (copies []any, edited bool) {
	for i := range rules {
		obj := &rules[i]
		if target.sourceOf(obj) != RuleSourceBuiltin {
			continue
		}
		groups, _, _ := unstructured.NestedSlice(obj.Object, "spec", "groups")
		for _, item := range groups {
			ruleGroup, _ := item.(map[string]any)
			groupRules, _ := ruleGroup["rules"].([]any)
			for index, groupRule := range groupRules {
				rule, _ := groupRule.(map[string]any)
				if rule["alert"] != alert {
					continue
				}
				if obj.GetNamespace() == loc.Namespace && obj.GetName() == loc.Name && ruleGroup["name"] == loc.Group && index == loc.Index {
					req.apply(rule)
					edited = true
				}
				copies = append(copies, rule)
			}
		}
	}
	return copies, edited
}

// setBuiltinDisabled turns a built-in alert off or back on through the
// defaultRules.disabled values of the release that ships it. The release keeps
// its deployed chart and values; only this key changes.
func setBuiltinDisabled(c *gin.Context, cs *cluster.ClientSet, target *ruleTarget, alert string, disabled bool) error {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, target.releaseNamespace)
	if err != nil {
		return err
	}
	rel, err := helmutil.GetRelease(cfg, target.releaseName)
	if err != nil {
		return err
	}
	action := "enabled"
	if disabled {
		action = "disabled"
	}
	values := map[string]any{"defaultRules": map[string]any{"disabled": map[string]any{alert: disabled}}}
	return helmutil.UpgradePlatformRelease(c.Request.Context(), cfg, cs.Name, c.MustGet("user").(model.User).ID, target.releaseName, rel.Chart, values, helmutil.UpgradeReleaseOptions{
		Namespace:   target.releaseNamespace,
		Timeout:     helmActionTimeout,
		ReuseValues: true,
		Description: fmt.Sprintf("Alert %s %s by MPlatform", alert, action),
	})
}

// canWriteRules requires a cluster-wide alertrules grant, since rules see
// series from every namespace. It writes the error response itself.
func canWriteRules(c *gin.Context, verb common.Verb) bool {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if !rbac.CanAccessAllNamespaces(user, alertRulesResource, string(verb), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(verb), alertRulesResource, common.AllNamespaces, cs.Name)})
		return false
	}
	return true
}

func ruleErrorStatus(err error) int {
	switch {
	case errors.Is(err, errNoPrometheus), errors.Is(err, errNoRuleSelector), errors.Is(err, errNotHelmManaged):
		return http.StatusConflict
	case errors.Is(err, errNotPlatform):
		return http.StatusForbidden
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) && status.Status().Code >= http.StatusBadRequest {
		return int(status.Status().Code)
	}
	return http.StatusInternalServerError
}

func writeRuleError(c *gin.Context, err error) {
	c.JSON(ruleErrorStatus(err), gin.H{"error": err.Error()})
}

func CreateAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbCreate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var req ruleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	target, err := findRuleTarget(ctx, cs)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	if err := checkExpr(ctx, cs, req.Expr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule := map[string]any{}
	req.apply(rule)
	obj := newPrometheusRule(target, req.Name, []any{rule})
	if err := cs.K8sClient.Create(ctx, obj); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"namespace": obj.GetNamespace(), "name": obj.GetName()})
}

func UpdateAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var req struct {
		ruleRequest
		Group string `json:"group"`
		Index int    `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	obj, err := getPlatformRule(ctx, cs, c.Param("namespace"), c.Param("name"))
	if err != nil {
		writeRuleError(c, err)
		return
	}
	if overrides := obj.GetAnnotations()[overridesAnnotation]; overrides != "" && req.Name != overrides {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("a modified built-in rule keeps its name %s", overrides)})
		return
	}
	rule := ruleAt(obj, req.Group, req.Index)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	if err := checkExpr(ctx, cs, req.Expr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.apply(rule)
	if err := cs.K8sClient.Update(ctx, obj); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rule updated"})
}

// DeleteAlertRule deletes a platform rule. Deleting a modified built-in rule
// turns the built-in one back on first, so the alert is never missing.
func DeleteAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbDelete) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	ctx := c.Request.Context()
	obj, err := getPlatformRule(ctx, cs, c.Param("namespace"), c.Param("name"))
	if err != nil {
		writeRuleError(c, err)
		return
	}
	if alert := obj.GetAnnotations()[overridesAnnotation]; alert != "" {
		target, err := findRuleTarget(ctx, cs)
		if err == nil && target.releaseName == "" {
			err = errNotHelmManaged
		}
		if err == nil {
			err = setBuiltinDisabled(c, cs, target, alert, false)
		}
		if err != nil {
			writeRuleError(c, err)
			return
		}
	}
	if err := cs.K8sClient.Delete(ctx, obj); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

// builtinTarget is the rule target of a request on built-in rules, which
// needs the Helm release shipping them.
func builtinTarget(c *gin.Context, cs *cluster.ClientSet) (*ruleTarget, bool) {
	target, err := findRuleTarget(c.Request.Context(), cs)
	if err == nil && target.releaseName == "" {
		err = errNotHelmManaged
	}
	if err != nil {
		writeRuleError(c, err)
		return nil, false
	}
	return target, true
}

type disabledRule struct {
	Alert      string `json:"alert"`
	Overridden bool   `json:"overridden"`
}

// ListDisabledBuiltinRules lists the built-in alerts turned off in the
// release values. Prometheus no longer reports them, so this is the only
// place they can be turned back on.
func ListDisabledBuiltinRules(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if !rbac.CanAccessAllNamespaces(user, "prometheus", string(common.VerbGet), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), "get", "prometheus", common.AllNamespaces, cs.Name)})
		return
	}
	target, ok := builtinTarget(c, cs)
	if !ok {
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, target.releaseNamespace)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	rel, err := helmutil.GetRelease(cfg, target.releaseName)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	rules, err := listPrometheusRules(c.Request.Context(), cs)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	disabled, _, _ := unstructured.NestedMap(rel.Config, "defaultRules", "disabled")
	result := []disabledRule{}
	for alert, value := range disabled {
		if off, _ := value.(bool); off {
			result = append(result, disabledRule{Alert: alert, Overridden: findOverride(rules, alert) != nil})
		}
	}
	c.JSON(http.StatusOK, result)
}

// SetBuiltinRuleEnabled turns a built-in alert off or back on. A modified
// alert comes back by deleting its replacement instead.
func SetBuiltinRuleEnabled(c *gin.Context) {
	if !canWriteRules(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	alert := c.Param("alert")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !alertNamePattern.MatchString(alert) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "an alert name and enabled are required"})
		return
	}
	target, ok := builtinTarget(c, cs)
	if !ok {
		return
	}
	rules, err := listPrometheusRules(c.Request.Context(), cs)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	if findOverride(rules, alert) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "this built-in rule is modified; delete the modified rule to restore it"})
		return
	}
	if !req.Enabled {
		if copies, _ := builtinCopies(target, rules, alert, ruleLocation{}, nil); len(copies) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "built-in rule not found"})
			return
		}
	}
	if err := setBuiltinDisabled(c, cs, target, alert, !req.Enabled); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "built-in rule updated"})
}

// OverrideBuiltinRule modifies a built-in rule: it adds a platform rule with
// every rule of that alert name, the one at the request's location changed,
// and then turns the built-in alert off.
func OverrideBuiltinRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbCreate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var req struct {
		ruleRequest
		Source ruleLocation `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alert := c.Param("alert")
	if req.Name != alert {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("a modified built-in rule keeps its name %s", alert)})
		return
	}
	ctx := c.Request.Context()
	target, ok := builtinTarget(c, cs)
	if !ok {
		return
	}
	rules, err := listPrometheusRules(ctx, cs)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	if findOverride(rules, alert) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "this built-in rule is already modified"})
		return
	}
	copies, edited := builtinCopies(target, rules, alert, req.Source, &req.ruleRequest)
	if !edited {
		c.JSON(http.StatusNotFound, gin.H{"error": "built-in rule not found"})
		return
	}
	if err := checkExpr(ctx, cs, req.Expr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	obj := newPrometheusRule(target, alert, copies)
	obj.SetAnnotations(map[string]string{overridesAnnotation: alert})
	if err := cs.K8sClient.Create(ctx, obj); err != nil {
		writeRuleError(c, err)
		return
	}
	if err := setBuiltinDisabled(c, cs, target, alert, true); err != nil {
		if deleteErr := cs.K8sClient.Delete(ctx, obj); deleteErr != nil {
			err = fmt.Errorf("%w; the modified rule %s/%s was also left in place: %w", err, obj.GetNamespace(), obj.GetName(), deleteErr)
		}
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"namespace": obj.GetNamespace(), "name": obj.GetName()})
}
