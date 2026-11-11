package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/alertmanager"
	"github.com/qzytwway/mplatform/pkg/alertnotify"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	alertNotificationsResource = "alertnotifications"

	// The Alertmanager loads its configuration from this Secret, which also
	// keeps the platform's settings it is rendered from. Sensitive settings
	// live only here.
	notificationSecretName = "mplatform-alertmanager-config"
	notificationsKey       = "notifications.json"
	alertmanagerConfigKey  = "alertmanager.yaml"

	testAlertName     = "MPlatformTestNotification"
	testAlertDuration = time.Minute
)

var (
	alertmanagerGVK     = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "Alertmanager"}
	alertmanagerListGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "AlertmanagerList"}

	errAlertmanagerDisabled   = errors.New("no Alertmanager managed by prometheus-operator in this cluster")
	errNotificationsUnmanaged = errors.New("built-in alerts are turned off through the Alertmanager configuration managed by MPlatform; save the notification settings once in the Notifications tab first")
	errConfigNotLoaded        = errors.New("alertmanager has not loaded the latest configuration yet; try again in a minute")
	errChannelNotFound        = errors.New("channel not found")
)

// notificationTarget is the cluster's Alertmanager managed by
// prometheus-operator, nil when it runs none.
type notificationTarget struct {
	alertmanager *unstructured.Unstructured
}

func (t *notificationTarget) namespace() string {
	return t.alertmanager.GetNamespace()
}

func (t *notificationTarget) managed() bool {
	if t.alertmanager == nil {
		return false
	}
	configSecret, _, _ := unstructured.NestedString(t.alertmanager.Object, "spec", "configSecret")
	return configSecret == notificationSecretName
}

// findNotificationTarget finds the Alertmanager through prometheus-operator
// only, whatever installed it, and prefers the one next to the cluster's
// Prometheus when there are several.
func findNotificationTarget(ctx context.Context, cs *cluster.ClientSet) (*notificationTarget, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(alertmanagerListGVK)
	if err := cs.K8sClient.List(ctx, list); err != nil {
		if apimeta.IsNoMatchError(err) {
			return &notificationTarget{}, nil
		}
		return nil, err
	}
	if len(list.Items) == 0 {
		return &notificationTarget{}, nil
	}
	target := &notificationTarget{alertmanager: &list.Items[0]}
	if prometheus, err := findPrometheus(ctx, cs); err == nil {
		for i := range list.Items {
			if list.Items[i].GetNamespace() == prometheus.GetNamespace() {
				target.alertmanager = &list.Items[i]
				break
			}
		}
	}
	return target, nil
}

// useNotificationSecret points the Alertmanager at the platform's
// configuration. It changes spec.configSecret only, which charts such as
// kube-prometheus-stack leave unset, so upgrades of the installation keep it.
func useNotificationSecret(ctx context.Context, cs *cluster.ClientSet, alertmanager *unstructured.Unstructured) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(alertmanagerGVK)
	obj.SetNamespace(alertmanager.GetNamespace())
	obj.SetName(alertmanager.GetName())
	patch := fmt.Appendf(nil, `{"spec":{"configSecret":%q}}`, notificationSecretName)
	return cs.K8sClient.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch), client.FieldOwner(managedByPlatform))
}

// reconcileError is why prometheus-operator could not apply the
// Alertmanager, such as an invalid configuration.
func reconcileError(alertmanager *unstructured.Unstructured) string {
	conditions, _, _ := unstructured.NestedSlice(alertmanager.Object, "status", "conditions")
	for _, item := range conditions {
		condition, _ := item.(map[string]any)
		if condition["type"] == "Reconciled" && condition["status"] == "False" {
			message, _ := condition["message"].(string)
			return message
		}
	}
	return ""
}

func loadNotifications(ctx context.Context, cs *cluster.ClientSet, namespace string) (alertnotify.Config, error) {
	secret, err := cs.K8sClient.ClientSet.CoreV1().Secrets(namespace).Get(ctx, notificationSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return alertnotify.DefaultConfig(), nil
	}
	if err != nil {
		return alertnotify.Config{}, err
	}
	var cfg alertnotify.Config
	if err := json.Unmarshal(secret.Data[notificationsKey], &cfg); err != nil {
		return alertnotify.Config{}, fmt.Errorf("failed to read %s/%s: %w", namespace, notificationSecretName, err)
	}
	return cfg, nil
}

// saveNotifications writes the settings and the Alertmanager configuration
// rendered from them, and deploys the forwarder when a channel needs it.
func saveNotifications(ctx context.Context, cs *cluster.ClientSet, namespace string, cfg alertnotify.Config) error {
	settings, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	rendered, err := alertnotify.Render(cfg, forwarderURL(namespace))
	if err != nil {
		return err
	}
	secrets := cs.K8sClient.ClientSet.CoreV1().Secrets(namespace)
	data := map[string][]byte{notificationsKey: settings, alertmanagerConfigKey: rendered}
	existing, err := secrets.Get(ctx, notificationSecretName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationSecretName,
				Namespace: namespace,
				Labels:    map[string]string{managedByLabel: managedByPlatform},
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		}, metav1.CreateOptions{})
	case err == nil:
		updated := existing.DeepCopy()
		updated.Data = data
		_, err = secrets.Update(ctx, updated, metav1.UpdateOptions{})
	}
	if err != nil {
		return err
	}
	if cfg.NeedsForwarder() {
		return ensureForwarder(ctx, cs, namespace)
	}
	return nil
}

// setAlertMuted adds a built-in alert to the alerts the platform's
// Alertmanager configuration drops, or removes it. It never takes over an
// Alertmanager that loads another configuration.
func setAlertMuted(ctx context.Context, cs *cluster.ClientSet, alert string, muted bool) error {
	target, err := findNotificationTarget(ctx, cs)
	if err != nil {
		return err
	}
	if target.alertmanager == nil {
		return errAlertmanagerDisabled
	}
	if !target.managed() {
		return errNotificationsUnmanaged
	}
	cfg, err := loadNotifications(ctx, cs, target.namespace())
	if err != nil {
		return err
	}
	cfg.MutedAlerts = slices.DeleteFunc(slices.Clone(cfg.MutedAlerts), func(name string) bool { return name == alert })
	if muted {
		cfg.MutedAlerts = append(cfg.MutedAlerts, alert)
	}
	normalized, err := cfg.Normalize()
	if err != nil {
		return err
	}
	return saveNotifications(ctx, cs, target.namespace(), normalized)
}

// MutedAlerts returns the built-in alerts turned off in the platform, none
// while the cluster's Alertmanager does not load the platform's
// configuration.
func MutedAlerts(ctx context.Context, cs *cluster.ClientSet) ([]string, error) {
	target, err := findNotificationTarget(ctx, cs)
	if err != nil || !target.managed() {
		return nil, err
	}
	cfg, err := loadNotifications(ctx, cs, target.namespace())
	if err != nil {
		return nil, err
	}
	return cfg.MutedAlerts, nil
}

// canManageNotifications requires a cluster-wide alertnotifications grant,
// since notifications carry alerts of every namespace. It writes the error
// response itself.
func canManageNotifications(c *gin.Context, verb common.Verb) bool {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if !rbac.CanAccessAllNamespaces(user, alertNotificationsResource, string(verb), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(verb), alertNotificationsResource, common.AllNamespaces, cs.Name)})
		return false
	}
	return true
}

func writeNotificationError(c *gin.Context, err error) {
	status := ruleErrorStatus(err)
	switch {
	case errors.Is(err, errConfigNotLoaded):
		status = http.StatusConflict
	case errors.Is(err, errChannelNotFound):
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

type notificationsResponse struct {
	// Enabled is whether the cluster runs an Alertmanager, and Managed
	// whether it loads the platform's configuration.
	Enabled        bool                        `json:"enabled"`
	Managed        bool                        `json:"managed"`
	ReconcileError string                      `json:"reconcileError,omitempty"`
	Channels       []alertnotify.MaskedChannel `json:"channels"`
	Routing        alertnotify.Routing         `json:"routing"`
}

func GetAlertNotifications(c *gin.Context) {
	if !canManageNotifications(c, common.VerbGet) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	ctx := c.Request.Context()
	target, err := findNotificationTarget(ctx, cs)
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	cfg := alertnotify.DefaultConfig()
	if target.alertmanager != nil {
		if cfg, err = loadNotifications(ctx, cs, target.namespace()); err != nil {
			writeNotificationError(c, err)
			return
		}
	}
	response := notificationsResponse{
		Enabled:  target.alertmanager != nil,
		Managed:  target.managed(),
		Channels: cfg.MaskedChannels(),
		Routing:  cfg.Routing,
	}
	if target.alertmanager != nil {
		response.ReconcileError = reconcileError(target.alertmanager)
	}
	c.JSON(http.StatusOK, response)
}

// UpdateAlertNotifications saves the channels and routing. The first save
// points the cluster's Alertmanager at the platform's configuration, which
// replaces the one it loaded before.
func UpdateAlertNotifications(c *gin.Context) {
	if !canManageNotifications(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var req alertnotify.Config
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	target, err := findNotificationTarget(ctx, cs)
	if err == nil && target.alertmanager == nil {
		err = errAlertmanagerDisabled
	}
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	stored, err := loadNotifications(ctx, cs, target.namespace())
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	// The muted alerts belong to the rules page.
	req.MutedAlerts = stored.MutedAlerts
	cfg, err := req.WithStoredSecrets(stored).Normalize()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// The Secret goes first: prometheus-operator cannot apply an Alertmanager
	// whose configSecret is missing.
	if err := saveNotifications(ctx, cs, target.namespace(), cfg); err != nil {
		writeNotificationError(c, err)
		return
	}
	if !target.managed() {
		if err := useNotificationSecret(ctx, cs, target.alertmanager); err != nil {
			writeNotificationError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "notifications updated"})
}

// SendTestNotification posts an alert that the configuration routes straight
// to one channel, so it is sent the way real alerts are.
func SendTestNotification(c *gin.Context) {
	if !canManageNotifications(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if cs.AlertmanagerClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Alertmanager is not configured for this cluster"})
		return
	}
	ctx := c.Request.Context()
	target, err := findNotificationTarget(ctx, cs)
	if err == nil && target.alertmanager == nil {
		err = errAlertmanagerDisabled
	}
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	cfg, err := loadNotifications(ctx, cs, target.namespace())
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	ch, ok := cfg.Channel(c.Param("id"))
	if !ok {
		writeNotificationError(c, errChannelNotFound)
		return
	}
	if err := checkReceiverLoaded(ctx, cs.AlertmanagerClient, ch.Name); errors.Is(err, errConfigNotLoaded) {
		writeNotificationError(c, err)
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	user := c.MustGet("user").(model.User)
	err = cs.AlertmanagerClient.PostAlerts(ctx, []alertmanager.PostableAlert{{
		Labels: map[string]string{
			"alertname":                  testAlertName,
			alertnotify.TestChannelLabel: ch.ID,
			// Every test is a new alert, so it is sent even right after
			// another one.
			"test_id": strconv.FormatInt(now.UnixNano(), 36),
		},
		Annotations: map[string]string{
			"summary":     "Test notification from MPlatform",
			"description": fmt.Sprintf("%s sent this to check the channel %s of cluster %s.", user.Key(), ch.Name, cs.Name),
		},
		StartsAt: now,
		EndsAt:   now.Add(testAlertDuration),
	}})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "test notification sent"})
}

// checkReceiverLoaded fails while Alertmanager runs a configuration without
// the channel: a saved Secret takes a minute or so to reach it, and a test
// alert sent before would go to the default receiver instead.
func checkReceiverLoaded(ctx context.Context, am *alertmanager.Client, name string) error {
	loaded, err := am.LoadedConfig(ctx)
	if err != nil {
		return err
	}
	type receiver struct {
		Name string `yaml:"name"`
	}
	var parsed struct {
		Receivers []receiver `yaml:"receivers"`
	}
	if err := yaml.Unmarshal([]byte(loaded), &parsed); err != nil {
		return fmt.Errorf("failed to read the Alertmanager configuration: %w", err)
	}
	for _, r := range parsed.Receivers {
		if r.Name == name {
			return nil
		}
	}
	return errConfigNotLoaded
}
