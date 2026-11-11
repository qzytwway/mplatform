package modelserving

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"gorm.io/gorm"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	publishedAnnotation = "model-serving.mplatform.io/published"
	ingressSuffix       = "-gateway"
	ingressClass        = "higress"
	// Higress routes on a request header when an Ingress carries this
	// annotation prefix followed by the header name.
	exactHeaderAnnotation = "higress.io/exact-match-header-"
)

var errIngressNameTaken = errors.New("an Ingress with the gateway route's name already exists")

type publishRequest struct {
	Published *bool `json:"published"`
}

type publishedService struct {
	Namespace string
	Name      string
	Model     string
}

func isPublished(workload *unstructured.Unstructured) bool {
	return workload.GetAnnotations()[publishedAnnotation] == "true"
}

func listPublished(ctx context.Context, cs *cluster.ClientSet) ([]publishedService, error) {
	workloads, err := listWorkloads(ctx, cs, "")
	if err != nil {
		return nil, err
	}
	published := []publishedService{}
	for i := range workloads {
		if isPublished(&workloads[i]) {
			published = append(published, publishedService{
				Namespace: workloads[i].GetNamespace(),
				Name:      workloads[i].GetName(),
				Model:     workloads[i].GetAnnotations()[modelAnnotation],
			})
		}
	}
	return published, nil
}

func (h *Handler) PublishService(c *gin.Context) {
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Published == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "published is required"})
		return
	}
	cs, workload, crd, ok := h.loadService(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if status, err := checkPublishable(ctx, cs, workload, *req.Published); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	change := unpublishService
	if *req.Published {
		change = publishService
	}
	updated, err := change(ctx, cs, workload)
	recordHistory(c, cs, crd, "update", workload, updated, err)
	if errors.Is(err, errIngressNameTaken) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Publishing has synced already, as part of what it rolls back on failure.
	if !*req.Published && !syncGatewayIfInstalled(c, cs) {
		return
	}
	service, err := describeWorkload(ctx, cs, updated)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, service)
}

// checkPublishable returns the response status and error when a service
// cannot be published: no gateway, or its model is already routed elsewhere.
func checkPublishable(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured, publish bool) (int, error) {
	if !publish {
		return http.StatusOK, nil
	}
	if _, err := loadGateway(cs.Name); errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusBadRequest, fmt.Errorf("install the gateway in this cluster first")
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	published, err := listPublished(ctx, cs)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	modelName := workload.GetAnnotations()[modelAnnotation]
	for _, other := range published {
		sameService := other.Namespace == workload.GetNamespace() && other.Name == workload.GetName()
		if other.Model == modelName && !sameService {
			return http.StatusConflict, fmt.Errorf("model %s is already published by service %s/%s", modelName, other.Namespace, other.Name)
		}
	}
	return http.StatusOK, nil
}

// publishService creates the service's gateway route, marks the workload and
// syncs the gateway. Until the sync lands the route is only covered by the
// gateway-wide key check, so any failure takes the route down again.
func publishService(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if err := applyIngress(ctx, cs, buildIngress(workload)); err != nil {
		return nil, err
	}
	marked, err := markPublished(ctx, cs, workload, true)
	if err == nil {
		err = syncInstalledGateway(ctx, cs)
	}
	if err == nil {
		return marked, nil
	}
	if rollbackErr := rollbackPublish(context.WithoutCancel(ctx), cs, workload, marked); rollbackErr != nil {
		klog.Errorf("Failed to roll back publishing %s/%s in cluster %s: %v", workload.GetNamespace(), workload.GetName(), cs.Name, rollbackErr)
		return nil, fmt.Errorf("publishing failed: %w; rolling it back also failed, unpublish the service to clean up: %w", err, rollbackErr)
	}
	return nil, fmt.Errorf("the service was not published: %w", err)
}

// rollbackPublish removes the route first, so nothing stays reachable even
// if a later step fails. marked is nil when the workload was never marked.
func rollbackPublish(ctx context.Context, cs *cluster.ClientSet, workload, marked *unstructured.Unstructured) error {
	if err := deleteIngress(ctx, cs, workload); err != nil {
		return err
	}
	if marked != nil {
		if _, err := markPublished(ctx, cs, marked, false); err != nil {
			return err
		}
	}
	// The route is gone either way; a stale rule for it is harmless.
	if err := syncInstalledGateway(ctx, cs); err != nil {
		klog.Warningf("Failed to resync model gateway in cluster %s after a failed publish: %v", cs.Name, err)
	}
	return nil
}

// unpublishService removes the route before the mark, so a failure never
// leaves a route that is reachable but not marked as published.
func unpublishService(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if err := deleteIngress(ctx, cs, workload); err != nil {
		return nil, err
	}
	return markPublished(ctx, cs, workload, false)
}

// markPublished returns the workload updated with or without the published
// annotation.
func markPublished(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured, published bool) (*unstructured.Unstructured, error) {
	updated := workload.DeepCopy()
	annotations := updated.GetAnnotations()
	if published {
		annotations[publishedAnnotation] = "true"
	} else {
		delete(annotations, publishedAnnotation)
	}
	updated.SetAnnotations(annotations)
	if err := cs.K8sClient.Update(ctx, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// buildIngress routes requests whose body names the service's model to the
// service; the gateway's model-router plugin copies the name into the header.
func buildIngress(workload *unstructured.Unstructured) *networkingv1.Ingress {
	name := workload.GetName()
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name + ingressSuffix,
			Namespace:   workload.GetNamespace(),
			Labels:      serviceLabels(name),
			Annotations: map[string]string{exactHeaderAnnotation + modelHeader: workload.GetAnnotations()[modelAnnotation]},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new(ingressClass),
			Rules: []networkingv1.IngressRule{{IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path:     "/",
					PathType: new(networkingv1.PathTypePrefix),
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
						Name: name,
						Port: networkingv1.ServiceBackendPort{Number: servingPort},
					}},
				}},
			}}}},
		},
	}
}

func applyIngress(ctx context.Context, cs *cluster.ClientSet, ingress *networkingv1.Ingress) error {
	err := cs.K8sClient.Create(ctx, ingress)
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	var existing networkingv1.Ingress
	if err := cs.K8sClient.Get(ctx, client.ObjectKeyFromObject(ingress), &existing); err != nil {
		return err
	}
	if existing.Labels[serviceLabel] != ingress.Labels[serviceLabel] {
		return errIngressNameTaken
	}
	existing.Annotations = ingress.Annotations
	existing.Spec = ingress.Spec
	return cs.K8sClient.Update(ctx, &existing)
}

func deleteIngress(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) error {
	// An unstructured read skips the informer cache, which may not have seen
	// a route that was only just created.
	ingress := &unstructured.Unstructured{}
	ingress.SetGroupVersionKind(networkingv1.SchemeGroupVersion.WithKind("Ingress"))
	key := client.ObjectKey{Namespace: workload.GetNamespace(), Name: workload.GetName() + ingressSuffix}
	if err := cs.K8sClient.Get(ctx, key, ingress); err != nil {
		return client.IgnoreNotFound(err)
	}
	if ingress.GetLabels()[serviceLabel] != workload.GetName() {
		return nil
	}
	return client.IgnoreNotFound(cs.K8sClient.Delete(ctx, ingress))
}
