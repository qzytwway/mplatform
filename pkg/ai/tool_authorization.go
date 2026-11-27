package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	pkgmodel "github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
)

type toolPermission struct {
	Resource  string
	Verb      string
	Namespace string
}

func permissionNamespace(resource resourceInfo, namespace string) string {
	if resource.ClusterScoped {
		return ""
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return common.AllNamespaces
	}
	return namespace
}

func requiredToolPermissions(ctx context.Context, cs *cluster.ClientSet, toolName string, args map[string]interface{}) ([]toolPermission, error) {
	switch toolName {
	case "get_resource", "describe_resource":
		kind, err := getRequiredString(args, "kind")
		if err != nil {
			return nil, err
		}
		namespace, _ := args["namespace"].(string)
		resource := resolveResourceInfo(ctx, cs, kind)
		return []toolPermission{{
			Resource:  common.HistoryResourceType(resource.Resource, resource.Group),
			Verb:      string(common.VerbGet),
			Namespace: permissionNamespace(resource, namespace),
		}}, nil
	case "list_resources":
		kind, err := getRequiredString(args, "kind")
		if err != nil {
			return nil, err
		}
		namespace, _ := args["namespace"].(string)
		resource := resolveResourceInfo(ctx, cs, kind)
		return []toolPermission{{
			Resource:  common.HistoryResourceType(resource.Resource, resource.Group),
			Verb:      string(common.VerbGet),
			Namespace: permissionNamespace(resource, namespace),
		}}, nil
	case "get_pod_logs":
		if _, err := getRequiredString(args, "name"); err != nil {
			return nil, err
		}
		namespace, err := getRequiredString(args, "namespace")
		if err != nil {
			return nil, err
		}
		return []toolPermission{{
			Resource:  string(common.Pods),
			Verb:      string(common.VerbLog),
			Namespace: namespace,
		}}, nil
	case "exec_in_pod":
		options, err := parseExecInPodOptions(args)
		if err != nil {
			return nil, err
		}
		return []toolPermission{{
			Resource:  string(common.Pods),
			Verb:      string(common.VerbExec),
			Namespace: options.Namespace,
		}}, nil
	case "get_cluster_overview":
		return []toolPermission{
			{Resource: string(common.Nodes), Verb: string(common.VerbGet), Namespace: ""},
			{Resource: string(common.Pods), Verb: string(common.VerbGet), Namespace: common.AllNamespaces},
			{Resource: string(common.Namespaces), Verb: string(common.VerbGet), Namespace: ""},
			{Resource: string(common.Services), Verb: string(common.VerbGet), Namespace: common.AllNamespaces},
		}, nil
	case "query_prometheus":
		// Prometheus queries can read metrics from any namespace, so require
		// the same permission as the HTTP query endpoint (pkg/metrics/query.go).
		return []toolPermission{{
			Resource:  "prometheus",
			Verb:      string(common.VerbGet),
			Namespace: common.AllNamespaces,
		}}, nil
	default:
		return nil, nil
	}
}

func currentUserFromGin(c *gin.Context) (pkgmodel.User, bool) {
	rawUser, ok := c.Get("user")
	if !ok {
		return pkgmodel.User{}, false
	}
	user, ok := rawUser.(pkgmodel.User)
	return user, ok
}

func AuthorizeTool(c *gin.Context, cs *cluster.ClientSet, toolName string, args map[string]interface{}) (string, bool) {
	if c == nil {
		return "Error: authorization context is required", true
	}
	if cs == nil {
		return "Error: cluster client is required", true
	}
	user, ok := currentUserFromGin(c)
	if !ok {
		return "Error: authenticated user not found in context", true
	}
	if result, isError := authorizeAdminOnlyTool(user, toolName); isError {
		return result, true
	}

	permissions, err := requiredToolPermissions(c.Request.Context(), cs, toolName, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	for _, permission := range permissions {
		// Same rule as the HTTP handlers: an all-namespaces request needs a
		// grant with no namespace exclusions.
		if permission.Namespace == common.AllNamespaces {
			if rbac.CanAccessAllNamespaces(user, permission.Resource, permission.Verb, cs.Name) {
				continue
			}
		} else if rbac.CanAccess(user, permission.Resource, permission.Verb, cs.Name, permission.Namespace) {
			continue
		}
		return "Forbidden: " + rbac.NoAccess(user.Key(), permission.Verb, permission.Resource, permission.Namespace, cs.Name), true
	}
	return "", false
}

// ExecuteTool runs a tool and returns the result as a string.
func ExecuteTool(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, toolName string, args map[string]interface{}) (string, bool) {
	if result, isError := AuthorizeTool(c, cs, toolName, args); isError {
		return result, true
	}

	user, _ := currentUserFromGin(c)

	switch toolName {
	case "get_resource":
		return executeGetResource(ctx, cs, args)
	case "describe_resource":
		return executeDescribeResource(ctx, cs, args)
	case "list_resources":
		return executeListResources(ctx, cs, args)
	case "get_pod_logs":
		return executeGetPodLogs(ctx, cs, args)
	case "exec_in_pod":
		return executeExecInPod(ctx, cs, user, args)
	case "get_cluster_overview":
		return executeGetClusterOverview(ctx, cs)
	case listInferenceServicesTool:
		return executeListInferenceServices(ctx, cs)
	case describeInferenceServiceTool:
		return executeDescribeInferenceService(ctx, cs, args)
	case inferenceServiceMetricsTool:
		return executeInferenceServiceMetrics(ctx, cs, args)
	case "query_prometheus":
		return executeQueryPrometheus(ctx, cs, args)
	default:
		return fmt.Sprintf("Unknown tool: %s", toolName), true
	}
}
