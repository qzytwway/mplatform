package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newToolTestContext(roles ...common.Role) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
	c.Set("user", model.User{Username: "alice", Roles: roles})
	return c
}

var allPermissionsRole = common.Role{
	Name:       "admin",
	Clusters:   []string{"*"},
	Namespaces: []string{"*"},
	Resources:  []string{"*"},
	Verbs:      []string{"*"},
}

func TestExecuteToolRejectsWriteToolsWithoutTouchingCluster(t *testing.T) {
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
		Data:       map[string]string{"mode": "original"},
	}
	updatedYAML := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: default\ndata:\n  mode: changed\n"
	newYAML := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: new-config\n  namespace: default\n"
	tests := []struct {
		toolName string
		args     map[string]interface{}
	}{
		{toolName: "create_resource", args: map[string]interface{}{"yaml": newYAML}},
		{toolName: "apply_resource", args: map[string]interface{}{"yaml": updatedYAML}},
		{toolName: "update_resource", args: map[string]interface{}{"yaml": updatedYAML}},
		{toolName: "patch_resource", args: map[string]interface{}{"kind": "ConfigMap", "name": "app-config", "namespace": "default", "patch": `{"data":{"mode":"changed"}}`}},
		{toolName: "delete_resource", args: map[string]interface{}{"kind": "ConfigMap", "name": "app-config", "namespace": "default"}},
	}

	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithObjects(existing.DeepCopy()).Build()
			cs := &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient}}

			result, isError := ExecuteTool(context.Background(), newToolTestContext(allPermissionsRole), cs, tc.toolName, tc.args)
			if !isError {
				t.Fatalf("ExecuteTool(%s) succeeded, want an error; result=%q", tc.toolName, result)
			}

			var configMaps corev1.ConfigMapList
			if err := fakeClient.List(context.Background(), &configMaps, client.InNamespace("default")); err != nil {
				t.Fatalf("list config maps: %v", err)
			}
			if len(configMaps.Items) != 1 || configMaps.Items[0].Name != "app-config" {
				t.Fatalf("cluster changed: got %d config maps %v", len(configMaps.Items), configMaps.Items)
			}
			if !reflect.DeepEqual(configMaps.Items[0].Data, existing.Data) {
				t.Fatalf("config map data changed: got %v", configMaps.Items[0].Data)
			}
		})
	}
}

func TestOfferedToolsAreReadOnly(t *testing.T) {
	writeTools := map[string]bool{
		"create_resource": true,
		"apply_resource":  true,
		"update_resource": true,
		"patch_resource":  true,
		"delete_resource": true,
	}
	clusters := map[string]*cluster.ClientSet{
		"without prometheus": nil,
		"with prometheus":    {PromClient: &prometheus.Client{}},
	}

	for name, cs := range clusters {
		t.Run(name, func(t *testing.T) {
			defs := toolDefinitions(cs)
			var offered []string
			for _, tool := range OpenAIToolDefs(defs) {
				offered = append(offered, tool.Function.Name)
			}
			for _, tool := range AnthropicToolDefs(defs) {
				offered = append(offered, tool.OfTool.Name)
			}
			for _, toolName := range offered {
				if writeTools[toolName] {
					t.Errorf("write tool %q is offered to the model", toolName)
				}
			}
		})
	}
}

func TestExecInPodNeedsConfirmationAndExecPermission(t *testing.T) {
	if !MutationTools["exec_in_pod"] {
		t.Fatalf("exec_in_pod must wait for user confirmation")
	}

	args := map[string]interface{}{"name": "api", "namespace": "default", "command": []interface{}{"ls"}, "timeout_seconds": float64(10)}
	podRole := func(verbs ...string) common.Role {
		return common.Role{Name: "pods", Clusters: []string{"*"}, Namespaces: []string{"default"}, Resources: []string{"pods"}, Verbs: verbs}
	}
	tests := []struct {
		name       string
		role       common.Role
		wantResult string
		wantError  bool
	}{
		{name: "without exec permission", role: podRole("get", "log"), wantResult: "Forbidden:", wantError: true},
		{name: "with exec permission", role: podRole("exec")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, isError := AuthorizeTool(newToolTestContext(tc.role), &cluster.ClientSet{Name: "test"}, "exec_in_pod", args)
			if isError != tc.wantError || !strings.HasPrefix(result, tc.wantResult) {
				t.Fatalf("AuthorizeTool() = (%q, %v), want prefix %q and error %v", result, isError, tc.wantResult, tc.wantError)
			}
		})
	}
}
