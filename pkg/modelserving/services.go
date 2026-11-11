package modelserving

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// workloadKinds are the custom resources an inference service can be, with
// the CRD that must be installed to use them.
var workloadKinds = []struct {
	crd string
	gvk schema.GroupVersionKind
}{
	{lwsCRD, lwsGVK},
	{disaggregatedSetCRD, disaggregatedSetGVK},
}

func crdInstalled(ctx context.Context, cs *cluster.ClientSet, name string) (bool, error) {
	var crd apiextensionsv1.CustomResourceDefinition
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Name: name}, &crd)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// isService tells inference services from other workloads, including the
// LeaderWorkerSets a DisaggregatedSet creates for its roles.
func isService(workload *unstructured.Unstructured) bool {
	labels := workload.GetLabels()
	return labels[managedByLabel] == managedByValue && labels[disaggregatedSetNameLabel] == ""
}

func (h *Handler) ListServices(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, err := listServices(c.Request.Context(), cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func listServices(ctx context.Context, cs *cluster.ClientSet) ([]inferenceService, error) {
	workloads, err := listWorkloads(ctx, cs)
	if err != nil {
		return nil, err
	}
	items := make([]inferenceService, 0, len(workloads))
	for i := range workloads {
		service, err := describeWorkload(ctx, cs, &workloads[i])
		if err != nil {
			return nil, err
		}
		items = append(items, service)
	}
	return items, nil
}

// listWorkloads returns the workloads of every inference service in the
// cluster, skipping kinds whose CRD is not installed.
func listWorkloads(ctx context.Context, cs *cluster.ClientSet) ([]unstructured.Unstructured, error) {
	items := []unstructured.Unstructured{}
	for _, kind := range workloadKinds {
		installed, err := crdInstalled(ctx, cs, kind.crd)
		if err != nil {
			return nil, err
		}
		if !installed {
			continue
		}
		workloads := &unstructured.UnstructuredList{}
		workloads.SetGroupVersionKind(kind.gvk)
		if err := cs.K8sClient.List(ctx, workloads, client.MatchingLabels{managedByLabel: managedByValue}); err != nil {
			return nil, err
		}
		for i := range workloads.Items {
			if isService(&workloads.Items[i]) {
				items = append(items, workloads.Items[i])
			}
		}
	}
	return items, nil
}

// findWorkload returns the workload of an inference service and the name of
// its CRD, or nil when there is no such service.
func findWorkload(ctx context.Context, cs *cluster.ClientSet, namespace, name string) (*unstructured.Unstructured, string, error) {
	for _, kind := range workloadKinds {
		installed, err := crdInstalled(ctx, cs, kind.crd)
		if err != nil {
			return nil, "", err
		}
		if !installed {
			continue
		}
		workload := &unstructured.Unstructured{}
		workload.SetGroupVersionKind(kind.gvk)
		err = cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, workload)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, "", err
		}
		if isService(workload) {
			return workload, kind.crd, nil
		}
	}
	return nil, "", nil
}

// loadService resolves the cluster and the service named in the route,
// writing the error response when either is missing.
func (h *Handler) loadService(c *gin.Context) (*cluster.ClientSet, *unstructured.Unstructured, string, bool) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, nil, "", false
	}
	workload, crd, err := findWorkload(c.Request.Context(), cs, c.Param("namespace"), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, nil, "", false
	}
	if workload == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inference service not found"})
		return nil, nil, "", false
	}
	return cs, workload, crd, true
}

func (h *Handler) GetService(c *gin.Context) {
	cs, workload, _, ok := h.loadService(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var router *appsv1.Deployment
	var err error
	if workload.GetKind() == disaggregatedSetGVK.Kind {
		if router, err = getRouter(ctx, cs, workload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	service, err := describeService(ctx, cs, workload, router)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pods, err := listServicePods(ctx, cs, workload.GetNamespace(), workload.GetName())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	config, err := readDeploymentConfig(workload, router)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, serviceDetail{inferenceService: service, Pods: pods, Deployment: config})
}

func (h *Handler) CreateService(c *gin.Context) {
	var body serviceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req, err := parseServiceRequest(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	served, storage, err := loadModelSource(req.ModelID)
	if err == nil {
		err = validateModelMode(&req, served)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errLookupFailed) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	crd, status, err := requireWorkloadCRD(ctx, cs, req.Mode)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if req.VGPU != nil {
		if status, err := requireModeComponents(cs, gpuModeVGPU); err != nil {
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
	}
	objects, workload, err := buildServiceObjects(&req, served, storage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = createObjects(ctx, cs, objects)
	recordHistory(c, cs, crd, "create", nil, workload, err)
	if err != nil {
		c.JSON(createErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	service, err := describeWorkload(ctx, cs, workload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, service)
}

var errLookupFailed = errors.New("failed to load model source")

// loadModelSource returns the model and, for s3 models, the object storage it
// is downloaded from. Errors other than errLookupFailed are invalid requests.
func loadModelSource(modelID uint) (*model.ServedModel, *model.ModelStorage, error) {
	served, err := loadModel(modelID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("model %d not found", modelID)
	} else if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errLookupFailed, err)
	}
	if served.SourceType != sourceTypeS3 {
		return served, nil, nil
	}
	storage, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("configure object storage before serving s3 models")
	} else if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errLookupFailed, err)
	}
	return served, storage, nil
}

// requireWorkloadCRD returns the CRD a mode deploys through, or the response
// status and error when it cannot be used.
func requireWorkloadCRD(ctx context.Context, cs *cluster.ClientSet, mode string) (string, int, error) {
	crd, missing := lwsCRD, "LeaderWorkerSet is not installed in this cluster; install LWS first"
	if mode == modePD {
		crd, missing = disaggregatedSetCRD, "DisaggregatedSet is not installed in this cluster; install LWS with DisaggregatedSet enabled first"
	}
	installed, err := crdInstalled(ctx, cs, crd)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}
	if !installed {
		return "", http.StatusBadRequest, errors.New(missing)
	}
	return crd, http.StatusOK, nil
}

// buildServiceObjects returns everything a service consists of in creation
// order, the workload last so its pods find their Secret and Service.
func buildServiceObjects(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage) ([]client.Object, *unstructured.Unstructured, error) {
	objects := []client.Object{}
	if storage != nil {
		objects = append(objects, buildStorageSecret(req, storage))
	}
	objects = append(objects, buildService(req))
	build := buildLeaderWorkerSet
	if req.Mode == modePD {
		objects = append(objects, routerObjects(req)...)
		build = buildDisaggregatedSet
	}
	workload, err := build(req, served, storage)
	if err != nil {
		return nil, nil, err
	}
	return append(objects, workload), workload, nil
}

// createObjects creates objects in order and removes the ones already created
// when a later one fails, so a failed service leaves nothing behind.
func createObjects(ctx context.Context, cs *cluster.ClientSet, objects []client.Object) error {
	for i, object := range objects {
		err := cs.K8sClient.Create(ctx, object)
		if err == nil {
			continue
		}
		for _, created := range objects[:i] {
			// Clean up even when the request was cancelled mid-create.
			if deleteErr := cs.K8sClient.Delete(context.WithoutCancel(ctx), created); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				klog.Warningf("Failed to clean up %T %s/%s in cluster %s: %v", created, created.GetNamespace(), created.GetName(), cs.Name, deleteErr)
			}
		}
		return err
	}
	return nil
}

func createErrorStatus(err error) int {
	switch {
	case apierrors.IsAlreadyExists(err), apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsNotFound(err), apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) ScaleService(c *gin.Context) {
	cs, workload, crd, ok := h.loadService(c)
	if !ok {
		return
	}
	var req scaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scaled, err := scaleWorkload(workload, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	// Look the router up first so a missing one fails before anything changes.
	var router *appsv1.Deployment
	if req.RouterReplicas != nil && workload.GetKind() == disaggregatedSetGVK.Kind {
		if router, err = getRouter(ctx, cs, workload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if router == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the router of this service is missing"})
			return
		}
	}
	err = cs.K8sClient.Update(ctx, scaled)
	recordHistory(c, cs, crd, "update", workload, scaled, err)
	if err != nil {
		c.JSON(createErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	var service inferenceService
	if router == nil {
		service, err = describeWorkload(ctx, cs, scaled)
	} else {
		if router, err = scaleRouter(ctx, cs, router, *req.RouterReplicas); err != nil {
			c.JSON(createErrorStatus(err), gin.H{"error": fmt.Sprintf("prefill and decode were scaled, but scaling the router failed: %v", err)})
			return
		}
		// The patched router, since the informer cache may not have it yet.
		service, err = describeService(ctx, cs, scaled, router)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, service)
}

// scaleWorkload returns a copy of workload with the requested replica counts.
// The router of a pd service is scaled apart.
func scaleWorkload(workload *unstructured.Unstructured, req *scaleRequest) (*unstructured.Unstructured, error) {
	scaled := workload.DeepCopy()
	if workload.GetKind() != disaggregatedSetGVK.Kind {
		if req.Replicas < 1 {
			return nil, fmt.Errorf("replicas must be at least 1")
		}
		return scaled, unstructured.SetNestedField(scaled.Object, int64(req.Replicas), "spec", "replicas")
	}
	if req.PrefillReplicas < 1 || req.DecodeReplicas < 1 {
		return nil, fmt.Errorf("prefillReplicas and decodeReplicas must be at least 1")
	}
	if req.PrefillReplicas > maxPDRoleReplicas || req.DecodeReplicas > maxPDRoleReplicas {
		return nil, fmt.Errorf("prefillReplicas and decodeReplicas must be at most %d", maxPDRoleReplicas)
	}
	if req.RouterReplicas != nil && *req.RouterReplicas < 1 {
		return nil, fmt.Errorf("routerReplicas must be at least 1")
	}
	replicas := map[string]int32{rolePrefill: req.PrefillReplicas, roleDecode: req.DecodeReplicas}
	roles, _, _ := unstructured.NestedSlice(scaled.Object, "spec", "roles")
	for _, item := range roles {
		role, _ := item.(map[string]any)
		name, _, _ := unstructured.NestedString(role, "name")
		if err := unstructured.SetNestedField(role, int64(replicas[name]), "spec", "replicas"); err != nil {
			return nil, err
		}
	}
	return scaled, unstructured.SetNestedSlice(scaled.Object, roles, "spec", "roles")
}

func (h *Handler) DeleteService(c *gin.Context) {
	cs, workload, crd, ok := h.loadService(c)
	if !ok {
		return
	}
	err := deleteServiceObjects(c.Request.Context(), cs, workload)
	recordHistory(c, cs, crd, "delete", workload, nil, err)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if isPublished(workload) && !syncGatewayIfInstalled(c, cs) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "inference service deleted"})
}

// deleteServiceObjects deletes the workload with the objects created next to
// it. Same-named objects without the service label are not ours and stay.
// The workload goes last so a delete that failed halfway can be retried.
func deleteServiceObjects(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) error {
	name, namespace := workload.GetName(), workload.GetNamespace()
	companions := []struct {
		gvk  schema.GroupVersionKind
		name string
	}{
		{appsv1.SchemeGroupVersion.WithKind("Deployment"), name + routerSuffix},
		{rbacv1.SchemeGroupVersion.WithKind("RoleBinding"), name + routerSuffix},
		{rbacv1.SchemeGroupVersion.WithKind("Role"), name + routerSuffix},
		{corev1.SchemeGroupVersion.WithKind("ServiceAccount"), name + routerSuffix},
		{networkingv1.SchemeGroupVersion.WithKind("Ingress"), name + ingressSuffix},
		{corev1.SchemeGroupVersion.WithKind("Service"), name},
		{corev1.SchemeGroupVersion.WithKind("Secret"), name + storageSecretSuffix},
	}
	for _, companion := range companions {
		// Unstructured reads skip the informer cache, which may not have seen
		// a service that was only just created.
		object := &unstructured.Unstructured{}
		object.SetGroupVersionKind(companion.gvk)
		err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: companion.name}, object)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
		if object.GetLabels()[serviceLabel] != name {
			continue
		}
		if err := cs.K8sClient.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := cs.K8sClient.Delete(ctx, workload); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
