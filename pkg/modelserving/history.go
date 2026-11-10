package modelserving

import (
	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// recordHistory audits a change to a service's workload under its CRD name,
// like the custom resource handler does. prev or curr may be nil.
func recordHistory(c *gin.Context, cs *cluster.ClientSet, crd, opType string, prev, curr *unstructured.Unstructured, opErr error) {
	named := curr
	if named == nil {
		named = prev
	}
	history := model.ResourceHistory{
		ClusterName:   cs.Name,
		ResourceType:  crd,
		ResourceName:  named.GetName(),
		Namespace:     named.GetNamespace(),
		OperationType: opType,
		ResourceYAML:  workloadYAML(curr),
		PreviousYAML:  workloadYAML(prev),
		Success:       opErr == nil,
		OperatorID:    c.MustGet("user").(model.User).ID,
	}
	if opErr != nil {
		history.ErrorMessage = opErr.Error()
	}
	if err := model.DB.Create(&history).Error; err != nil {
		klog.Errorf("Failed to create resource history: %v", err)
	}
}

func workloadYAML(workload *unstructured.Unstructured) string {
	if workload == nil {
		return ""
	}
	copied := workload.DeepCopy()
	copied.SetManagedFields(nil)
	yamlBytes, err := yaml.Marshal(copied)
	if err != nil {
		return ""
	}
	return string(yamlBytes)
}
