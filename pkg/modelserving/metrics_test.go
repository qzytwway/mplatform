package modelserving

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
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
	metricsTestModel     = &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypeLocal, Path: "/data/qwen"}
	metricsPrometheusGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "Prometheus"}
)

// metricsTestRequest returns a service request for runtime and mode; multi-node
// groups have two nodes, as does the prefill role of pd mode.
func metricsTestRequest(runtime, mode string) *serviceRequest {
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

func metricsClientSet(t *testing.T, funcs interceptor.Funcs, objects ...client.Object) *cluster.ClientSet {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, gvk := range []schema.GroupVersionKind{lwsGVK, disaggregatedSetGVK, podMonitorGVK, metricsPrometheusGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(funcs).Build()
	return &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient}}
}

func metricsPodMonitorCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: podMonitorCRD}}
}

func metricsPrometheus(matchLabels map[string]any) *unstructured.Unstructured {
	prometheus := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "k8s", "namespace": "monitoring"},
		"spec":     map[string]any{"podMonitorSelector": map[string]any{"matchLabels": matchLabels}},
	}}
	prometheus.SetGroupVersionKind(metricsPrometheusGVK)
	return prometheus
}

// metricsFailKind makes reads of kind fail with err, as a cluster without the
// kind's CRD does with a NoKindMatchError.
func metricsFailKind(kind string, err error) interceptor.Funcs {
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

func metricsNoKindMatch(gvk schema.GroupVersionKind) error {
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
			req := metricsTestRequest(tt.runtime, tt.mode)
			args := runtimeArgs(req, metricsTestModel, tt.pdRole, tt.worker)
			want := 0
			if tt.want {
				want = 1
			}
			if got := metricsCountArg(args, "--enable-metrics"); got != want {
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
			got := runtimeArgs(metricsTestRequest(tt.runtime, modeSingle), metricsTestModel, "", false)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("runtimeArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func metricsCountArg(args []string, arg string) int {
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
		{
			name:             "Prometheus labels naming the service labels",
			prometheusLabels: map[string]string{managedByLabel: "Helm", serviceLabel: "other", "release": "monitoring"},
			wantLabels:       map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "monitoring"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor, err := buildPodMonitor(metricsTestRequest(runtimeSGLang, modeSingle), tt.prometheusLabels)
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
				map[string]any{"sourceLabels": []any{"__meta_kubernetes_pod_ready"}, "regex": "true", "action": "keep"},
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

// metricsPod is a pod LeaderWorkerSet would run from a template, with the
// worker index label it adds.
type metricsPod struct {
	name   string
	labels map[string]string
	ports  []corev1.ContainerPort
	args   []string
	leader bool
}

// metricsServicePods returns the pods of one group of every role of objects, and the
// router pod in pd mode.
func metricsServicePods(t *testing.T, objects []client.Object) []metricsPod {
	t.Helper()
	var pods []metricsPod
	for _, object := range objects {
		switch object := object.(type) {
		case *appsv1.Deployment:
			pods = append(pods, metricsTemplatePod(t, "router", object.Spec.Template, ""))
		case *unstructured.Unstructured:
			if object.GetKind() == lwsGVK.Kind {
				template, _, _ := unstructured.NestedMap(object.Object, "spec", "leaderWorkerTemplate")
				pods = append(pods, metricsGroupPods(t, "group", template)...)
			}
			if object.GetKind() == disaggregatedSetGVK.Kind {
				roles, _, _ := unstructured.NestedSlice(object.Object, "spec", "roles")
				for _, item := range roles {
					role := item.(map[string]any)
					template, _, _ := unstructured.NestedMap(role, "spec", "leaderWorkerTemplate")
					pods = append(pods, metricsGroupPods(t, role["name"].(string), template)...)
				}
			}
		}
	}
	return pods
}

func metricsGroupPods(t *testing.T, name string, template map[string]any) []metricsPod {
	t.Helper()
	size, _, _ := unstructured.NestedInt64(template, "size")
	worker := metricsPodTemplate(t, template["workerTemplate"])
	leader := worker
	if template["leaderTemplate"] != nil {
		leader = metricsPodTemplate(t, template["leaderTemplate"])
	}
	pods := []metricsPod{metricsTemplatePod(t, name+"-leader", leader, "0")}
	pods[0].leader = true
	for index := int64(1); index < size; index++ {
		pods = append(pods, metricsTemplatePod(t, name+"-worker", worker, "1"))
	}
	return pods
}

func metricsPodTemplate(t *testing.T, value any) corev1.PodTemplateSpec {
	t.Helper()
	var template corev1.PodTemplateSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value.(map[string]any), &template); err != nil {
		t.Fatal(err)
	}
	return template
}

func metricsTemplatePod(t *testing.T, name string, template corev1.PodTemplateSpec, workerIndex string) metricsPod {
	t.Helper()
	podLabels := maps.Clone(template.Labels)
	if workerIndex != "" {
		podLabels[lwsWorkerIndexLabel] = workerIndex
	}
	container := template.Spec.Containers[0]
	return metricsPod{name: name, labels: podLabels, ports: container.Ports, args: container.Args}
}

// metricsRelabel applies relabelings the way Prometheus does to what it
// discovers on a pod, returning the target labels they set, or false when
// they drop the target.
func metricsRelabel(t *testing.T, relabelings []any, podLabels map[string]string, ready bool) (map[string]string, bool) {
	t.Helper()
	sanitize := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	discovered := map[string]string{"__meta_kubernetes_pod_ready": strconv.FormatBool(ready)}
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
		switch action, _ := rule["action"].(string); action {
		case "keep":
			if match == nil {
				return nil, false
			}
		case "drop":
			if match != nil {
				return nil, false
			}
		case "", "replace":
			if match != nil {
				discovered[rule["targetLabel"].(string)] = string(re.ExpandString(nil, replacement, value, match))
			}
		default:
			t.Fatalf("relabel action %q is not simulated", action)
		}
	}
	return map[string]string{
		"inference_service": discovered["inference_service"],
		"inference_role":    discovered["inference_role"],
	}, true
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
			req := metricsTestRequest(tt.runtime, tt.mode)
			objects, _, err := buildServiceObjects(req, metricsTestModel, nil)
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
			for _, pod := range metricsServicePods(t, objects) {
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
				if _, kept := metricsRelabel(t, relabelings, pod.labels, false); kept {
					t.Fatalf("%s is scraped before it is ready", pod.name)
				}
				target, kept := metricsRelabel(t, relabelings, pod.labels, true)
				if !kept {
					t.Fatalf("%s is not scraped once it is ready", pod.name)
				}
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

// metricsOtherPodMonitor is a PodMonitor named like the test service that is
// not an inference service's.
func metricsOtherPodMonitor() *unstructured.Unstructured {
	monitor := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "qwen", "namespace": "ai"},
	}}
	monitor.SetGroupVersionKind(podMonitorGVK)
	return monitor
}

func metricsServicePodMonitor(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	monitor, err := buildPodMonitor(metricsTestRequest(runtimeVLLM, modeSingle), nil)
	if err != nil {
		t.Fatal(err)
	}
	return monitor
}

func TestWithPodMonitor(t *testing.T) {
	defaultLabels := map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "kube-prometheus-stack"}
	tests := []struct {
		name         string
		objects      []client.Object
		funcs        interceptor.Funcs
		wantMonitor  bool
		wantLabels   map[string]string
		wantErr      bool
		wantConflict bool
	}{
		{
			name: "no PodMonitor CRD",
		},
		{
			name:        "no Prometheus",
			objects:     []client.Object{metricsPodMonitorCRD()},
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "Prometheus selecting other labels",
			objects:     []client.Object{metricsPodMonitorCRD(), metricsPrometheus(map[string]any{"release": "monitoring", "team": "ml"})},
			wantMonitor: true,
			wantLabels:  map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen", "release": "monitoring", "team": "ml"},
		},
		{
			name:        "Prometheus selecting every PodMonitor",
			objects:     []client.Object{metricsPodMonitorCRD(), metricsPrometheus(map[string]any{})},
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "no Prometheus CRD",
			objects:     []client.Object{metricsPodMonitorCRD()},
			funcs:       metricsFailKind(metricsPrometheusGVK.Kind, metricsNoKindMatch(metricsPrometheusGVK)),
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:        "listing Prometheus fails",
			objects:     []client.Object{metricsPodMonitorCRD()},
			funcs:       metricsFailKind(metricsPrometheusGVK.Kind, errors.New("connection refused")),
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			// Creating the service then fails on its Service, which says the
			// name is taken.
			name:        "PodMonitor of a service with the name",
			objects:     []client.Object{metricsPodMonitorCRD(), metricsServicePodMonitor(t)},
			wantMonitor: true,
			wantLabels:  defaultLabels,
		},
		{
			name:         "PodMonitor of something else with the name",
			objects:      []client.Object{metricsPodMonitorCRD(), metricsOtherPodMonitor()},
			wantErr:      true,
			wantConflict: true,
		},
		{
			name:    "looking up the CRD fails",
			funcs:   metricsFailKind("CustomResourceDefinition", errors.New("connection refused")),
			wantErr: true,
		},
		{
			name:    "looking up the PodMonitor fails",
			objects: []client.Object{metricsPodMonitorCRD()},
			funcs:   metricsFailKind(podMonitorGVK.Kind, errors.New("connection refused")),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := metricsClientSet(t, tt.funcs, tt.objects...)
			req := metricsTestRequest(runtimeSGLang, modeSingle)
			objects, workload, err := buildServiceObjects(req, metricsTestModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := withPodMonitor(context.Background(), cs, req, objects)
			if tt.wantErr {
				checkPodMonitorError(t, err, tt.wantConflict)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got[len(got)-1] != client.Object(workload) {
				t.Fatalf("the workload is not created last")
			}
			if !tt.wantMonitor {
				if !slices.Equal(got, objects) {
					t.Fatalf("got objects %v, want the service's %v", got, objects)
				}
				return
			}
			// The PodMonitor goes right before the workload.
			if want := slices.Insert(slices.Clone(objects), len(objects)-1, got[len(got)-2]); !slices.Equal(got, want) {
				t.Fatalf("got objects %v, want %v", got, want)
			}
			monitor, ok := got[len(got)-2].(*unstructured.Unstructured)
			if !ok || monitor.GroupVersionKind() != podMonitorGVK {
				t.Fatalf("object before the workload is %T, want a PodMonitor", got[len(got)-2])
			}
			if !maps.Equal(monitor.GetLabels(), tt.wantLabels) {
				t.Fatalf("labels = %v, want %v", monitor.GetLabels(), tt.wantLabels)
			}
		})
	}
}

func checkPodMonitorError(t *testing.T, err error, wantConflict bool) {
	t.Helper()
	if err == nil {
		t.Fatal("withPodMonitor() succeeded, want an error")
	}
	if !wantConflict {
		return
	}
	if !apierrors.IsAlreadyExists(err) || createErrorStatus(err) != http.StatusConflict {
		t.Fatalf("withPodMonitor() = %v, want a conflict", err)
	}
	if want := "PodMonitor ai/qwen already exists and is not an inference service's"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not say %q", err, want)
	}
}

// TestCreateObjectsRemovesPodMonitor fails the workload, created after the
// PodMonitor, and expects nothing of the service to be left.
func TestCreateObjectsRemovesPodMonitor(t *testing.T) {
	for _, mode := range []string{modeSingle, modePD} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			req := metricsTestRequest(runtimeSGLang, mode)
			objects, workload, err := buildServiceObjects(req, metricsTestModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			denyWorkload := interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if obj == client.Object(workload) {
						return errors.New("admission webhook denied the request")
					}
					return c.Create(ctx, obj, opts...)
				},
			}
			cs := metricsClientSet(t, denyWorkload, metricsPodMonitorCRD())
			if objects, err = withPodMonitor(ctx, cs, req, objects); err != nil {
				t.Fatal(err)
			}
			if err := createObjects(ctx, cs, objects); err == nil {
				t.Fatal("createObjects() succeeded, want the workload to fail")
			}
			for _, object := range objects {
				err := cs.K8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)
				if !apierrors.IsNotFound(err) {
					t.Errorf("%T %s is left after the service failed: %v", object, object.GetName(), err)
				}
			}
		})
	}
}

func TestCreateAndDeleteServiceObjects(t *testing.T) {
	for _, mode := range []string{modeSingle, modeMulti, modePD} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			cs := metricsClientSet(t, interceptor.Funcs{}, metricsPodMonitorCRD())
			req := metricsTestRequest(runtimeSGLang, mode)
			objects, workload, err := buildServiceObjects(req, metricsTestModel, nil)
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
	ingressGVK := schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}
	tests := []struct {
		name        string
		monitor     client.Object
		funcs       interceptor.Funcs
		wantMonitor bool
		wantErr     bool
	}{
		{name: "PodMonitor of the service", monitor: metricsServicePodMonitor(t)},
		{name: "same-named PodMonitor without the service label", monitor: metricsOtherPodMonitor(), wantMonitor: true},
		{name: "no PodMonitor"},
		{name: "no PodMonitor CRD", funcs: metricsFailKind(podMonitorGVK.Kind, metricsNoKindMatch(podMonitorGVK))},
		// Only the PodMonitor is optional; other kinds missing is an error.
		{name: "no Ingress kind", funcs: metricsFailKind(ingressGVK.Kind, metricsNoKindMatch(ingressGVK)), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			workload, err := buildLeaderWorkerSet(metricsTestRequest(runtimeVLLM, modeSingle), metricsTestModel, nil)
			if err != nil {
				t.Fatal(err)
			}
			objects := []client.Object{workload, buildService(metricsTestRequest(runtimeVLLM, modeSingle))}
			if tt.monitor != nil {
				objects = append(objects, tt.monitor)
			}
			cs := metricsClientSet(t, tt.funcs, objects...)

			err = deleteServiceObjects(ctx, cs, workload)
			if tt.wantErr {
				if err == nil {
					t.Fatal("deleteServiceObjects() succeeded, want an error")
				}
				return
			}
			if err != nil {
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
