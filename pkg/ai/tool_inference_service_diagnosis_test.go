package ai

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

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
