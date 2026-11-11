package modelserving

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// The GPU Operator provides the driver, container toolkit, GPU Feature
// Discovery and DCGM on every GPU node; HAMi shares the GPUs of the nodes in
// vGPU mode. Both are installed from the local chart directory.
const gpuComponentActionTimeout = 5 * time.Minute

var (
	errGPUOperatorMissing = errors.New("install the GPU Operator first")
	errHAMiMissing        = errors.New("install HAMi first")
)

type gpuComponent struct {
	Installed bool        `json:"installed"`
	Chart     chartStatus `json:"chart"`
}

func releaseInstalled(cs *cluster.ClientSet, namespace, name string) (bool, error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, namespace)
	if err != nil {
		return false, err
	}
	return helmutil.ReleaseExists(cfg, name)
}

func (h *Handler) GetGPUComponents(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	gpuOperator := gpuComponent{}
	gpuOperator.Installed, err = releaseInstalled(cs, gpuOperatorNamespace, gpuOperatorReleaseName)
	if err == nil {
		gpuOperator.Chart, err = gpuOperatorChartStatus()
	}
	hami := gpuComponent{}
	if err == nil {
		hami.Installed, err = releaseInstalled(cs, hamiNamespace, hamiReleaseName)
	}
	if err == nil {
		hami.Chart, err = hamiChartStatus(cs.Version)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"gpuOperator": gpuOperator, "hami": hami})
}

// loadComponentChart writes the error response when the chart cannot be
// loaded from the local chart directory.
func loadComponentChart(c *gin.Context, name string) (*chart.Chart, bool) {
	loadedChart, err := helmutil.LoadLocalChart(name)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return loadedChart, true
}

func installComponent(c *gin.Context, cs *cluster.ClientSet, loadedChart *chart.Chart, releaseName, namespace string, values map[string]any, message string) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	uninstallOpts := helmutil.UninstallReleaseOptions{Timeout: gpuComponentActionTimeout}
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, releaseName, namespace, uninstallOpts); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	err = helmutil.InstallPlatformRelease(c.Request.Context(), cfg, cs.Name, operatorID, loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName:     releaseName,
		Namespace:       namespace,
		Timeout:         gpuComponentActionTimeout,
		Description:     "Installed by MPlatform model serving",
		CreateNamespace: true,
	})
	if err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": message})
}

func uninstallComponent(c *gin.Context, cs *cluster.ClientSet, releaseName, namespace, message string) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	opts := helmutil.UninstallReleaseOptions{Timeout: gpuComponentActionTimeout, Description: "Uninstalled by MPlatform model serving"}
	if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, releaseName, namespace, opts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}
