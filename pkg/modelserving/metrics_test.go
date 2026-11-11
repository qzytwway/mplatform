package modelserving

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	testModel     = &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypeLocal, Path: "/data/qwen"}
	prometheusGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "Prometheus"}
)

// testRequest returns a service request for runtime and mode; multi-node
// groups have two nodes, as does the prefill role of pd mode.
func testRequest(runtime, mode string) *serviceRequest {
	req := &serviceRequest{
		Name: "qwen", Namespace: "ai", Runtime: runtime, Image: "image", Mode: mode,
		Replicas: 1, GroupSize: 1, GPUCount: 1,
	}
	switch mode {
	case modeMulti:
		req.GroupSize = 2
	case modePD:
		req.PD = &pdRequest{
			Prefill:     pdRoleRequest{Replicas: 1, GroupSize: 2, GPUCount: 1},
			Decode:      pdRoleRequest{Replicas: 2, GroupSize: 1, GPUCount: 1},
			RouterImage: "router",
		}
	}
	return req
}

func newTestClientSet(t *testing.T, funcs interceptor.Funcs, objects ...client.Object) *cluster.ClientSet {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, gvk := range []schema.GroupVersionKind{lwsGVK, disaggregatedSetGVK, podMonitorGVK, prometheusGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(funcs).Build()
	return &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient}}
}

func podMonitorCRDObject() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: podMonitorCRD}}
}

func prometheusObject(matchLabels map[string]any) *unstructured.Unstructured {
	prometheus := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "k8s", "namespace": "monitoring"},
		"spec":     map[string]any{"podMonitorSelector": map[string]any{"matchLabels": matchLabels}},
	}}
	prometheus.SetGroupVersionKind(prometheusGVK)
	return prometheus
}

// failKind makes reads of kind fail with err, as a cluster without the
// kind's CRD does with a NoKindMatchError.
func failKind(kind string, err error) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if gvk, _ := apiutil.GVKForObject(obj, c.Scheme()); gvk.Kind == kind {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if gvk, _ := apiutil.GVKForObject(list, c.Scheme()); gvk.Kind == kind+"List" {
				return err
			}
			return c.List(ctx, list, opts...)
		},
	}
}

func noKindMatch(gvk schema.GroupVersionKind) error {
	return &apimeta.NoKindMatchError{GroupKind: gvk.GroupKind(), SearchedVersions: []string{gvk.Version}}
}

func TestRuntimeArgsEnableMetrics(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		mode    string
		pdRole  string
		worker  bool
		want    bool
	}{
		{"sglang single", runtimeSGLang, modeSingle, "", false, true},
		{"sglang multi leader", runtimeSGLang, modeMulti, "", false, true},
		{"sglang multi worker", runtimeSGLang, modeMulti, "", true, false},
		{"sglang prefill leader", runtimeSGLang, modePD, rolePrefill, false, true},
		{"sglang prefill worker", runtimeSGLang, modePD, rolePrefill, true, false},
		{"sglang decode leader", runtimeSGLang, modePD, roleDecode, false, true},
		{"vllm single", runtimeVLLM, modeSingle, "", false, false},
		{"vllm multi leader", runtimeVLLM, modeMulti, "", false, false},
		{"vllm multi worker", runtimeVLLM, modeMulti, "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testRequest(tt.runtime, tt.mode)
			args := runtimeArgs(req, testModel, tt.pdRole, tt.worker)
			want := 0
			if tt.want {
				want = 1
			}
			if got := countArg(args, "--enable-metrics"); got != want {
				t.Fatalf("--enable-metrics appears %d times in %q, want %d", got, args, want)
			}
		})
	}
}

func TestRuntimeArgsSingleNode(t *testing.T) {
	tests := []struct {
		runtime string
		want    []string
	}{
		{runtimeVLLM, []string{"/models/qwen", "--served-model-name", "qwen", "--host", "0.0.0.0", "--port", "8000"}},
		{runtimeSGLang, []string{"--model-path", "/models/qwen", "--served-model-name", "qwen", "--host", "0.0.0.0", "--port", "8000", "--enable-metrics"}},
	}
	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			got := runtimeArgs(testRequest(tt.runtime, modeSingle), testModel, "", false)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("runtimeArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func countArg(args []string, arg string) int {
	count := 0
	for _, item := range args {
		if item == arg {
			count++
		}
	}
	return count
}

func TestBuildPodMonitor(t *testing.T) {
	tests := []struct {
		name             string
		prometheusLabels map[string]string
		wantLabels       map[string]string
	}{
		{
			name:       "without Prometheus labels",
			wantLabels: map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen"},
		},
		{
			name:             "with Prometheus labels",
			prometheusLabels: map[string]string{"release": "kube-prometheus-stack", "team": "ml"},
			wantLabels:       map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "kube-prometheus-stack", "team": "ml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor, err := buildPodMonitor(testRequest(runtimeSGLang, modeSingle), tt.prometheusLabels)
			if err != nil {
				t.Fatal(err)
			}
			// Unstructured objects must hold only JSON values, or copying panics.
			monitor = monitor.DeepCopy()
			if got := monitor.GroupVersionKind(); got != podMonitorGVK {
				t.Fatalf("GVK = %v, want %v", got, podMonitorGVK)
			}
			if monitor.GetName() != "qwen" || monitor.GetNamespace() != "ai" {
				t.Fatalf("PodMonitor is %s/%s, want ai/qwen", monitor.GetNamespace(), monitor.GetName())
			}
			if got := monitor.GetLabels(); !maps.Equal(got, tt.wantLabels) {
				t.Fatalf("labels = %v, want %v", got, tt.wantLabels)
			}
			selector, _, _ := unstructured.NestedStringMap(monitor.Object, "spec", "selector", "matchLabels")
			wantSelector := map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", lwsWorkerIndexLabel: "0"}
			if !maps.Equal(selector, wantSelector) {
				t.Fatalf("selector = %v, want %v", selector, wantSelector)
			}
			endpoints, _, _ := unstructured.NestedSlice(monitor.Object, "spec", "podMetricsEndpoints")
			if len(endpoints) != 1 {
				t.Fatalf("got %d endpoints, want 1", len(endpoints))
			}
			endpoint := endpoints[0].(map[string]any)
			if endpoint["port"] != "http" || endpoint["path"] != "/metrics" {
				t.Fatalf("endpoint scrapes port %v path %v, want http /metrics", endpoint["port"], endpoint["path"])
			}
			wantRelabelings := []any{
				map[string]any{"targetLabel": "inference_service", "replacement": "qwen"},
				map[string]any{"targetLabel": "inference_role", "replacement": "default"},
				map[string]any{"sourceLabels": []any{"__meta_kubernetes_pod_label_model_serving_mplatform_io_role"}, "regex": "(.+)", "targetLabel": "inference_role"},
			}
			if got := endpoint["relabelings"]; !reflect.DeepEqual(got, wantRelabelings) {
				t.Fatalf("relabelings = %v, want %v", got, wantRelabelings)
			}
		})
	}
}

// testPod is a pod LeaderWorkerSet would run from a template, with the
// worker index label it adds.
type testPod struct {
	name   string
	labels map[string]string
	ports  []corev1.ContainerPort
	args   []string
	leader bool
}

// servicePods returns the pods of one group of every role of objects, and the
// router pod in pd mode.
func servicePods(t *testing.T, objects []client.Object) []testPod {
	t.Helper()
	var pods []testPod
	for _, object := range objects {
		switch object := object.(type) {
		case *appsv1.Deployment:
			pods = append(pods, templatePod(t, "router", object.Spec.Template, ""))
		case *unstructured.Unstructured:
			if object.GetKind() == lwsGVK.Kind {
				template, _, _ := unstructured.NestedMap(object.Object, "spec", "leaderWorkerTemplate")
				pods = append(pods, groupPods(t, "group", template)...)
			}
			if object.GetKind() == disaggregatedSetGVK.Kind {
				roles, _, _ := unstructured.NestedSlice(object.Object, "spec", "roles")
				for _, item := range roles {
					role := item.(map[string]any)
					template, _, _ := unstructured.NestedMap(role, "spec", "leaderWorkerTemplate")
					pods = append(pods, groupPods(t, role["name"].(string), template)...)
				}
			}
		}
	}
	return pods
}

func groupPods(t *testing.T, name string, template map[string]any) []testPod {
	t.Helper()
	size, _, _ := unstructured.NestedInt64(template, "size")
	worker := podTemplate(t, template["workerTemplate"])
	leader := worker
	if template["leaderTemplate"] != nil {
		leader = podTemplate(t, template["leaderTemplate"])
	}
	pods := []testPod{templatePod(t, name+"-leader", leader, "0")}
	pods[0].leader = true
	for index := int64(1); index < size; index++ {
		pods = append(pods, templatePod(t, name+"-worker", worker, "1"))
	}
	return pods
}

func podTemplate(t *testing.T, value any) corev1.PodTemplateSpec {
	t.Helper()
	var template corev1.PodTemplateSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value.(map[string]any), &template); err != nil {
		t.Fatal(err)
	}
	return template
}

func templatePod(t *testing.T, name string, template corev1.PodTemplateSpec, workerIndex string) testPod {
	t.Helper()
	podLabels := maps.Clone(template.Labels)
	if workerIndex != "" {
		podLabels[lwsWorkerIndexLabel] = workerIndex
	}
	container := template.Spec.Containers[0]
	return testPod{name: name, labels: podLabels, ports: container.Ports, args: container.Args}
}

// relabel applies relabelings the way Prometheus does to a pod's discovered
// labels, returning the target labels they set.
func relabel(t *testing.T, relabelings []any, podLabels map[string]string) map[string]string {
	t.Helper()
	sanitize := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	discovered := map[string]string{}
	for key, value := range podLabels {
		discovered["__meta_kubernetes_pod_label_"+sanitize.ReplaceAllString(key, "_")] = value
	}
	for _, item := range relabelings {
		rule := item.(map[string]any)
		var values []string
		sources, _ := rule["sourceLabels"].([]any)
		for _, source := range sources {
			values = append(values, discovered[source.(string)])
		}
		pattern, _ := rule["regex"].(string)
		if pattern == "" {
			pattern = "(.*)"
		}
		replacement, ok := rule["replacement"].(string)
		if !ok {
			replacement = "$1"
		}
		re := regexp.MustCompile("^(?:" + pattern + ")$")
		value := strings.Join(values, ";")
		match := re.FindStringSubmatchIndex(value)
		if match == nil {
			continue
		}
		discovered[rule["targetLabel"].(string)] = string(re.ExpandString(nil, replacement, value, match))
	}
	return map[string]string{
		"inference_service": discovered["inference_service"],
		"inference_role":    discovered["inference_role"],
	}
}

func TestPodMonitorScrapesServingPods(t *testing.T) {
	tests := []struct {
		name        string
		runtime     string
		mode        string
		wantLeaders map[string]string // leader pod name to inference_role
	}{
		{"vllm single", runtimeVLLM, modeSingle, map[string]string{"group-leader": "default"}},
		{"vllm multi", runtimeVLLM, modeMulti, map[string]string{"group-leader": "default"}},
		{"sglang single", runtimeSGLang, modeSingle, map[string]string{"group-leader": "default"}},
		{"sglang multi", runtimeSGLang, modeMulti, map[string]string{"group-leader": "default"}},
		{"sglang pd", runtimeSGLang, modePD, map[string]string{"prefill-leader": "prefill", "decode-leader": "decode"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testRequest(tt.runtime, tt.mode)
			objects, _, err := buildServiceObjects(req, testModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			monitor, err := buildPodMonitor(req, nil)
			if err != nil {
				t.Fatal(err)
			}
			matchLabels, _, _ := unstructured.NestedStringMap(monitor.Object, "spec", "selector", "matchLabels")
			selector := labels.SelectorFromSet(matchLabels)
			endpoints, _, _ := unstructured.NestedSlice(monitor.Object, "spec", "podMetricsEndpoints")
			endpoint := endpoints[0].(map[string]any)
			relabelings, _ := endpoint["relabelings"].([]any)

			scraped := map[string]string{}
			for _, pod := range servicePods(t, objects) {
				if !selector.Matches(labels.Set(pod.labels)) {
					continue
				}
				if !pod.leader {
					t.Fatalf("PodMonitor selects %s, which serves no HTTP", pod.name)
				}
				if !slices.ContainsFunc(pod.ports, func(port corev1.ContainerPort) bool {
					return port.Name == endpoint["port"] && port.ContainerPort == servingPort
				}) {
					t.Fatalf("%s has no port %v", pod.name, endpoint["port"])
				}
				if tt.runtime == runtimeSGLang && !slices.Contains(pod.args, "--enable-metrics") {
					t.Fatalf("%s runs SGLang without --enable-metrics", pod.name)
				}
				target := relabel(t, relabelings, pod.labels)
				if target["inference_service"] != req.Name {
					t.Fatalf("%s is labeled inference_service=%q, want %q", pod.name, target["inference_service"], req.Name)
				}
				scraped[pod.name] = target["inference_role"]
			}
			if !maps.Equal(scraped, tt.wantLeaders) {
				t.Fatalf("scraped pods and roles = %v, want %v", scraped, tt.wantLeaders)
			}
		})
	}
}

func TestWithPodMonitor(t *testing.T) {
	defaultLabels := map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "kube-prometheus-stack"}
	tests := []struct {
		name        string
		objects     []client.Object
		funcs       interceptor.Funcs
		wantMonitor bool
		wantLabels  map[string]string
		wantErr     bool
	}{
		{
			name: "no PodMonitor CRD",
		},
		{
			name:        "no Prometheus",
			objects:     []client.Object{podMonitorCRDObject()},
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "Prometheus selecting other labels",
			objects:     []client.Object{podMonitorCRDObject(), prometheusObject(map[string]any{"release": "monitoring", "team": "ml"})},
			wantMonitor: true,
			wantLabels:  map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "monitoring", "team": "ml"},
		},
		{
			name:        "Prometheus selecting every PodMonitor",
			objects:     []client.Object{podMonitorCRDObject(), prometheusObject(map[string]any{})},
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "no Prometheus CRD",
			objects:     []client.Object{podMonitorCRDObject()},
			funcs:       failKind(prometheusGVK.Kind, noKindMatch(prometheusGVK)),
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "listing Prometheus fails",
			objects:     []client.Object{podMonitorCRDObject()},
			funcs:       failKind(prometheusGVK.Kind, errors.New("connection refused")),
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:    "looking up the CRD fails",
			funcs:   failKind("CustomResourceDefinition", errors.New("connection refused")),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestClientSet(t, tt.funcs, tt.objects...)
			req := testRequest(runtimeSGLang, modeSingle)
			objects, workload, err := buildServiceObjects(req, testModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := withPodMonitor(context.Background(), cs, req, objects)
			if tt.wantErr {
				if err == nil {
					t.Fatal("withPodMonitor() succeeded, want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got[len(got)-1] != client.Object(workload) {
				t.Fatalf("the workload is not created last")
			}
			if !tt.wantMonitor {
				if len(got) != len(objects) {
					t.Fatalf("got %d objects, want the %d of the service", len(got), len(objects))
				}
				return
			}
			if len(got) != len(objects)+1 {
				t.Fatalf("got %d objects, want the %d of the service and a PodMonitor", len(got), len(objects))
			}
			monitor, ok := got[0].(*unstructured.Unstructured)
			if !ok || monitor.GroupVersionKind() != podMonitorGVK {
				t.Fatalf("first object is %T, want a PodMonitor", got[0])
			}
			if !maps.Equal(monitor.GetLabels(), tt.wantLabels) {
				t.Fatalf("labels = %v, want %v", monitor.GetLabels(), tt.wantLabels)
			}
		})
	}
}

func TestCreateAndDeleteServiceObjects(t *testing.T) {
	for _, mode := range []string{modeSingle, modeMulti, modePD} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			cs := newTestClientSet(t, interceptor.Funcs{}, podMonitorCRDObject())
			req := testRequest(runtimeSGLang, mode)
			objects, workload, err := buildServiceObjects(req, testModel, nil)
			if err == nil {
				objects, err = withPodMonitor(ctx, cs, req, objects)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := createObjects(ctx, cs, objects); err != nil {
				t.Fatal(err)
			}
			monitor := &unstructured.Unstructured{}
			monitor.SetGroupVersionKind(podMonitorGVK)
			if err := cs.K8sClient.Get(ctx, client.ObjectKey{Namespace: "ai", Name: "qwen"}, monitor); err != nil {
				t.Fatalf("PodMonitor was not created: %v", err)
			}

			if err := deleteServiceObjects(ctx, cs, workload); err != nil {
				t.Fatal(err)
			}
			for _, object := range objects {
				err := cs.K8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)
				if !apierrors.IsNotFound(err) {
					t.Errorf("%T %s is left after deleting the service: %v", object, object.GetName(), err)
				}
			}
		})
	}
}

func TestDeleteServiceObjectsPodMonitor(t *testing.T) {
	ours := func() client.Object {
		monitor, err := buildPodMonitor(testRequest(runtimeVLLM, modeSingle), nil)
		if err != nil {
			t.Fatal(err)
		}
		return monitor
	}
	notOurs := func() client.Object {
		monitor := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "qwen", "namespace": "ai"},
		}}
		monitor.SetGroupVersionKind(podMonitorGVK)
		return monitor
	}
	tests := []struct {
		name        string
		monitor     func() client.Object
		funcs       interceptor.Funcs
		wantMonitor bool
	}{
		{name: "PodMonitor of the service", monitor: ours},
		{name: "same-named PodMonitor without the service label", monitor: notOurs, wantMonitor: true},
		{name: "no PodMonitor", monitor: nil},
		{name: "no PodMonitor CRD", funcs: failKind(podMonitorGVK.Kind, noKindMatch(podMonitorGVK))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			workload, err := buildLeaderWorkerSet(testRequest(runtimeVLLM, modeSingle), testModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			objects := []client.Object{workload, buildService(testRequest(runtimeVLLM, modeSingle))}
			if tt.monitor != nil {
				objects = append(objects, tt.monitor())
			}
			cs := newTestClientSet(t, tt.funcs, objects...)

			if err := deleteServiceObjects(ctx, cs, workload); err != nil {
				t.Fatalf("deleteServiceObjects() = %v", err)
			}
			if err := cs.K8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload.DeepCopy()); !apierrors.IsNotFound(err) {
				t.Fatalf("the workload is left: %v", err)
			}
			if tt.monitor == nil {
				return
			}
			monitor := &unstructured.Unstructured{}
			monitor.SetGroupVersionKind(podMonitorGVK)
			err = cs.K8sClient.Get(ctx, client.ObjectKey{Namespace: "ai", Name: "qwen"}, monitor)
			if tt.wantMonitor && err != nil {
				t.Fatalf("the PodMonitor that is not the service's was deleted: %v", err)
			}
			if !tt.wantMonitor && !apierrors.IsNotFound(err) {
				t.Fatalf("the PodMonitor is left: %v", err)
			}
		})
	}
}
