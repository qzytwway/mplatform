package modelserving

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

const (
	// higressHub is the registry Higress publishes its Wasm plugins to.
	higressHub = "higress-registry.cn-hangzhou.cr.aliyuncs.com"
	// modelHeader carries the request body's model name, which the Ingress of
	// each published service matches on.
	modelHeader = "x-higress-llm-model"

	// key-auth compares the whole header value, so credentials carry the
	// scheme callers send in the Authorization header.
	credentialScheme = "Bearer "
	// nobodyConsumer is no caller's name (it is not a DNS label): a route
	// that allows only it rejects every caller.
	nobodyConsumer = "_nobody"
	// unusedConsumer keeps the consumer list non-empty while there are no
	// callers; its credential is random and never handed out.
	unusedConsumer = "_unused"
)

var wasmPluginGVK = schema.GroupVersionKind{Group: "extensions.higress.io", Version: "v1alpha1", Kind: "WasmPlugin"}

// gatewayPlugins are the Wasm plugins the gateway pulls as OCI artifacts.
// Phase and priority are the documented ones: model-router has to set the
// model header before key-auth picks the rule of the matched route.
var gatewayPlugins = []struct {
	name, version string
	priority      int64
}{
	{"key-auth", "1.0.0", 310},
	{"model-router", "2.0.4", 900},
}

type pluginStatus struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// pluginURLs returns where the gateway of a cluster pulls each plugin from:
// hub, or the official registry when it is empty, moved under the cluster's
// image registry when one is set.
func pluginURLs(clusterName, hub string) ([]pluginStatus, error) {
	if hub == "" {
		hub = higressHub
	}
	imageRegistry, err := helmutil.ClusterImageRegistry(clusterName)
	if err != nil {
		return nil, err
	}
	plugins := make([]pluginStatus, 0, len(gatewayPlugins))
	for _, plugin := range gatewayPlugins {
		image := hub + "/plugins/" + plugin.name + ":" + plugin.version
		if imageRegistry != "" {
			image = helmutil.RewriteImageRegistry(image, imageRegistry)
		}
		plugins = append(plugins, pluginStatus{Name: plugin.name, URL: "oci://" + image})
	}
	return plugins, nil
}

// syncGateway regenerates the gateway plugins of a cluster from its callers
// and published services, and applies them.
func syncGateway(ctx context.Context, cs *cluster.ClientSet) error {
	gateway, err := loadGateway(cs.Name)
	if err != nil {
		return err
	}
	urls, err := pluginURLs(cs.Name, gateway.Hub)
	if err != nil {
		return err
	}
	published, err := listPublished(ctx, cs)
	if err != nil {
		return err
	}
	var callers []model.ModelCaller
	if err := model.DB.Where("cluster_name = ? AND enabled = ?", cs.Name, true).Order("name").Find(&callers).Error; err != nil {
		return err
	}
	keyAuth, err := keyAuthSpec(callers, published)
	if err != nil {
		return err
	}
	specs := map[string]map[string]any{
		"key-auth":     keyAuth,
		"model-router": {"defaultConfig": map[string]any{"modelToHeader": modelHeader}},
	}
	for i, plugin := range gatewayPlugins {
		spec := specs[plugin.name]
		spec["url"] = urls[i].URL
		spec["phase"] = "AUTHN"
		spec["priority"] = plugin.priority
		spec["defaultConfigDisable"] = false
		if err := applyWasmPlugin(ctx, cs, plugin.name, spec); err != nil {
			return fmt.Errorf("failed to apply gateway plugin %s: %w", plugin.name, err)
		}
	}
	return nil
}

// keyAuthSpec requires a key on every route and lets each published service
// be called only by the callers allowed for its model.
func keyAuthSpec(callers []model.ModelCaller, published []publishedService) (map[string]any, error) {
	consumers := []any{}
	for _, caller := range callers {
		consumers = append(consumers, map[string]any{"name": caller.Name, "credential": credentialScheme + string(caller.APIKey)})
	}
	if len(consumers) == 0 {
		key, err := newAPIKey()
		if err != nil {
			return nil, err
		}
		consumers = append(consumers, map[string]any{"name": unusedConsumer, "credential": credentialScheme + key})
	}
	rules := []any{}
	for _, service := range published {
		allow := []any{}
		for _, caller := range callers {
			if caller.AllowAll || containsModel(caller.Models, service.Model) {
				allow = append(allow, caller.Name)
			}
		}
		if len(allow) == 0 {
			allow = append(allow, nobodyConsumer)
		}
		rules = append(rules, map[string]any{
			"ingress":       []any{service.Namespace + "/" + service.Name + ingressSuffix},
			"configDisable": false,
			"config":        map[string]any{"allow": allow},
		})
	}
	return map[string]any{
		"defaultConfig": map[string]any{
			"global_auth": true,
			"consumers":   consumers,
			"keys":        []any{"Authorization"},
			"in_header":   true,
			"in_query":    false,
		},
		"matchRules": rules,
	}, nil
}

func containsModel(models []string, name string) bool {
	for _, allowed := range models {
		if allowed == name {
			return true
		}
	}
	return false
}

func pluginName(name string) string {
	return "mplatform-" + name
}

func applyWasmPlugin(ctx context.Context, cs *cluster.ClientSet, name string, spec map[string]any) error {
	plugin := &unstructured.Unstructured{}
	plugin.SetGroupVersionKind(wasmPluginGVK)
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: gatewayNamespace, Name: pluginName(name)}, plugin)
	if apierrors.IsNotFound(err) {
		plugin.SetNamespace(gatewayNamespace)
		plugin.SetName(pluginName(name))
		plugin.SetLabels(map[string]string{managedByLabel: managedByValue})
		plugin.Object["spec"] = spec
		return cs.K8sClient.Create(ctx, plugin)
	} else if err != nil {
		return err
	}
	plugin.Object["spec"] = spec
	return cs.K8sClient.Update(ctx, plugin)
}

func deleteWasmPlugins(ctx context.Context, cs *cluster.ClientSet) error {
	for _, gatewayPlugin := range gatewayPlugins {
		plugin := &unstructured.Unstructured{}
		plugin.SetGroupVersionKind(wasmPluginGVK)
		plugin.SetNamespace(gatewayNamespace)
		plugin.SetName(pluginName(gatewayPlugin.name))
		if err := cs.K8sClient.Delete(ctx, plugin); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func loadGateway(clusterName string) (*model.ModelGateway, error) {
	var gateway model.ModelGateway
	if err := model.DB.Where("cluster_name = ?", clusterName).First(&gateway).Error; err != nil {
		return nil, err
	}
	return &gateway, nil
}

// syncInstalledGateway syncs the cluster's gateway when the platform
// installed one, and does nothing otherwise.
func syncInstalledGateway(ctx context.Context, cs *cluster.ClientSet) error {
	if _, err := loadGateway(cs.Name); errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return syncGateway(ctx, cs)
}

// syncGatewayIfInstalled applies a saved change to the cluster's gateway. It
// writes the error response and returns false on failure.
func syncGatewayIfInstalled(c *gin.Context, cs *cluster.ClientSet) bool {
	if err := syncInstalledGateway(c.Request.Context(), cs); err != nil {
		klog.Errorf("Failed to sync model gateway in cluster %s: %v", cs.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "the change was saved, but syncing it to the gateway failed; resync the gateway: " + err.Error()})
		return false
	}
	return true
}

// SyncGateway regenerates the gateway configuration on demand, to recover
// from a sync that failed after a change was saved.
func (h *Handler) SyncGateway(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := loadGateway(cs.Name); errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the gateway is not installed by this platform"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := syncGateway(c.Request.Context(), cs); err != nil {
		klog.Errorf("Failed to sync model gateway in cluster %s: %v", cs.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway synced"})
}
