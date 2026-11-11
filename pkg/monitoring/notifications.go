package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/alertmanager"
	"github.com/qzytwway/mplatform/pkg/alertnotify"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	alertmanagerListGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "AlertmanagerList"}

	errAlertmanagerDisabled = errors.New("alertmanager is not enabled in the cluster's kube-prometheus-stack")
	errAlertmanagerEnabled  = errors.New("alertmanager is already enabled")
	errConfigNotLoaded      = errors.New("alertmanager has not loaded the latest configuration yet; try again in a minute")
	errChannelNotFound      = errors.New("channel not found")
)

// notificationTarget is the Helm release of the cluster's Prometheus and the
// Alertmanager it ships, nil while the release has it disabled.
type notificationTarget struct {
	releaseName      string
	releaseNamespace string
	alertmanager     *unstructured.Unstructured
}

func (t *notificationTarget) namespace() string {
	if t.alertmanager != nil {
		return t.alertmanager.GetNamespace()
	}
	return t.releaseNamespace
}

func (t *notificationTarget) managed() bool {
	if t.alertmanager == nil {
		return false
	}
	configSecret, _, _ := unstructured.NestedString(t.alertmanager.Object, "spec", "configSecret")
	return configSecret == notificationSecretName
}

func findNotificationTarget(ctx context.Context, cs *cluster.ClientSet) (*notificationTarget, error) {
	prometheus, err := findPrometheus(ctx, cs)
	if err != nil {
		return nil, err
	}
	annotations := prometheus.GetAnnotations()
	target := &notificationTarget{
		releaseName:      annotations[helmReleaseNameAnnotation],
		releaseNamespace: annotations[helmReleaseNamespaceAnnotation],
	}
	if target.releaseName == "" {
		return nil, errNotHelmManaged
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(alertmanagerListGVK)
	if err := cs.K8sClient.List(ctx, list); err != nil && !apimeta.IsNoMatchError(err) {
		return nil, err
	}
	for i := range list.Items {
		annotations := list.Items[i].GetAnnotations()
		if annotations[helmReleaseNameAnnotation] == target.releaseName && annotations[helmReleaseNamespaceAnnotation] == target.releaseNamespace {
			target.alertmanager = &list.Items[i]
			break
		}
	}
	return target, nil
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

// setAlertmanagerValues upgrades the release with its deployed chart and
// values, changing only the given alertmanager values.
func setAlertmanagerValues(c *gin.Context, cs *cluster.ClientSet, target *notificationTarget, values map[string]any, description string) error {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, target.releaseNamespace)
	if err != nil {
		return err
	}
	rel, err := helmutil.GetRelease(cfg, target.releaseName)
	if err != nil {
		return err
	}
	return helmutil.UpgradePlatformRelease(c.Request.Context(), cfg, cs.Name, c.MustGet("user").(model.User).ID, target.releaseName, rel.Chart, map[string]any{"alertmanager": values}, helmutil.UpgradeReleaseOptions{
		Namespace:   target.releaseNamespace,
		Timeout:     helmActionTimeout,
		ReuseValues: true,
		Description: description,
	})
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
	case errors.Is(err, errAlertmanagerDisabled), errors.Is(err, errAlertmanagerEnabled), errors.Is(err, errConfigNotLoaded):
		status = http.StatusConflict
	case errors.Is(err, errChannelNotFound):
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

type notificationsResponse struct {
	// Enabled is whether the release ships an Alertmanager, and Managed
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
	cfg, err := loadNotifications(ctx, cs, target.namespace())
	if err != nil {
		writeNotificationError(c, err)
		return
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
// points the release's Alertmanager at the platform's configuration, which
// replaces the one in the release values.
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
	cfg, err := req.WithStoredSecrets(stored).Normalize()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := saveNotifications(ctx, cs, target.namespace(), cfg); err != nil {
		writeNotificationError(c, err)
		return
	}
	if !target.managed() {
		values := map[string]any{"alertmanagerSpec": map[string]any{"configSecret": notificationSecretName}}
		if err := setAlertmanagerValues(c, cs, target, values, "Alertmanager configuration managed by MPlatform"); err != nil {
			writeNotificationError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "notifications updated"})
}

// EnableAlertmanager turns on the Alertmanager of the release, loading the
// platform's configuration from the start.
func EnableAlertmanager(c *gin.Context) {
	if !canManageNotifications(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	ctx := c.Request.Context()
	target, err := findNotificationTarget(ctx, cs)
	if err == nil && target.alertmanager != nil {
		err = errAlertmanagerEnabled
	}
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	cfg, err := loadNotifications(ctx, cs, target.namespace())
	if err == nil {
		err = saveNotifications(ctx, cs, target.namespace(), cfg)
	}
	if err == nil {
		values := map[string]any{"enabled": true, "alertmanagerSpec": map[string]any{"configSecret": notificationSecretName}}
		err = setAlertmanagerValues(c, cs, target, values, "Alertmanager enabled by MPlatform")
	}
	if err != nil {
		writeNotificationError(c, err)
		return
	}
	// Connect the new Alertmanager.
	cluster.TriggerClusterSync()
	c.JSON(http.StatusOK, gin.H{"message": "alertmanager enabled"})
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
func checkReceiverLoaded(ctx context.Context, client *alertmanager.Client, name string) error {
	loaded, err := client.LoadedConfig(ctx)
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
