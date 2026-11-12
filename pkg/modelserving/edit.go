package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const notEditable = "this service was created before services could be edited; delete and recreate it to change it"

// storedRequest returns the request recorded on a workload, or nil for
// workloads created before requests were recorded.
func storedRequest(workload *unstructured.Unstructured) (*serviceRequest, error) {
	raw, ok := workload.GetAnnotations()[requestAnnotation]
	if !ok {
		return nil, nil
	}
	var req serviceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return nil, fmt.Errorf("read the recorded request of %s: %w", workload.GetName(), err)
	}
	return &req, nil
}

// keepReplicas sets the replica counts of req to the service's current ones,
// which scaling changes without editing the recorded request.
func keepReplicas(req *serviceRequest, workload *unstructured.Unstructured, router *appsv1.Deployment) {
	if req.PD == nil {
		replicas, _, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas")
		req.Replicas = int32(replicas)
		return
	}
	specs := pdRoleSpecs(workload)
	prefill, _, _ := unstructured.NestedInt64(specs[rolePrefill], "replicas")
	decode, _, _ := unstructured.NestedInt64(specs[roleDecode], "replicas")
	req.PD.Prefill.Replicas, req.PD.Decode.Replicas = int32(prefill), int32(decode)
	if router != nil && router.Spec.Replicas != nil {
		req.PD.RouterReplicas = *router.Spec.Replicas
	}
}

// GetServiceRequest returns the request the edit form of a service starts
// from. It is kept apart from the service detail, which masks sensitive
// values that the form must show.
func (h *Handler) GetServiceRequest(c *gin.Context) {
	cs, workload, _, ok := h.loadService(c)
	if !ok {
		return
	}
	req, err := storedRequest(workload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req == nil {
		c.JSON(http.StatusConflict, gin.H{"error": notEditable})
		return
	}
	var router *appsv1.Deployment
	if req.PD != nil {
		if router, err = getRouter(c.Request.Context(), cs, workload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	keepReplicas(req, workload, router)
	c.JSON(http.StatusOK, req)
}

// UpdateService renders a service again from an edited request. Its name,
// namespace, runtime, mode and model stay; replicas stay as scaled. The
// workload rolls its groups over to the new templates.
func (h *Handler) UpdateService(c *gin.Context) {
	cs, workload, crd, ok := h.loadService(c)
	if !ok {
		return
	}
	req, served, storage, status, err := parseEdit(c, workload)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	if req.VGPU != nil {
		if status, err := requireModeComponents(cs, gpuModeVGPU); err != nil {
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
	}
	if status, err := requirePriorityClass(ctx, cs, req.PriorityClassName); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	// Look the router up first so a missing one fails before anything changes.
	var router *appsv1.Deployment
	if req.PD != nil {
		if router, err = getRouter(ctx, cs, workload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if router == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the router of this service is missing"})
			return
		}
	}
	keepReplicas(&req, workload, router)
	objects, built, err := buildServiceObjects(&req, served, storage)
	if err != nil {
		c.JSON(createErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	updated, err := applyEdit(c, cs, crd, workload, router, objects, built)
	if err != nil {
		c.JSON(createErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	service, err := describeWorkload(ctx, cs, updated)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, service)
}

// parseEdit reads the edited request of a service with the model it serves,
// returning the response status for errors.
func parseEdit(c *gin.Context, workload *unstructured.Unstructured) (serviceRequest, *model.ServedModel, *model.ModelStorage, int, error) {
	stored, err := storedRequest(workload)
	if err != nil {
		return serviceRequest{}, nil, nil, http.StatusInternalServerError, err
	}
	if stored == nil {
		return serviceRequest{}, nil, nil, http.StatusConflict, errors.New(notEditable)
	}
	var body serviceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		return serviceRequest{}, nil, nil, http.StatusBadRequest, err
	}
	body.Name, body.Namespace = workload.GetName(), workload.GetNamespace()
	// Replicas stay as scaled, which UpdateService sets after parsing; any
	// count passes here, even when the service is scaled to zero.
	body.Replicas = 1
	if body.PD != nil {
		body.PD.Prefill.Replicas, body.PD.Decode.Replicas = 1, 1
	}
	req, err := parseServiceRequest(&body)
	if err != nil {
		return req, nil, nil, http.StatusBadRequest, err
	}
	if req.Runtime != stored.Runtime || req.Mode != stored.Mode || req.ModelID != stored.ModelID {
		return req, nil, nil, http.StatusBadRequest, errors.New("runtime, mode and model cannot be changed; create another service instead")
	}
	served, storage, err := loadModelSource(req.ModelID)
	if errors.Is(err, errLookupFailed) {
		return req, nil, nil, http.StatusInternalServerError, err
	} else if err != nil {
		return req, nil, nil, http.StatusBadRequest, err
	}
	// Clients and the gateway reach the service by the model's name, and the
	// storage Secret only exists for models that were in object storage.
	annotations := workload.GetAnnotations()
	if served.Name != annotations[modelAnnotation] || served.Type != annotations[modelTypeAnnotation] || served.SourceType != annotations[sourceTypeAnnotation] {
		return req, nil, nil, http.StatusConflict, fmt.Errorf("model %d was renamed, retyped or moved to another source since this service was created; create another service for it", req.ModelID)
	}
	if err := validateModelMode(&req, served); err != nil {
		return req, nil, nil, http.StatusBadRequest, err
	}
	return req, served, storage, http.StatusOK, nil
}

// applyEdit writes the objects rebuilt from an edited request: the storage
// Secret before the workload whose new pods read it, then the router.
func applyEdit(c *gin.Context, cs *cluster.ClientSet, crd string, workload *unstructured.Unstructured, router *appsv1.Deployment, objects []client.Object, built *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	ctx := c.Request.Context()
	for _, object := range objects {
		if secret, ok := object.(*corev1.Secret); ok {
			if err := refreshSecret(ctx, cs, secret); err != nil {
				return nil, err
			}
		}
	}
	// The new spec replaces the old one whole; annotations other than the
	// platform's, such as the gateway's, are kept.
	updated := workload.DeepCopy()
	updated.Object["spec"] = built.Object["spec"]
	annotations := updated.GetAnnotations()
	maps.Copy(annotations, built.GetAnnotations())
	updated.SetAnnotations(annotations)
	err := cs.K8sClient.Update(ctx, updated)
	recordHistory(c, cs, crd, "update", workload, updated, err)
	if err != nil || router == nil {
		return updated, err
	}
	// A patch, since the router read from the informer cache may be stale.
	edited := router.DeepCopy()
	for _, object := range objects {
		if deployment, ok := object.(*appsv1.Deployment); ok {
			edited.Spec.Template = deployment.Spec.Template
		}
	}
	if err := cs.K8sClient.Patch(ctx, edited, client.MergeFrom(router)); err != nil {
		return nil, fmt.Errorf("prefill and decode were updated, but updating the router failed; save again to retry: %w", err)
	}
	return updated, nil
}

// refreshSecret brings the storage Secret of a service up to the current
// object storage settings, which its downloader is rendered with.
func refreshSecret(ctx context.Context, cs *cluster.ClientSet, secret *corev1.Secret) error {
	current := &corev1.Secret{ObjectMeta: secret.ObjectMeta}
	err := cs.K8sClient.Patch(ctx, secret.DeepCopy(), client.MergeFrom(current))
	if apierrors.IsNotFound(err) {
		return cs.K8sClient.Create(ctx, secret)
	}
	return err
}
