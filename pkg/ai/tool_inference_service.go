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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	listInferenceServicesTool    = "list_inference_services"
	describeInferenceServiceTool = "describe_inference_service"
)

// adminOnlyTools read model serving, whose pages and APIs are admin-only.
var adminOnlyTools = map[string]bool{
	listInferenceServicesTool:    true,
	describeInferenceServiceTool: true,
}

func inferenceServiceToolDefinitions() []agentToolDefinition {
	return []agentToolDefinition{
		{
			Name:        listInferenceServicesTool,
			Description: "List the Inference Services (model serving) in the current cluster with their Model, Engine (runtime), Deployment Mode and each Role's desired and ready Replicas. Admin only.",
			Properties:  map[string]any{},
		},
		{
			Name:        describeInferenceServiceTool,
			Description: "Describe one Inference Service as its console detail page shows it: Model, Engine (runtime), Deployment Mode, each Role's desired and ready Replicas, its Pods (Leader pod / Worker pod), deployment settings, recent Pod events and the error lines of the Engine logs of Pods that are not ready or restarted. Use this to diagnose why an Inference Service is unhealthy. Admin only.",
			Properties: map[string]any{
				"namespace": map[string]any{"type": "string", "description": "The namespace of the Inference Service."},
				"name":      map[string]any{"type": "string", "description": "The name of the Inference Service."},
			},
			Required: []string{"namespace", "name"},
		},
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
	maxPodEvents          = 10
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
// recent events of its pods and the error lines in the Engine logs of pods
// that are not ready or restarted, both keyed by pod name.
type inferenceServiceDiagnosis struct {
	*modelserving.ServiceDetail
	Events          map[string][]podEvent `json:"events,omitempty"`
	EngineLogErrors map[string][]string   `json:"engineLogErrors,omitempty"`
}

func executeDescribeInferenceService(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	detail, result, isError := lookupInferenceService(ctx, cs, args)
	if isError {
		return result, true
	}
	diagnosis := inferenceServiceDiagnosis{ServiceDetail: detail, EngineLogErrors: map[string][]string{}}
	pods := map[string]bool{}
	for _, pod := range detail.Pods {
		pods[pod.Name] = true
		if pod.Ready && pod.Restarts == 0 {
			continue
		}
		if lines := engineLogErrors(ctx, cs, detail.Namespace, pod.Name, pod.Restarts > 0); len(lines) > 0 {
			diagnosis.EngineLogErrors[pod.Name] = lines
		}
	}
	events, err := podEvents(ctx, cs, detail.Namespace, pods)
	if err != nil {
		return fmt.Sprintf("Error listing events of inference service %s/%s: %v", detail.Namespace, detail.Name, err), true
	}
	diagnosis.Events = events
	return toolJSON(diagnosis)
}

// podEvents returns the latest events of each of pods, newest first.
func podEvents(ctx context.Context, cs *cluster.ClientSet, namespace string, pods map[string]bool) (map[string][]podEvent, error) {
	list, err := cs.K8sClient.ClientSet.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.kind=Pod"})
	if err != nil {
		return nil, err
	}
	items := list.Items
	sort.SliceStable(items, func(i, j int) bool { return eventTime(&items[i]).After(eventTime(&items[j])) })
	events := map[string][]podEvent{}
	for i := range items {
		event := &items[i]
		pod := event.InvolvedObject.Name
		if !pods[pod] || len(events[pod]) == maxPodEvents {
			continue
		}
		item := podEvent{Type: event.Type, Reason: event.Reason, Message: event.Message, Count: event.Count}
		if seen := eventTime(event); !seen.IsZero() {
			item.LastSeen = seen.UTC().Format(time.RFC3339)
		}
		events[pod] = append(events[pod], item)
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

// engineLogErrors returns the last error lines of a pod's Engine log, of the
// container that ran before the last restart when previous is set. A log that
// cannot be read, as of a pod still pending, is reported as its only line.
func engineLogErrors(ctx context.Context, cs *cluster.ClientSet, namespace, pod string, previous bool) []string {
	tailLines := int64(engineLogTailLines)
	stream, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).
		GetLogs(pod, &corev1.PodLogOptions{TailLines: &tailLines, Previous: previous}).Stream(ctx)
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
