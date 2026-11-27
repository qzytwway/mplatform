package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	testLWSGVK              = schema.GroupVersionKind{Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet"}
	testDisaggregatedSetGVK = schema.GroupVersionKind{Group: "disaggregatedset.x-k8s.io", Version: "v1", Kind: "DisaggregatedSet"}
)

func adminUser() model.User {
	return model.User{Username: "root", Roles: []common.Role{{Name: model.DefaultAdminRole.Name}}}
}

func readOnlyUser() model.User {
	return model.User{Username: "alice", Roles: []common.Role{{
		Name: "viewer", Clusters: []string{"*"}, Namespaces: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "log"},
	}}}
}

func toolContext(user model.User) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
	c.Set("user", user)
	return c
}

// inferenceServiceCluster is a fake cluster with the LeaderWorkerSet and
// DisaggregatedSet CRDs installed. Its API server serves the events matching
// a request's field selector, recorded in eventSelectors, or fails with
// eventsStatus when set, and the pod logs in logs, keyed
// <pod>[/<container>][/previous]. reads counts every request to either client.
type inferenceServiceCluster struct {
	cs             *cluster.ClientSet
	mu             sync.Mutex
	reads          int
	events         []corev1.Event
	eventsStatus   int
	eventSelectors []string
	logs           map[string]string
	podGetErr      error
}

func newInferenceServiceCluster(t *testing.T, objects ...client.Object) *inferenceServiceCluster {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{appsv1.AddToScheme, corev1.AddToScheme, apiextensionsv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range []schema.GroupVersionKind{testLWSGVK, testDisaggregatedSetGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Pod"), meta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Node"), meta.RESTScopeRoot)
	for _, kind := range []string{"Deployment", "ReplicaSet", "StatefulSet"} {
		mapper.Add(appsv1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	mapper.Add(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), meta.RESTScopeRoot)
	objects = append(objects,
		&apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "leaderworkersets.leaderworkerset.x-k8s.io"}},
		&apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "disaggregatedsets.disaggregatedset.x-k8s.io"}},
	)

	fc := &inferenceServiceCluster{logs: map[string]string{}}
	count := func() {
		fc.mu.Lock()
		defer fc.mu.Unlock()
		fc.reads++
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				count()
				if _, isPod := obj.(*corev1.Pod); isPod && fc.podGetErr != nil {
					return fc.podGetErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				count()
				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count()
		if pod, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/default/pods/"), "/log"); ok {
			if container := r.URL.Query().Get("container"); container != "" {
				pod += "/" + container
			}
			if r.URL.Query().Get("previous") == "true" {
				pod += "/previous"
			}
			logs, found := fc.logs[pod]
			if !found {
				http.Error(w, `container "engine" in pod "`+pod+`" is waiting to start`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(logs))
			return
		}
		if r.URL.Path == "/api/v1/namespaces/default/events" {
			fc.serveEvents(t, w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	fc.cs = &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient, ClientSet: clientSet}}
	return fc
}

func (fc *inferenceServiceCluster) serveEvents(t *testing.T, w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("fieldSelector")
	selector, err := fields.ParseSelector(query)
	if err != nil {
		t.Errorf("events field selector %q: %v", query, err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.eventSelectors = append(fc.eventSelectors, query)
	if fc.eventsStatus != 0 {
		http.Error(w, `events is forbidden: User "mplatform" cannot list resource "events"`, fc.eventsStatus)
		return
	}
	items := []corev1.Event{}
	for _, event := range fc.events {
		if selector.Matches(fields.Set{"involvedObject.kind": event.InvolvedObject.Kind, "involvedObject.name": event.InvolvedObject.Name}) {
			items = append(items, event)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(corev1.EventList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "EventList"}, Items: items}); err != nil {
		t.Errorf("encoding events: %v", err)
	}
}

func TestInferenceServiceToolsAreAdminOnly(t *testing.T) {
	fc := newInferenceServiceCluster(t)
	tests := []struct {
		toolName string
		args     map[string]interface{}
	}{
		{toolName: "list_inference_services", args: map[string]interface{}{}},
		{toolName: "describe_inference_service", args: map[string]interface{}{"namespace": "default", "name": "qwen"}},
		{toolName: "get_inference_service_gpus", args: map[string]interface{}{"namespace": "default", "name": "qwen"}},
	}
	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			result, isError := ExecuteTool(context.Background(), toolContext(readOnlyUser()), fc.cs, tc.toolName, tc.args)

			if !isError || !strings.HasPrefix(result, "Forbidden:") {
				t.Fatalf("want a Forbidden result, got isError=%v result=%q", isError, result)
			}
			if fc.reads != 0 {
				t.Fatalf("a non-admin call read the cluster %d times", fc.reads)
			}
		})
	}
}

func lwsService(name, mode string, size, replicas, ready int64) *unstructured.Unstructured {
	workload := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": "default",
			"labels":    map[string]any{"app.kubernetes.io/managed-by": "mplatform-model-serving"},
			"annotations": map[string]any{
				"model-serving.mplatform.io/model":   "qwen3-8b",
				"model-serving.mplatform.io/runtime": "vllm",
				"model-serving.mplatform.io/mode":    mode,
			},
		},
		"spec": map[string]any{
			"replicas": replicas,
			"leaderWorkerTemplate": map[string]any{
				"size": size,
				"workerTemplate": map[string]any{"spec": map[string]any{"containers": []any{
					map[string]any{"name": "engine", "image": "vllm/vllm-openai:v0.11.0"},
				}}},
			},
		},
		"status": map[string]any{"readyReplicas": ready},
	}}
	workload.SetGroupVersionKind(testLWSGVK)
	return workload
}

func servicePod(name, service, group, worker string, ready bool, restarts int32) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{
			"model-serving.mplatform.io/service":       service,
			"leaderworkerset.sigs.k8s.io/group-index":  group,
			"leaderworkerset.sigs.k8s.io/worker-index": worker,
		}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "engine", RestartCount: restarts}},
		},
	}
}

func podEventFor(pod, reason, message string) corev1.Event {
	return corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: pod + "." + reason, Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: pod},
		Type:           corev1.EventTypeWarning,
		Reason:         reason,
		Message:        message,
		Count:          4,
	}
}

func describeInferenceService(t *testing.T, fc *inferenceServiceCluster, name string) map[string]any {
	t.Helper()
	result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs, "describe_inference_service",
		map[string]interface{}{"namespace": "default", "name": name})
	if isError {
		t.Fatalf("describe_inference_service failed: %s", result)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, result)
	}
	return got
}

func TestDescribeSingleNodeInferenceService(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 2, 1),
		servicePod("qwen-0", "qwen", "0", "0", false, 3),
		servicePod("qwen-1", "qwen", "1", "0", true, 0),
	)
	fc.events = []corev1.Event{
		podEventFor("qwen-0", "BackOff", "Back-off restarting failed container engine"),
		podEventFor("nginx-0", "BackOff", "Back-off restarting failed container nginx"),
	}
	fc.logs["qwen-0/previous"] = "INFO Loading weights\ntorch.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB\nINFO shutting down\n"
	// A ready pod's logs are not read.
	fc.logs["qwen-1"] = "ERROR should not be reported\n"

	got := describeInferenceService(t, fc, "qwen")

	assertJSON(t, got, "model", "qwen3-8b")
	assertJSON(t, got, "runtime", "vllm")
	assertJSON(t, got, "mode", "single")
	assertJSON(t, got, "roles", []any{map[string]any{"name": "default", "replicas": 2.0, "readyReplicas": 1.0, "ready": false}})
	assertJSON(t, got, "events", map[string]any{"qwen-0": []any{map[string]any{
		"type": "Warning", "reason": "BackOff", "message": "Back-off restarting failed container engine", "count": 4.0,
	}}})
	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-0": []any{
		"torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB",
	}})
	pods, _ := got["pods"].([]any)
	if len(pods) != 2 {
		t.Fatalf("want 2 pods, got %v", got["pods"])
	}
}

func TestDescribeMultiNodeInferenceService(t *testing.T) {
	worker := servicePod("llama-0-1", "llama", "0", "1", false, 0)
	worker.Status.Phase = corev1.PodPending
	fc := newInferenceServiceCluster(t,
		lwsService("llama", "multi", 2, 1, 0),
		servicePod("llama-0", "llama", "0", "0", false, 0),
		worker,
	)
	fc.events = []corev1.Event{podEventFor("llama-0-1", "FailedScheduling", "0/3 nodes are available: 3 Insufficient nvidia.com/gpu.")}
	fc.logs["llama-0"] = "INFO waiting for workers\nRuntimeError: NCCL error: unhandled system error\n"

	got := describeInferenceService(t, fc, "llama")

	assertJSON(t, got, "mode", "multi")
	assertJSON(t, got, "groupSize", 2.0)
	assertJSON(t, got, "roles", []any{map[string]any{"name": "default", "replicas": 1.0, "readyReplicas": 0.0, "ready": false}})
	pods, _ := got["pods"].([]any)
	var roles []any
	for _, pod := range pods {
		roles = append(roles, pod.(map[string]any)["role"])
	}
	if !reflect.DeepEqual(roles, []any{"leader", "worker"}) {
		t.Fatalf("want a Leader pod and a Worker pod, got roles %v", roles)
	}
	assertJSON(t, got, "events", map[string]any{"llama-0-1": []any{map[string]any{
		"type": "Warning", "reason": "FailedScheduling", "message": "0/3 nodes are available: 3 Insufficient nvidia.com/gpu.", "count": 4.0,
	}}})
	logErrors, _ := got["engineLogErrors"].(map[string]any)
	if !reflect.DeepEqual(logErrors["llama-0"], []any{"RuntimeError: NCCL error: unhandled system error"}) {
		t.Fatalf("leader log errors: %#v", logErrors["llama-0"])
	}
	// The pending worker has no log yet, which is reported, not fatal.
	if lines, _ := logErrors["llama-0-1"].([]any); len(lines) != 1 || !strings.HasPrefix(lines[0].(string), "logs unavailable: ") {
		t.Fatalf("worker log errors: %#v", logErrors["llama-0-1"])
	}
}

func pdService() []client.Object {
	role := func(name string, replicas int64) map[string]any {
		return map[string]any{"name": name, "spec": map[string]any{
			"replicas":             replicas,
			"leaderWorkerTemplate": map[string]any{"size": int64(1)},
		}}
	}
	workload := lwsService("qwen-pd", "pd", 1, 0, 0)
	workload.SetGroupVersionKind(testDisaggregatedSetGVK)
	workload.Object["spec"] = map[string]any{"roles": []any{role("prefill", 1), role("decode", 2)}}
	child := func(role string, ready int64) *unstructured.Unstructured {
		lws := lwsService("qwen-pd-"+role, "", 1, 0, ready)
		lws.SetLabels(map[string]string{"disaggregatedset.x-k8s.io/name": "qwen-pd", "disaggregatedset.x-k8s.io/role": role})
		return lws
	}
	router := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-pd-router", Namespace: "default", Labels: map[string]string{"model-serving.mplatform.io/service": "qwen-pd"}},
		Spec:       appsv1.DeploymentSpec{Replicas: new(int32(1))},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
	pod := func(name, role string, ready bool) *corev1.Pod {
		p := servicePod(name, "qwen-pd", "0", "0", ready, 0)
		p.Labels["model-serving.mplatform.io/role"] = role
		return p
	}
	return []client.Object{
		workload, child("prefill", 1), child("decode", 1), router,
		pod("qwen-pd-prefill-0", "prefill", true),
		pod("qwen-pd-decode-0", "decode", true),
		pod("qwen-pd-decode-1", "decode", false),
	}
}

func TestDescribePDInferenceService(t *testing.T) {
	fc := newInferenceServiceCluster(t, pdService()...)
	fc.logs["qwen-pd-decode-1"] = "ERROR Mooncake transfer engine: failed to connect to prefill\n"

	got := describeInferenceService(t, fc, "qwen-pd")

	assertJSON(t, got, "mode", "pd")
	roles, _ := got["roles"].([]any)
	var readiness []string
	for _, item := range roles {
		role := item.(map[string]any)
		readiness = append(readiness, fmt.Sprintf("%s %v/%v", role["name"], role["readyReplicas"], role["replicas"]))
	}
	if want := []string{"prefill 1/1", "decode 1/2", "router 1/1"}; !reflect.DeepEqual(readiness, want) {
		t.Fatalf("role readiness: want %v, got %v", want, readiness)
	}
	if pods, _ := got["pods"].([]any); len(pods) != 3 {
		t.Fatalf("want 3 pods, got %v", got["pods"])
	}
	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-pd-decode-1": []any{
		"ERROR Mooncake transfer engine: failed to connect to prefill",
	}})
}

func TestListInferenceServices(t *testing.T) {
	objects := append(pdService(), lwsService("qwen", "single", 1, 2, 1))
	fc := newInferenceServiceCluster(t, objects...)

	result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs, "list_inference_services", map[string]interface{}{})

	if isError {
		t.Fatalf("list_inference_services failed: %s", result)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, result)
	}
	var services []string
	for _, service := range got {
		services = append(services, fmt.Sprintf("%s/%s %s ready=%v", service["namespace"], service["name"], service["mode"], service["ready"]))
	}
	if want := []string{"default/qwen single ready=false", "default/qwen-pd pd ready=false"}; !reflect.DeepEqual(services, want) {
		t.Fatalf("want %v, got %v", want, services)
	}
}

func TestToolDefinitionsOfferInferenceServiceTools(t *testing.T) {
	required := map[string][]string{}
	for _, def := range toolDefinitions(nil) {
		required[def.Name] = def.Required
	}
	for name, want := range map[string][]string{
		"list_inference_services":    nil,
		"describe_inference_service": {"namespace", "name"},
		"get_inference_service_gpus": {"namespace", "name"},
	} {
		got, ok := required[name]
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: want offered with required %v, got offered=%v required %v", name, want, ok, got)
		}
	}
}

func assertJSON(t *testing.T, got map[string]any, key string, want any) {
	t.Helper()
	if !reflect.DeepEqual(got[key], want) {
		t.Fatalf("%s:\nwant %#v\ngot  %#v", key, want, got[key])
	}
}

func TestDescribeMissingInferenceService(t *testing.T) {
	fc := newInferenceServiceCluster(t)

	result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs, "describe_inference_service",
		map[string]interface{}{"namespace": "default", "name": "qwen"})

	if !isError || result != "Error: inference service default/qwen not found" {
		t.Fatalf("want a not found error, got isError=%v result=%q", isError, result)
	}
}
