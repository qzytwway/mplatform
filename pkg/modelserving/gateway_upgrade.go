package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// gatewayUpgradeRequired reports whether the gateway's release lacks values
// the platform installs it with now, such as Redis and its password, or the
// gateway has no key to administer quotas with yet.
func gatewayUpgradeRequired(ctx context.Context, cs *cluster.ClientSet, gateway *model.ModelGateway) (bool, error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, gatewayNamespace)
	if err != nil {
		return false, err
	}
	rel, err := helmutil.GetRelease(cfg, gatewayReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// A failed upgrade leaves its values in the release without applying them.
	if rel.Info.Status != releasecommon.StatusDeployed {
		return true, nil
	}
	metrics, err := gatewayMetricsValues(ctx, cs)
	if err != nil {
		return false, err
	}
	storageClass, err := redisStorageClass(ctx, cs)
	if errors.Is(err, errNoDefaultStorageClass) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	// The release stores its values as JSON, so compare them decoded from it.
	var want any
	encoded, err := json.Marshal(gatewayValues(gateway, metrics, storageClass))
	if err == nil {
		err = json.Unmarshal(encoded, &want)
	}
	if err != nil {
		return false, err
	}
	return gateway.QuotaAdminKey == "" || gateway.RedisPassword == "" || !valuesContain(rel.Config, want), nil
}

// valuesContain reports whether have sets every value of want.
func valuesContain(have, want any) bool {
	wantMap, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(have, want)
	}
	haveMap, _ := have.(map[string]any)
	for key, value := range wantMap {
		if !valuesContain(haveMap[key], value) {
			return false
		}
	}
	return true
}

// UpgradeGateway brings an installed gateway to the values new gateways are
// installed with, keeping its address and hub.
func (h *Handler) UpgradeGateway(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	gateway, err := loadGateway(cs.Name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the gateway is not installed by this platform"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	loadedChart, err := helmutil.LoadLocalChart(gatewayChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	var metrics map[string]any
	if err == nil {
		metrics, err = gatewayMetricsValues(ctx, cs)
	}
	var storageClass string
	if err == nil {
		storageClass, err = redisStorageClass(ctx, cs)
	}
	if errors.Is(err, errNoDefaultStorageClass) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Saving the gateway below fills in its password, so note this first.
	hadRedisPassword := gateway.RedisPassword != ""
	upgraded := *gateway
	for _, secret := range []*model.SecretString{&upgraded.QuotaAdminKey, &upgraded.RedisPassword} {
		if err == nil && *secret == "" {
			var generated string
			generated, err = newAPIKey()
			*secret = model.SecretString(generated)
		}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, gatewayNamespace)
	if err == nil {
		err = helmutil.UpgradePlatformRelease(ctx, cfg, cs.Name, c.MustGet("user").(model.User).ID, gatewayReleaseName, loadedChart, gatewayValues(&upgraded, metrics, storageClass), helmutil.UpgradeReleaseOptions{
			Namespace:   gatewayNamespace,
			Timeout:     lwsActionTimeout,
			Description: "Upgraded by MPlatform model serving",
		})
	}
	// The key marks the gateway as having Redis, so it is saved only now.
	if err == nil {
		err = model.DB.Model(gateway).Updates(map[string]any{"quota_admin_key": upgraded.QuotaAdminKey, "redis_password": upgraded.RedisPassword}).Error
	}
	// Redis only reads a new password when it starts, and the chart does not
	// restart it.
	if err == nil && !hadRedisPassword {
		redis := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: gatewayNamespace, Name: redisService + "-0"}}
		err = client.IgnoreNotFound(cs.K8sClient.Delete(ctx, redis))
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := syncNewGateway(ctx, cs); err != nil {
		klog.Errorf("Failed to configure the upgraded model gateway in cluster %s: %v", cs.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "the gateway is upgraded, but configuring its plugins failed; resync the gateway: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway upgraded"})
}
