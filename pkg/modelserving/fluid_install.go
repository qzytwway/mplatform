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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Fluid is installed the way its documentation does: release fluid of chart
// fluid in namespace fluid-system. Its Datasets serve models that inference
// services in other namespaces reach through references to them.
const (
	fluidChartName     = "fluid"
	fluidReleaseName   = "fluid"
	fluidNamespace     = "fluid-system"
	fluidActionTimeout = 5 * time.Minute
	fluidDatasetCRD    = "datasets.data.fluid.io"
	testedFluidVersion = "1.0.8"
)

var datasetGVK = schema.GroupVersionKind{Group: "data.fluid.io", Version: "v1alpha1", Kind: "Dataset"}

// fluidValues installs the chart with its defaults; the runtime images it
// hands its controllers follow the cluster image registry like the others.
func fluidValues(*chart.Chart) map[string]any {
	return map[string]any{}
}

// listDatasets returns the Fluid Datasets of a namespace, or of every
// namespace when it is empty; none when Fluid is not installed.
func listDatasets(ctx context.Context, cs *cluster.ClientSet, namespace string) ([]unstructured.Unstructured, error) {
	installed, err := crdInstalled(ctx, cs, fluidDatasetCRD)
	if err != nil || !installed {
		return nil, err
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(datasetGVK.GroupVersion().WithKind(datasetGVK.Kind + "List"))
	if err := cs.K8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (h *Handler) GetFluid(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	crd, err := crdInstalled(ctx, cs, fluidDatasetCRD)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, fluidNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	installed, err := helmutil.ReleaseExists(cfg, fluidReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	chart, err := componentChartStatus(ctx, fluidChartName, fluidReleaseName, fluidNamespace, testedFluidVersion, cs.Version, fluidValues)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"crd": crd, "installed": installed, "chart": chart})
}

func (h *Handler) InstallFluid(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loadedChart, err := helmutil.LoadLocalChart(fluidChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, fluidNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	uninstallOpts := helmutil.UninstallReleaseOptions{Timeout: fluidActionTimeout}
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, fluidReleaseName, fluidNamespace, uninstallOpts); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	err = helmutil.InstallPlatformRelease(c.Request.Context(), cfg, cs.Name, operatorID, loadedChart, fluidValues(loadedChart), helmutil.InstallReleaseOptions{
		ReleaseName:     fluidReleaseName,
		Namespace:       fluidNamespace,
		Timeout:         fluidActionTimeout,
		Description:     "Installed by MPlatform model serving",
		CreateNamespace: true,
	})
	if err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Fluid installed"})
}

// UninstallFluid refuses while Datasets exist: their volumes stop working
// once the controllers that serve them are gone.
func (h *Handler) UninstallFluid(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, fluidNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	installed, err := helmutil.ReleaseExists(cfg, fluidReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fluid is not installed by this platform"})
		return
	}
	datasets, err := listDatasets(c.Request.Context(), cs, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(datasets) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "delete the Fluid Datasets in this cluster first, including those inference services reference models through"})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	opts := helmutil.UninstallReleaseOptions{Timeout: fluidActionTimeout, Description: "Uninstalled by MPlatform model serving"}
	if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, fluidReleaseName, fluidNamespace, opts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Fluid uninstalled"})
}
