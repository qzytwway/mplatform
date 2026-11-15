package modelserving

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// servicePreview is what creating a service would make: the YAML of each
// object and how each role is deployed, as the service detail shows it.
type servicePreview struct {
	Resources  []previewResource `json:"resources"`
	Deployment *deploymentConfig `json:"deployment"`
}

// previewResource is a file of the YAML tree the Helm dry run also shows.
type previewResource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PreviewService renders the objects CreateService would create from the same
// request. Only the PodMonitor needs the cluster; whether the cluster can run
// the service is left to creating it.
func (h *Handler) PreviewService(c *gin.Context) {
	req, served, storage, status, err := parseCreate(c)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	objects, workload, err := buildServiceObjects(&req, served, storage)
	if err == nil {
		objects, err = withPodMonitor(c.Request.Context(), cs, &req, objects)
	}
	if err != nil {
		c.JSON(createErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	var router *appsv1.Deployment
	for _, object := range objects {
		if deployment, ok := object.(*appsv1.Deployment); ok {
			router = deployment
		}
	}
	config, err := readDeploymentConfig(workload, router)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resources, err := renderObjects(cs.K8sClient, objects)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servicePreview{Resources: resources, Deployment: config})
}

// renderObjects writes each object as YAML at <namespace>/<kind>/<name>.yaml.
// The storage Secret holds the object storage credentials, which are masked.
func renderObjects(k8s client.Client, objects []client.Object) ([]previewResource, error) {
	resources := make([]previewResource, 0, len(objects))
	for _, object := range objects {
		if secret, ok := object.(*corev1.Secret); ok {
			masked := secret.DeepCopy()
			for key := range masked.StringData {
				masked.StringData[key] = maskedValue
			}
			object = masked
		}
		// Typed objects are built without their kind.
		gvk, err := k8s.GroupVersionKindFor(object)
		if err != nil {
			return nil, err
		}
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object.DeepCopyObject())
		if err != nil {
			return nil, err
		}
		rendered := &unstructured.Unstructured{Object: content}
		rendered.SetGroupVersionKind(gvk)
		// Neither is set on objects that do not exist yet.
		unstructured.RemoveNestedField(rendered.Object, "metadata", "creationTimestamp")
		unstructured.RemoveNestedField(rendered.Object, "status")
		data, err := yaml.Marshal(rendered.Object)
		if err != nil {
			return nil, err
		}
		resources = append(resources, previewResource{
			Path:    fmt.Sprintf("%s/%s/%s.yaml", object.GetNamespace(), gvk.Kind, object.GetName()),
			Content: string(data),
		})
	}
	return resources, nil
}
