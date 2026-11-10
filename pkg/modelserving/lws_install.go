package modelserving

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// LeaderWorkerSet is installed the way its documentation does: release lws of
// chart lws in namespace lws-system.
const (
	lwsChartName     = "lws"
	lwsReleaseName   = "lws"
	lwsNamespace     = "lws-system"
	lwsActionTimeout = 5 * time.Minute
)

// chartStatus reports whether the LeaderWorkerSet chart is in the local chart
// repository, and the image an offline registry must provide for it before
// the cluster image registry rewrite is applied.
type chartStatus struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
}

func lwsChartStatus() (chartStatus, error) {
	status := chartStatus{Name: lwsChartName}
	loadedChart, err := helmutil.LoadLocalChart(lwsChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.Version = loadedChart.Metadata.Version
	repository, _, _ := unstructured.NestedString(loadedChart.Values, "image", "manager", "repository")
	tag, _, _ := unstructured.NestedString(loadedChart.Values, "image", "manager", "tag")
	if repository != "" && tag != "" {
		status.Image = repository + ":" + tag
	}
	return status, nil
}

func (h *Handler) GetStatus(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lws, err := crdInstalled(c.Request.Context(), cs, lwsCRD)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	disaggregatedSet, err := crdInstalled(c.Request.Context(), cs, disaggregatedSetCRD)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, lwsNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	installed, err := helmutil.ReleaseExists(cfg, lwsReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	chart, err := lwsChartStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"lws": lws, "disaggregatedSet": disaggregatedSet, "installed": installed, "chart": chart})
}

func (h *Handler) InstallLWS(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loadedChart, err := helmutil.LoadLocalChart(lwsChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, lwsNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	uninstallOpts := helmutil.UninstallReleaseOptions{Timeout: lwsActionTimeout}
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, lwsReleaseName, lwsNamespace, uninstallOpts); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	// The CRD and controller of DisaggregatedSet always ship with the chart;
	// this adds its validating webhook.
	values := map[string]any{"enableDisaggregatedSet": true}
	err = helmutil.InstallPlatformRelease(ctx, cfg, cs.Name, operatorID, loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName:     lwsReleaseName,
		Namespace:       lwsNamespace,
		Timeout:         lwsActionTimeout,
		Description:     "Installed by MPlatform model serving",
		CreateNamespace: true,
	})
	if err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "LeaderWorkerSet installed"})
}

func installErrorStatus(err error) int {
	if errors.Is(err, helmutil.ErrReleaseAlreadyInstalled) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func (h *Handler) UninstallLWS(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, lwsNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	installed, err := helmutil.ReleaseExists(cfg, lwsReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": "LeaderWorkerSet is not installed by this platform"})
		return
	}
	services, err := listServices(c.Request.Context(), cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(services) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "delete the inference services in this cluster first"})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	opts := helmutil.UninstallReleaseOptions{Timeout: lwsActionTimeout, Description: "Uninstalled by MPlatform model serving"}
	if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, lwsReleaseName, lwsNamespace, opts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "LeaderWorkerSet uninstalled"})
}
