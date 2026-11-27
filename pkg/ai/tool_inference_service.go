package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/qzytwway/mplatform/pkg/cluster"
	pkgmodel "github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/modelserving"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	listInferenceServicesTool    = "list_inference_services"
	describeInferenceServiceTool = "describe_inference_service"
)

// adminOnlyTools read model serving, whose pages and APIs are admin-only.
var adminOnlyTools = map[string]bool{
	listInferenceServicesTool:    true,
	describeInferenceServiceTool: true,
	inferenceServiceGPUsTool:     true,
	inferenceServiceMetricsTool:  true,
}

func inferenceServiceToolDefinitions() []agentToolDefinition {
	return []agentToolDefinition{
		{
			Name:        listInferenceServicesTool,
			Description: "List the Inference Services (model serving) in the current cluster with their Model, Engine (vLLM or SGLang), Deployment Mode and each Role's desired and ready Replicas. Admin only.",
			Properties:  map[string]any{},
		},
		{
			Name:        describeInferenceServiceTool,
			Description: "Describe one Inference Service as its console detail page shows it: Model, Engine (vLLM or SGLang), Deployment Mode, each Role's desired and ready Replicas, its Pods (Leader pod / Worker pod), deployment settings, recent events of its Pods and of the workload objects that create them (keyed <kind>/<name>: LeaderWorkerSet, DisaggregatedSet, router Deployment, their StatefulSets and ReplicaSets, which report quota and admission failures such as FailedCreate when Pods are missing), and the error lines of the logs of Pods that are not ready or restarted (of the model downloader init container while a Pod is stuck initializing, otherwise of the Engine). eventsError says when events could not be read. Use this to diagnose why an Inference Service is unhealthy. Admin only.",
			Properties: map[string]any{
				"namespace": map[string]any{"type": "string", "description": "The namespace of the Inference Service."},
				"name":      map[string]any{"type": "string", "description": "The name of the Inference Service."},
			},
			Required: []string{"namespace", "name"},
		},
		inferenceServiceGPUsToolDefinition(),
		inferenceServiceMetricsToolDefinition(),
	}
}

// authorizeAdminOnlyTool applies RequireAdmin's rule to adminOnlyTools.
func authorizeAdminOnlyTool(user pkgmodel.User, toolName string) (string, bool) {
	if !adminOnlyTools[toolName] || rbac.UserHasRole(user, pkgmodel.DefaultAdminRole.Name) {
		return "", false
	}
	return "Forbidden: inference services are admin-only, and user " + user.Key() + " is not an admin", true
}

// lookupInferenceService reads the inference service named in args as its
// detail page shows it, or returns the tool's error result.
func lookupInferenceService(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (*modelserving.ServiceDetail, string, bool) {
	namespace, err := getRequiredString(args, "namespace")
	if err != nil {
		return nil, "Error: " + err.Error(), true
	}
	name, err := getRequiredString(args, "name")
	if err != nil {
		return nil, "Error: " + err.Error(), true
	}
	detail, err := modelserving.GetServiceDetail(ctx, cs, namespace, name)
	if errors.Is(err, modelserving.ErrServiceNotFound) {
		return nil, fmt.Sprintf("Error: inference service %s/%s not found", namespace, name), true
	} else if err != nil {
		return nil, fmt.Sprintf("Error reading inference service %s/%s: %v", namespace, name, err), true
	}
	return detail, "", false
}

func executeListInferenceServices(ctx context.Context, cs *cluster.ClientSet) (string, bool) {
	services, err := modelserving.ListInferenceServices(ctx, cs)
	if err != nil {
		return "Error listing inference services: " + err.Error(), true
	}
	return toolJSON(services)
}

const (
	maxObjectEvents       = 10
	maxConcurrentReads    = 8
	engineLogTailLines    = 500
	maxEngineLogBytes     = 256 * 1024
	maxEngineLogErrors    = 20
	maxEngineLogLineBytes = 500
)

// engineErrorWords mark the Engine log lines worth reporting, lowercased.
var engineErrorWords = []string{"error", "exception", "traceback", "fatal", "out of memory"}

type podEvent struct {
	Type     string `json:"type"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	Count    int32  `json:"count"`
	LastSeen string `json:"lastSeen,omitempty"`
}

// inferenceServiceDiagnosis is the detail page's view of a service with the
// recent events of its pods, keyed by pod name, and of the workload objects
// that create them, keyed <kind>/<name>, and the error lines in the logs of
// pods that are not ready or restarted, keyed by pod name. EventsError says
// why events are missing.
type inferenceServiceDiagnosis struct {
	*modelserving.ServiceDetail
	Events          map[string][]podEvent `json:"events,omitempty"`
	WorkloadEvents  map[string][]podEvent `json:"workloadEvents,omitempty"`
	EventsError     string                `json:"eventsError,omitempty"`
	EngineLogErrors map[string][]string   `json:"engineLogErrors,omitempty"`
}

func executeDescribeInferenceService(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	detail, result, isError := lookupInferenceService(ctx, cs, args)
	if isError {
		return result, true
	}
	diagnosis := inferenceServiceDiagnosis{ServiceDetail: detail, EngineLogErrors: map[string][]string{}}
	for _, pod := range detail.Pods {
		if pod.Ready && pod.Restarts == 0 {
			continue
		}
		if lines := podLogErrors(ctx, cs, detail.Namespace, pod.Name, pod.Restarts > 0); len(lines) > 0 {
			diagnosis.EngineLogErrors[pod.Name] = lines
		}
	}
	var err error
	diagnosis.Events, diagnosis.WorkloadEvents, err = serviceEvents(ctx, cs, detail)
	if err != nil {
		klog.Warningf("Reading the events of inference service %s/%s: %v", detail.Namespace, detail.Name, err)
		diagnosis.EventsError = "events could not be read: " + err.Error()
	}
	return toolJSON(diagnosis)
}

// serviceEvents returns the events of a service's pods, keyed by pod name,
// and of its workload objects, keyed <kind>/<name>, with those read before an
// error.
func serviceEvents(ctx context.Context, cs *cluster.ClientSet, detail *modelserving.ServiceDetail) (map[string][]podEvent, map[string][]podEvent, error) {
	objects, err := modelserving.ServiceObjects(ctx, cs, detail.Namespace, detail.Name)
	for _, pod := range detail.Pods {
		objects = append(objects, modelserving.ServiceObject{Kind: "Pod", Name: pod.Name})
	}
	found := make([][]podEvent, len(objects))
	var group errgroup.Group
	group.SetLimit(maxConcurrentReads)
	for i, object := range objects {
		group.Go(func() error {
			var readErr error
			found[i], readErr = objectEvents(ctx, cs, detail.Namespace, object)
			return readErr
		})
	}
	err = errors.Join(err, group.Wait())
	pods, workload := map[string][]podEvent{}, map[string][]podEvent{}
	for i, object := range objects {
		switch {
		case len(found[i]) == 0:
		case object.Kind == "Pod":
			pods[object.Name] = found[i]
		default:
			workload[object.Kind+"/"+object.Name] = found[i]
		}
	}
	return pods, workload, err
}

// objectEvents returns the latest events of one object, newest first.
func objectEvents(ctx context.Context, cs *cluster.ClientSet, namespace string, object modelserving.ServiceObject) ([]podEvent, error) {
	selector := fields.Set{"involvedObject.kind": object.Kind, "involvedObject.name": object.Name}.AsSelector().String()
	list, err := cs.K8sClient.ClientSet.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", object.Kind, object.Name, err)
	}
	items := list.Items
	sort.SliceStable(items, func(i, j int) bool { return eventTime(&items[i]).After(eventTime(&items[j])) })
	items = items[:min(len(items), maxObjectEvents)]
	events := make([]podEvent, 0, len(items))
	for i := range items {
		event := &items[i]
		item := podEvent{Type: event.Type, Reason: event.Reason, Message: event.Message, Count: event.Count}
		if seen := eventTime(event); !seen.IsZero() {
			item.LastSeen = seen.UTC().Format(time.RFC3339)
		}
		events = append(events, item)
	}
	return events, nil
}

func eventTime(event *corev1.Event) time.Time {
	switch {
	case !event.LastTimestamp.IsZero():
		return event.LastTimestamp.Time
	case !event.EventTime.IsZero():
		return event.EventTime.Time
	default:
		return event.CreationTimestamp.Time
	}
}

// podLogErrors returns the error lines of the log that explains a pod that
// is not ready: of its init container that has not finished, such as the
// model downloader, while it initializes, and otherwise of its Engine.
func podLogErrors(ctx context.Context, cs *cluster.ClientSet, namespace, name string, restarted bool) []string {
	var pod corev1.Pod
	if err := cs.K8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &pod); err != nil {
		klog.Warningf("Reading pod %s/%s for its init containers: %v", namespace, name, err)
		note := "pod could not be read, so its init containers are unknown: " + err.Error()
		return append([]string{note}, engineLogErrors(ctx, cs, namespace, name, "", restarted)...)
	}
	for _, status := range pod.Status.InitContainerStatuses {
		// An init container is ready once it exited 0.
		if status.Ready {
			continue
		}
		lines := engineLogErrors(ctx, cs, namespace, name, status.Name, status.RestartCount > 0)
		if len(lines) == 0 {
			lines = []string{initContainerState(status) + ", no error lines in its log"}
		}
		for i, line := range lines {
			lines[i] = "init container " + status.Name + ": " + line
		}
		return lines
	}
	return engineLogErrors(ctx, cs, namespace, name, "", restarted)
}

// initContainerState says what an unfinished init container is doing, as in
// "waiting (CrashLoopBackOff), 3 restarts, last exit code 1".
func initContainerState(status corev1.ContainerStatus) string {
	state := "running"
	if waiting := status.State.Waiting; waiting != nil {
		state = "waiting (" + waiting.Reason + ")"
	} else if terminated := status.State.Terminated; terminated != nil {
		state = fmt.Sprintf("terminated (%s, exit code %d)", terminated.Reason, terminated.ExitCode)
	}
	state += fmt.Sprintf(", %d restarts", status.RestartCount)
	if last := status.LastTerminationState.Terminated; last != nil {
		state += fmt.Sprintf(", last exit code %d", last.ExitCode)
	}
	return state
}

// engineLogErrors returns the last error lines of a pod's log, of container
// or else its Engine, and of the run before the last restart when previous
// is set. A log that cannot be read, as of a pod still pending, is reported
// as its only line.
func engineLogErrors(ctx context.Context, cs *cluster.ClientSet, namespace, pod, container string, previous bool) []string {
	tailLines := int64(engineLogTailLines)
	stream, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).
		GetLogs(pod, &corev1.PodLogOptions{Container: container, TailLines: &tailLines, Previous: previous}).Stream(ctx)
	if err != nil {
		return []string{"logs unavailable: " + err.Error()}
	}
	defer func() {
		if err := stream.Close(); err != nil {
			klog.Warningf("Failed to close pod log stream for %s/%s: %v", namespace, pod, err)
		}
	}()
	lines := []string{}
	scanner := bufio.NewScanner(io.LimitReader(stream, maxEngineLogBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), maxEngineLogBytes)
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if !slices.ContainsFunc(engineErrorWords, func(word string) bool { return strings.Contains(lower, word) }) {
			continue
		}
		if len(line) > maxEngineLogLineBytes {
			line = line[:maxEngineLogLineBytes] + "..."
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		lines = append(lines, "reading logs failed: "+err.Error())
	}
	return lines[max(0, len(lines)-maxEngineLogErrors):]
}

func toolJSON(value any) (string, bool) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "Error: " + err.Error(), true
	}
	return string(data), false
}
