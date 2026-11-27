package modelserving

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// LeaderWorkerSet is installed the way its documentation does: release lws of
// chart lws in namespace lws-system.
const (
	lwsChartName     = "lws"
	lwsReleaseName   = "lws"
	lwsNamespace     = "lws-system"
	lwsActionTimeout = 5 * time.Minute
)

// The chart versions the platform is verified with; the README lists them
// too.
const (
	testedLWSVersion         = "v1.0.0"
	testedGPUOperatorVersion = "v26.7.2-mplatform.1"
	testedHAMiVersion        = "2.11.0"
	testedGatewayVersion     = "2.2.6"
)

// chartStatus reports whether a chart model serving installs is in the local
// chart repository, the version the platform is verified with, and the images
// an offline registry must provide for it before the cluster image registry
// rewrite is applied, or why they cannot be listed.
type chartStatus struct {
	Name          string   `json:"name"`
	Version       string   `json:"version,omitempty"`
	TestedVersion string   `json:"testedVersion,omitempty"`
	Images        []string `json:"images,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// componentChartStatus lists the images a chart deploys in a cluster of
// kubeVersion, rendered with the values the platform installs it with.
func componentChartStatus(ctx context.Context, name, release, namespace, testedVersion, kubeVersion string, values func(*chart.Chart) map[string]any) (chartStatus, error) {
	status := chartStatus{Name: name, TestedVersion: testedVersion}
	loadedChart, err := helmutil.LoadLocalChart(name)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.Version = loadedChart.Metadata.Version
	status.Images, err = helmutil.ChartImages(ctx, loadedChart, values(loadedChart), helmutil.ChartImagesOptions{ReleaseName: release, Namespace: namespace, KubeVersion: kubeVersion})
	if err != nil {
		status.Error = err.Error()
	}
	return status, nil
}

// lwsValues adds the validating webhook of DisaggregatedSet, whose CRD and
// controller always ship with the chart.
func lwsValues(*chart.Chart) map[string]any {
	return map[string]any{"enableDisaggregatedSet": true}
}

func lwsChartStatus(ctx context.Context, kubeVersion string) (chartStatus, error) {
	return componentChartStatus(ctx, lwsChartName, lwsReleaseName, lwsNamespace, testedLWSVersion, kubeVersion, lwsValues)
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
	chart, err := lwsChartStatus(c.Request.Context(), cs.Version)
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
	err = helmutil.InstallPlatformRelease(ctx, cfg, cs.Name, operatorID, loadedChart, lwsValues(loadedChart), helmutil.InstallReleaseOptions{
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
	h.uninstallComponent(c, platformComponent{"LeaderWorkerSet", lwsReleaseName, lwsNamespace, lwsActionTimeout},
		func(ctx context.Context, cs *cluster.ClientSet) (string, error) {
			services, err := ListInferenceServices(ctx, cs)
			if err != nil || len(services) == 0 {
				return "", err
			}
			return "delete the inference services in this cluster first", nil
		})
}

// platformComponent is a release model serving installs into a cluster.
type platformComponent struct {
	title, releaseName, namespace string
	timeout                       time.Duration
}

// uninstallComponent uninstalls a component the platform installed, unless
// inUse names what still depends on it.
func (h *Handler) uninstallComponent(c *gin.Context, component platformComponent, inUse func(context.Context, *cluster.ClientSet) (string, error)) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, component.namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	installed, err := helmutil.ReleaseExists(cfg, component.releaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": component.title + " is not installed by this platform"})
		return
	}
	reason, err := inUse(c.Request.Context(), cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reason != "" {
		c.JSON(http.StatusConflict, gin.H{"error": reason})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	opts := helmutil.UninstallReleaseOptions{Timeout: component.timeout, Description: "Uninstalled by MPlatform model serving"}
	if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, component.releaseName, component.namespace, opts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": component.title + " uninstalled"})
}
