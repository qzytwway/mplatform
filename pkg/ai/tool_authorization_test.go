package ai

import (
	"context"
	"encoding/json"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestPermissionNamespace(t *testing.T) {
	tests := []struct {
		name      string
		resource  resourceInfo
		namespace string
		want      string
	}{
		{
			name:      "cluster scoped ignores namespace",
			resource:  resourceInfo{ClusterScoped: true},
			namespace: "default",
			want:      "",
		},
		{
			name:      "namespaced empty becomes all",
			resource:  resourceInfo{},
			namespace: " ",
			want:      "_all",
		},
		{
			name:      "namespaced passes through",
			resource:  resourceInfo{},
			namespace: " default ",
			want:      "default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := permissionNamespace(tc.resource, tc.namespace); got != tc.want {
				t.Fatalf("unexpected namespace: want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRequiredToolPermissions(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     []toolPermission
	}{
		{
			name:     "get resource",
			toolName: "get_resource",
			args: map[string]interface{}{
				"kind":      "pods",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "pods", Verb: "get", Namespace: "default"}},
		},
		{
			name:     "list resources across namespaces",
			toolName: "list_resources",
			args: map[string]interface{}{
				"kind": "Deployment",
			},
			want: []toolPermission{{Resource: "deployments", Verb: "get", Namespace: "_all"}},
		},
		{
			name:     "get pod logs",
			toolName: "get_pod_logs",
			args: map[string]interface{}{
				"name":      "nginx",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "pods", Verb: "log", Namespace: "default"}},
		},
		{
			name:     "cluster overview",
			toolName: "get_cluster_overview",
			args:     map[string]interface{}{},
			want: []toolPermission{
				{Resource: "nodes", Verb: "get", Namespace: ""},
				{Resource: "pods", Verb: "get", Namespace: "_all"},
				{Resource: "namespaces", Verb: "get", Namespace: ""},
				{Resource: "services", Verb: "get", Namespace: "_all"},
			},
		},
		{
			name:     "describe cluster scoped resource",
			toolName: "describe_resource",
			args: map[string]interface{}{
				"kind":      "Node",
				"name":      "node-1",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "nodes", Verb: "get", Namespace: ""}},
		},
		{
			name:     "prometheus query",
			toolName: "query_prometheus",
			args:     map[string]interface{}{"query": "up"},
			want:     []toolPermission{{Resource: "prometheus", Verb: "get", Namespace: "_all"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requiredToolPermissions(context.Background(), &cluster.ClientSet{}, tc.toolName, tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected permissions:\nwant: %#v\ngot:  %#v", tc.want, got)
			}
		})
	}
}

func TestRequiredToolPermissionsQualifiesCustomResources(t *testing.T) {
	cs := newToolAuthorizationTestClientSet(t)
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		verb     string
	}{
		{name: "get", toolName: "get_resource", args: map[string]interface{}{"kind": "widgets.example.com", "namespace": "default"}, verb: "get"},
		{name: "list", toolName: "list_resources", args: map[string]interface{}{"kind": "widgets.example.com", "namespace": "default"}, verb: "get"},
		{name: "describe", toolName: "describe_resource", args: map[string]interface{}{"kind": "widgets.example.com", "name": "example", "namespace": "default"}, verb: "get"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requiredToolPermissions(context.Background(), cs, tc.toolName, tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := []toolPermission{{Resource: "widgets.example.com", Verb: tc.verb, Namespace: "default"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected permissions:\nwant: %#v\ngot:  %#v", want, got)
			}
		})
	}
}

func TestAuthorizeToolCustomResourceGroups(t *testing.T) {
	cs := newToolAuthorizationTestClientSet(t)
	tests := []struct {
		name              string
		resource          string
		kind              string
		wantError         bool
		wantErrorResource string
	}{
		{
			name:     "qualified custom resource permission allows",
			resource: "widgets.example.com",
			kind:     "Widget",
		},
		{
			name:              "same plural in another group is denied",
			resource:          "widgets.other.example.com",
			kind:              "Widget",
			wantError:         true,
			wantErrorResource: "widgets.example.com",
		},
		{
			name:              "builtin permission with same plural is denied",
			resource:          "pods",
			kind:              "CustomPod",
			wantError:         true,
			wantErrorResource: "pods.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
			c.Set("user", model.User{
				Username: "alice",
				Roles: []common.Role{{
					Name:       "resource-reader",
					Clusters:   []string{"cluster-a"},
					Namespaces: []string{"default"},
					Resources:  []string{tc.resource},
					Verbs:      []string{"get"},
				}},
			})

			result, isError := AuthorizeTool(c, cs, "get_resource", map[string]interface{}{"kind": tc.kind, "name": "example", "namespace": "default"})
			if isError != tc.wantError {
				t.Fatalf("AuthorizeTool() error = %v, want %v; result=%q", isError, tc.wantError, result)
			}
			if tc.wantError && (!strings.Contains(result, "Forbidden:") || !strings.Contains(result, tc.wantErrorResource)) {
				t.Fatalf("unexpected authorization error: %q", result)
			}
		})
	}
}

func newToolAuthorizationTestClientSet(t *testing.T) *cluster.ClientSet {
	t.Helper()
	resources := metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{APIVersion: "v1", Kind: "APIResourceList"},
		GroupVersion: "example.com/v1",
		APIResources: []metav1.APIResource{
			{Name: "widgets", SingularName: "widget", Namespaced: true, Kind: "Widget"},
			{Name: "pods", SingularName: "custompod", Namespaced: true, Kind: "CustomPod"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_ = json.NewEncoder(w).Encode(metav1.APIVersions{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIVersions"},
				Versions: []string{"v1"},
			})
		case "/apis":
			_ = json.NewEncoder(w).Encode(metav1.APIGroupList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIGroupList"},
				Groups: []metav1.APIGroup{{
					Name:             "example.com",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "example.com/v1", Version: "v1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.com/v1", Version: "v1"},
				}},
			})
		case "/api/v1":
			_ = json.NewEncoder(w).Encode(metav1.APIResourceList{GroupVersion: "v1"})
		case "/apis/example.com/v1":
			_ = json.NewEncoder(w).Encode(resources)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("create kubernetes client: %v", err)
	}
	return &cluster.ClientSet{
		Name:      "cluster-a",
		K8sClient: &kube.K8sClient{ClientSet: clientSet},
	}
}

func TestAuthorizeToolAllNamespacesRespectsExclusions(t *testing.T) {
	cs := newToolAuthorizationTestClientSet(t)
	excludingKubeSystem := common.Role{
		Name:       "all-but-kube-system",
		Clusters:   []string{"*"},
		Namespaces: []string{"*", "!kube-system"},
		Resources:  []string{"*"},
		Verbs:      []string{"get"},
	}
	unrestricted := common.Role{
		Name:       "all-namespaces",
		Clusters:   []string{"*"},
		Namespaces: []string{"*"},
		Resources:  []string{"*"},
		Verbs:      []string{"get"},
	}
	tests := []struct {
		name      string
		role      common.Role
		toolName  string
		args      map[string]interface{}
		wantError bool
	}{
		{name: "list in allowed namespace", role: excludingKubeSystem, toolName: "list_resources", args: map[string]interface{}{"kind": "pods", "namespace": "default"}},
		{name: "list across namespaces with exclusion", role: excludingKubeSystem, toolName: "list_resources", args: map[string]interface{}{"kind": "pods"}, wantError: true},
		{name: "prometheus with exclusion", role: excludingKubeSystem, toolName: "query_prometheus", args: map[string]interface{}{"query": "up"}, wantError: true},
		{name: "list across namespaces unrestricted", role: unrestricted, toolName: "list_resources", args: map[string]interface{}{"kind": "pods"}},
		{name: "prometheus unrestricted", role: unrestricted, toolName: "query_prometheus", args: map[string]interface{}{"query": "up"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
			c.Set("user", model.User{Username: "alice", Roles: []common.Role{tc.role}})

			result, isError := AuthorizeTool(c, cs, tc.toolName, tc.args)
			if isError != tc.wantError {
				t.Fatalf("AuthorizeTool() error = %v, want %v; result=%q", isError, tc.wantError, result)
			}
		})
	}
}
