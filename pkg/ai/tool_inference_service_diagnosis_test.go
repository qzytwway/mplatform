package ai

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDescribeReportsAnInitContainerWithoutErrorLines(t *testing.T) {
	pod := servicePod("qwen-0", "qwen", "0", "0", false, 0)
	pod.Status.Phase = corev1.PodPending
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name:  "model-downloader",
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}}
	fc := newInferenceServiceCluster(t, lwsService("qwen", "single", 1, 1, 0), pod)
	fc.logs["qwen-0/model-downloader"] = "download: s3://models/qwen3-8b/model-00001-of-00004.safetensors\n"

	got := describeInferenceService(t, fc, "qwen")

	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-0": []any{
		"init container model-downloader: running, 0 restarts, no error lines in its log",
	}})
}

func TestDescribeSaysWhenAPodCannotBeReadForItsInitContainers(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 1, 0),
		servicePod("qwen-0", "qwen", "0", "0", false, 0),
	)
	fc.podGetErr = errors.New("pods is forbidden")
	fc.logs["qwen-0"] = "RuntimeError: CUDA error: no kernel image is available\n"

	got := describeInferenceService(t, fc, "qwen")

	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-0": []any{
		"pod could not be read, so its init containers are unknown: pods is forbidden",
		"RuntimeError: CUDA error: no kernel image is available",
	}})
}

func TestDescribeKeepsTheDiagnosisWhenEventsCannotBeRead(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 1, 0),
		servicePod("qwen-0", "qwen", "0", "0", false, 1),
	)
	fc.eventsStatus = http.StatusForbidden
	fc.logs["qwen-0/previous"] = "torch.OutOfMemoryError: CUDA out of memory.\n"

	got := describeInferenceService(t, fc, "qwen")

	assertJSON(t, got, "model", "qwen3-8b")
	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-0": []any{"torch.OutOfMemoryError: CUDA out of memory."}})
	if note, _ := got["eventsError"].(string); !strings.HasPrefix(note, "events could not be read: ") {
		t.Fatalf("want a note that events could not be read, got %#v", got["eventsError"])
	}
}

// ownedBy makes object a child of owner, as the controllers that create
// StatefulSets, ReplicaSets and Pods do.
func ownedBy[T client.Object](object T, owner client.Object, uid string) T {
	object.SetUID(types.UID(uid))
	object.SetOwnerReferences([]metav1.OwnerReference{{Name: owner.GetName(), UID: owner.GetUID()}})
	return object
}

func eventOn(kind, name, reason, message string) corev1.Event {
	event := podEventFor(name, reason, message)
	event.Name = kind + "." + name + "." + reason
	event.InvolvedObject.Kind = kind
	return event
}

func statefulSet(name string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
}

// TestDescribeReportsTheEventsOfTheWorkloadObjects shows why a service's
// Pods are missing: its controllers report quota and admission failures on
// the StatefulSets and ReplicaSets that create them.
func TestDescribeReportsTheEventsOfTheWorkloadObjects(t *testing.T) {
	quota := `pods "qwen-0" is forbidden: exceeded quota: gpu-quota, requested: requests.nvidia.com/gpu=1, used: requests.nvidia.com/gpu=8, limited: requests.nvidia.com/gpu=8`
	lws := lwsService("qwen", "single", 1, 1, 0)
	lws.SetUID("lws")
	leaderSet := ownedBy(statefulSet("qwen"), lws, "leader-set")
	multi := lwsService("llama", "multi", 2, 1, 0)
	multi.SetUID("multi")
	multiLeaderSet := ownedBy(statefulSet("llama"), multi, "multi-leader-set")
	leader := ownedBy(servicePod("llama-0", "llama", "0", "0", false, 0), multiLeaderSet, "leader")
	pd := pdService()
	router := pd[3].(*appsv1.Deployment)
	router.SetUID("router")
	tests := []struct {
		name    string
		objects []client.Object
		events  []corev1.Event
		want    map[string]any
	}{{
		name:    "qwen",
		objects: []client.Object{lws, leaderSet, statefulSet("other")},
		events: []corev1.Event{
			eventOn("StatefulSet", "qwen", "FailedCreate", "create Pod qwen-0 in StatefulSet qwen failed error: "+quota),
			eventOn("StatefulSet", "other", "FailedCreate", "not this service's"),
		},
		want: map[string]any{"StatefulSet/qwen": []any{map[string]any{
			"type": "Warning", "reason": "FailedCreate", "message": "create Pod qwen-0 in StatefulSet qwen failed error: " + quota, "count": 4.0,
		}}},
	}, {
		// The worker StatefulSet of a group is named after, and owned by, its
		// leader Pod.
		name:    "llama",
		objects: []client.Object{multi, multiLeaderSet, leader, ownedBy(statefulSet("llama-0"), leader, "worker-set")},
		events:  []corev1.Event{eventOn("StatefulSet", "llama-0", "FailedCreate", "create Pod llama-0-1 failed")},
		want: map[string]any{"StatefulSet/llama-0": []any{map[string]any{
			"type": "Warning", "reason": "FailedCreate", "message": "create Pod llama-0-1 failed", "count": 4.0,
		}}},
	}, {
		name: "qwen-pd",
		objects: append(pd, ownedBy(&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
			Name: "qwen-pd-router-5d8f", Namespace: "default",
		}}, router, "router-set")),
		events: []corev1.Event{
			eventOn("ReplicaSet", "qwen-pd-router-5d8f", "FailedCreate", "admission webhook denied the request"),
			eventOn("Deployment", "qwen-pd-router", "ScalingReplicaSet", "Scaled up replica set qwen-pd-router-5d8f from 0 to 1"),
		},
		want: map[string]any{
			"ReplicaSet/qwen-pd-router-5d8f": []any{map[string]any{
				"type": "Warning", "reason": "FailedCreate", "message": "admission webhook denied the request", "count": 4.0,
			}},
			"Deployment/qwen-pd-router": []any{map[string]any{
				"type": "Warning", "reason": "ScalingReplicaSet", "message": "Scaled up replica set qwen-pd-router-5d8f from 0 to 1", "count": 4.0,
			}},
		},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := newInferenceServiceCluster(t, tc.objects...)
			fc.events = tc.events

			got := describeInferenceService(t, fc, tc.name)

			assertJSON(t, got, "workloadEvents", tc.want)
			for _, selector := range fc.eventSelectors {
				if !strings.Contains(selector, "involvedObject.name=") {
					t.Fatalf("events were listed beyond one object: fieldSelector %q", selector)
				}
			}
		})
	}
}

func TestDescribeReportsTheInitContainerErrorsOfAPodStuckInInit(t *testing.T) {
	pod := servicePod("qwen-0", "qwen", "0", "0", false, 0)
	pod.Status.Phase = corev1.PodPending
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name: "model-downloader", RestartCount: 2,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
	}}
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}
	fc := newInferenceServiceCluster(t, lwsService("qwen", "single", 1, 1, 0), pod)
	fc.logs["qwen-0/model-downloader/previous"] = "download: s3://models/qwen3-8b/config.json\n" +
		"fatal error: An error occurred (AccessDenied) when calling the ListObjectsV2 operation: Access Denied\n"

	got := describeInferenceService(t, fc, "qwen")

	assertJSON(t, got, "engineLogErrors", map[string]any{"qwen-0": []any{
		"init container model-downloader: fatal error: An error occurred (AccessDenied) when calling the ListObjectsV2 operation: Access Denied",
	}})
}
