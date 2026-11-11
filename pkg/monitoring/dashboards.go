package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"helm.sh/helm/v4/pkg/action"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// dashboardLabel marks the ConfigMaps the chart writes for Grafana's sidecar.
const dashboardLabel = "grafana_dashboard"

type dashboardSummary struct {
	UID   string   `json:"uid"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

type chartDashboards struct {
	version     string
	summaries   []dashboardSummary
	definitions map[string]json.RawMessage
}

var (
	dashboardsMu    sync.Mutex
	dashboardsCache *chartDashboards
)

func RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/prometheus/dashboards", ListDashboards)
	api.GET("/prometheus/dashboards/:uid", GetDashboard)
	api.POST("/prometheus/alert-rules", CreateAlertRule)
	api.PUT("/prometheus/alert-rules/:namespace/:name", UpdateAlertRule)
	api.DELETE("/prometheus/alert-rules/:namespace/:name", DeleteAlertRule)
	api.PUT("/prometheus/builtin-rules/:alert", SetBuiltinRuleEnabled)
	api.POST("/prometheus/builtin-rules/:alert/override", OverrideBuiltinRule)
	api.GET("/alertmanager/notifications", GetAlertNotifications)
	api.PUT("/alertmanager/notifications", UpdateAlertNotifications)
	api.POST("/alertmanager/notifications/channels/:id/test", SendTestNotification)
}

// loadDashboards renders the local chart the way it deploys dashboards for
// Grafana and keeps them for the chart version. The dashboards only depend on
// the chart, so rendering needs no cluster.
func loadDashboards() (*chartDashboards, error) {
	loadedChart, err := helmutil.LoadLocalChart(chartName)
	if err != nil {
		return nil, err
	}
	dashboardsMu.Lock()
	defer dashboardsMu.Unlock()
	if dashboardsCache != nil && dashboardsCache.version == loadedChart.Metadata.Version {
		return dashboardsCache, nil
	}
	values := map[string]any{"grafana": map[string]any{"enabled": false, "forceDeployDashboards": true}}
	rel, err := helmutil.InstallRelease(context.Background(), action.NewConfiguration(), loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName: releaseName,
		Namespace:   Namespace,
		DryRun:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render the %s chart: %w", chartName, err)
	}
	dashboards := &chartDashboards{version: loadedChart.Metadata.Version, definitions: map[string]json.RawMessage{}}
	for _, manifest := range releaseutil.SplitManifests(rel.Manifest) {
		var configMap corev1.ConfigMap
		if err := yaml.Unmarshal([]byte(manifest), &configMap); err != nil {
			return nil, fmt.Errorf("failed to read the %s chart: %w", chartName, err)
		}
		if configMap.Kind != "ConfigMap" || configMap.Labels[dashboardLabel] == "" {
			continue
		}
		for key, definition := range configMap.Data {
			if !strings.HasSuffix(key, ".json") {
				continue
			}
			var summary dashboardSummary
			if err := json.Unmarshal([]byte(definition), &summary); err != nil || summary.UID == "" {
				return nil, fmt.Errorf("dashboard %s of the %s chart is not valid", key, chartName)
			}
			dashboards.summaries = append(dashboards.summaries, summary)
			dashboards.definitions[summary.UID] = json.RawMessage(definition)
		}
	}
	sort.Slice(dashboards.summaries, func(i, j int) bool {
		return dashboards.summaries[i].Title < dashboards.summaries[j].Title
	})
	dashboardsCache = dashboards
	return dashboards, nil
}

// canReadDashboards requires the grant that querying Prometheus needs, since
// the dashboards are only shown with query results.
func canReadDashboards(c *gin.Context) bool {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if !rbac.CanAccessAllNamespaces(user, "prometheus", string(common.VerbGet), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), "get", "prometheus", common.AllNamespaces, cs.Name)})
		return false
	}
	return true
}

func dashboardErrorStatus(err error) int {
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func ListDashboards(c *gin.Context) {
	if !canReadDashboards(c) {
		return
	}
	dashboards, err := loadDashboards()
	if err != nil {
		c.JSON(dashboardErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"chart":      chartStatus{Name: chartName, Version: dashboards.version},
		"dashboards": dashboards.summaries,
	})
}

func GetDashboard(c *gin.Context) {
	if !canReadDashboards(c) {
		return
	}
	dashboards, err := loadDashboards()
	if err != nil {
		c.JSON(dashboardErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	definition, ok := dashboards.definitions[c.Param("uid")]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "dashboard not found"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", definition)
}
