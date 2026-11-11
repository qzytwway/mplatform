package monitoring

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	helmcommon "helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const inferenceDashboardUID = "mplatform-inference-services"

// The parts of a dashboard the UI's renderer reads, see
// ui/src/components/observability/dashboard.
type dashboardDefinition struct {
	UID        string `json:"uid"`
	Title      string `json:"title"`
	Templating struct {
		List []struct {
			Name       string `json:"name"`
			Type       string `json:"type"`
			Query      string `json:"query"`
			IncludeAll bool   `json:"includeAll"`
			Multi      bool   `json:"multi"`
		} `json:"list"`
	} `json:"templating"`
	Panels []dashboardPanel `json:"panels"`
}

var (
	// The panel types and units of ui/src/components/observability/dashboard.
	renderedPanelTypes = []string{"timeseries", "stat", "gauge", "bargauge", "table"}
	renderedUnits      = []string{
		"percentunit", "percent", "s", "ms", "none", "short", "bytes", "decbytes",
		"bps", "Bps", "binBps", "pps", "ops", "reqps", "iops", "rds", "wps", "cps",
	}
	// dashboardLabelValues is the variable query form the UI resolves.
	dashboardLabelValues = regexp.MustCompile(`^\s*label_values\(\s*([\s\S]+),\s*(\w+)\s*\)\s*$`)
	// The vLLM and SGLang metrics the dashboard shows.
	inferenceMetrics = []string{
		"vllm:time_to_first_token_seconds_bucket", "sglang:time_to_first_token_seconds_bucket",
		"vllm:inter_token_latency_seconds_bucket", "sglang:inter_token_latency_seconds_bucket",
		"vllm:e2e_request_latency_seconds_bucket", "sglang:e2e_request_latency_seconds_bucket",
		"vllm:prompt_tokens_total", "sglang:prompt_tokens_total",
		"vllm:generation_tokens_total", "sglang:generation_tokens_total",
		"vllm:num_requests_running", "sglang:num_running_reqs",
		"vllm:num_requests_waiting", "sglang:num_queue_reqs",
		"vllm:kv_cache_usage_perc", "sglang:token_usage",
	}
	// pdRoleFilters leave out the pd role whose numbers would mislead: prefill
	// pods do not decode, and decode pods see every prompt again.
	pdRoleFilters = []struct{ metric, filter string }{
		{"_seconds_bucket", `inference_role!="prefill"`},
		{"generation_tokens_total", `inference_role!="prefill"`},
		{"prompt_tokens_total", `inference_role!="decode"`},
	}
)

type dashboardPanel struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	GridPos struct {
		X, Y, W, H int
	} `json:"gridPos"`
	FieldConfig struct {
		Defaults struct {
			Unit string `json:"unit"`
		} `json:"defaults"`
	} `json:"fieldConfig"`
	Targets []struct {
		Expr         string `json:"expr"`
		LegendFormat string `json:"legendFormat"`
	} `json:"targets"`
}

// inferenceDashboard reads the inference services dashboard from the
// built-in dashboards.
func inferenceDashboard(t *testing.T) dashboardDefinition {
	t.Helper()
	summaries, definitions, err := builtinDashboards()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != len(definitions) {
		t.Fatalf("got %d summaries for %d definitions", len(summaries), len(definitions))
	}
	index := slices.IndexFunc(summaries, func(summary dashboardSummary) bool { return summary.UID == inferenceDashboardUID })
	if index < 0 {
		t.Fatalf("built-in dashboards %v lack %s", summaries, inferenceDashboardUID)
	}
	if got := summaries[index].Title; got != "Model Serving / Inference Services" {
		t.Fatalf("title = %q", got)
	}
	var dashboard dashboardDefinition
	if err := json.Unmarshal(definitions[inferenceDashboardUID], &dashboard); err != nil {
		t.Fatal(err)
	}
	return dashboard
}

func TestInferenceDashboardVariables(t *testing.T) {
	var names []string
	for _, variable := range inferenceDashboard(t).Templating.List {
		names = append(names, variable.Name)
		if variable.Type != "query" || !dashboardLabelValues.MatchString(variable.Query) {
			t.Errorf("variable %s is not a label_values query: %q", variable.Name, variable.Query)
		}
		// Several services or roles are matched as a regular expression.
		if variable.Name != "namespace" && (!variable.IncludeAll || !variable.Multi) {
			t.Errorf("variable %s does not allow all and several values", variable.Name)
		}
	}
	if want := []string{"namespace", "service", "role"}; !slices.Equal(names, want) {
		t.Fatalf("variables = %v, want %v", names, want)
	}
}

func TestInferenceDashboardPanels(t *testing.T) {
	panels := inferenceDashboard(t).Panels
	if len(panels) == 0 {
		t.Fatal("the dashboard has no panels")
	}
	ids := map[int]bool{}
	for _, panel := range panels {
		if ids[panel.ID] {
			t.Errorf("panel id %d is used twice", panel.ID)
		}
		ids[panel.ID] = true
		checkInferencePanel(t, panel)
	}
}

// checkInferencePanel checks that the platform renders panel and that its queries
// follow the dashboard's variables for both runtimes.
func checkInferencePanel(t *testing.T, panel dashboardPanel) {
	t.Helper()
	if !slices.Contains(renderedPanelTypes, panel.Type) {
		t.Errorf("panel %q has type %q, which the platform does not render", panel.Title, panel.Type)
	}
	if !slices.Contains(renderedUnits, panel.FieldConfig.Defaults.Unit) {
		t.Errorf("panel %q has unit %q, which the platform does not format", panel.Title, panel.FieldConfig.Defaults.Unit)
	}
	if pos := panel.GridPos; pos.W <= 0 || pos.H <= 0 || pos.X < 0 || pos.X+pos.W > 24 {
		t.Errorf("panel %q is placed at %+v, outside the 24 column grid", panel.Title, pos)
	}
	if len(panel.Targets) == 0 {
		t.Errorf("panel %q has no queries", panel.Title)
	}
	for _, target := range panel.Targets {
		expr := target.Expr
		if strings.TrimSpace(expr) == "" {
			t.Errorf("panel %q has an empty query", panel.Title)
		}
		for _, filter := range []string{`namespace="$namespace"`, `inference_service=~"$service"`, `inference_role=~"$role"`} {
			if !strings.Contains(expr, filter) {
				t.Errorf("query of panel %q does not filter by %s: %s", panel.Title, filter, expr)
			}
		}
		if !strings.Contains(expr, "vllm:") || !strings.Contains(expr, "sglang:") {
			t.Errorf("query of panel %q does not read both vLLM and SGLang metrics: %s", panel.Title, expr)
		}
		checkPDRoleFilter(t, panel.Title, expr)
	}
}

// checkPDRoleFilter checks that a query leaves out the pd role it should, and
// only that one.
func checkPDRoleFilter(t *testing.T, title, expr string) {
	t.Helper()
	if strings.Contains(expr, "prompt_tokens_total") && strings.Contains(expr, "generation_tokens_total") {
		t.Errorf("query of panel %q adds up input and output tokens: %s", title, expr)
	}
	want := ""
	for _, rule := range pdRoleFilters {
		if strings.Contains(expr, rule.metric) {
			want = rule.filter
		}
	}
	if want != "" && !strings.Contains(expr, want) {
		t.Errorf("query of panel %q does not filter by %s: %s", title, want, expr)
	}
	others := strings.Count(expr, "inference_role!=")
	if want != "" {
		others -= strings.Count(expr, want)
	}
	if others > 0 {
		t.Errorf("query of panel %q leaves out other roles than %q: %s", title, want, expr)
	}
}

func TestInferenceDashboardMetrics(t *testing.T) {
	var exprs []string
	for _, panel := range inferenceDashboard(t).Panels {
		for _, target := range panel.Targets {
			exprs = append(exprs, target.Expr)
		}
	}
	all := strings.Join(exprs, "\n")
	for _, metric := range inferenceMetrics {
		if !regexp.MustCompile(`[\s"|]` + regexp.QuoteMeta(metric) + `[|"]`).MatchString(all) {
			t.Errorf("no panel shows %s", metric)
		}
	}
}

// TestLoadDashboardsListsBuiltins loads the dashboards the way the metrics
// page does, from a local kube-prometheus-stack chart with one dashboard.
func TestLoadDashboardsListsBuiltins(t *testing.T) {
	chartDir := t.TempDir()
	setupDashboardRepository(t, chartDir)
	chartDashboard := `{"uid": "chart-dashboard", "title": "Kubernetes / Chart"}`
	saveDashboardChart(t, chartDir, `apiVersion: v1
kind: ConfigMap
metadata:
  name: chart-dashboards
  labels:
    grafana_dashboard: "1"
data:
  chart.json: '`+chartDashboard+`'
`)
	dashboardsMu.Lock()
	dashboardsCache = nil
	dashboardsMu.Unlock()
	t.Cleanup(func() {
		dashboardsMu.Lock()
		dashboardsCache = nil
		dashboardsMu.Unlock()
	})

	dashboards, err := loadDashboards()
	if err != nil {
		t.Fatal(err)
	}
	var uids []string
	for _, summary := range dashboards.summaries {
		uids = append(uids, summary.UID)
	}
	if want := []string{"chart-dashboard", inferenceDashboardUID}; !slices.Equal(uids, want) {
		t.Fatalf("listed dashboards = %v, want %v sorted by title", uids, want)
	}
	_, builtins, err := builtinDashboards()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(dashboards.definitions[inferenceDashboardUID]); got != string(builtins[inferenceDashboardUID]) {
		t.Fatalf("definition of %s = %s", inferenceDashboardUID, got)
	}
	if got := string(dashboards.definitions["chart-dashboard"]); got != chartDashboard {
		t.Fatalf("definition of chart-dashboard = %s", got)
	}
}

// TestDashboardsWithoutChart lists and serves the built-in dashboards, and
// the cluster's, when the local chart directory lacks kube-prometheus-stack.
func TestDashboardsWithoutChart(t *testing.T) {
	setupDashboardRepository(t, t.TempDir())
	clusterDashboard := `{"uid": "cluster-dashboard", "title": "GPU / Cluster"}`
	cs := &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dashboards", Namespace: "gpu-operator", Labels: map[string]string{dashboardLabel: "1"}},
		Data:       map[string]string{"gpu.json": clusterDashboard},
	}).Build()}}
	_, builtins, err := builtinDashboards()
	if err != nil {
		t.Fatal(err)
	}

	recorder := serveDashboards(t, cs, ListDashboards, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("listing returned %d: %s", recorder.Code, recorder.Body)
	}
	var list struct {
		Chart      map[string]string  `json:"chart"`
		Dashboards []dashboardSummary `json:"dashboards"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"name": chartName}; !maps.Equal(list.Chart, want) {
		t.Fatalf("chart = %v, want %v with no version", list.Chart, want)
	}
	var uids []string
	for _, summary := range list.Dashboards {
		uids = append(uids, summary.UID)
	}
	if want := []string{"cluster-dashboard", inferenceDashboardUID}; !slices.Equal(uids, want) {
		t.Fatalf("listed dashboards = %v, want %v", uids, want)
	}

	for _, tt := range []struct {
		uid      string
		wantCode int
		wantBody string
	}{
		{inferenceDashboardUID, http.StatusOK, string(builtins[inferenceDashboardUID])},
		{"cluster-dashboard", http.StatusOK, clusterDashboard},
		{"missing", http.StatusNotFound, ""},
	} {
		recorder := serveDashboards(t, cs, GetDashboard, tt.uid)
		if recorder.Code != tt.wantCode {
			t.Fatalf("getting %s returned %d, want %d: %s", tt.uid, recorder.Code, tt.wantCode, recorder.Body)
		}
		if tt.wantBody != "" && recorder.Body.String() != tt.wantBody {
			t.Fatalf("got %s for %s", recorder.Body, tt.uid)
		}
	}
}

// serveDashboards calls a dashboard handler as a user who may query
// Prometheus.
func serveDashboards(t *testing.T, cs *cluster.ClientSet, handler gin.HandlerFunc, uid string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/prometheus/dashboards", nil)
	c.Params = gin.Params{{Key: "uid", Value: uid}}
	c.Set("cluster", cs)
	c.Set("user", model.User{Username: "viewer", Roles: []common.Role{{
		Name: "viewer", Clusters: []string{"*"}, Namespaces: []string{"*"}, Resources: []string{"prometheus"}, Verbs: []string{"get"},
	}}})
	handler(c)
	return recorder
}

func setupDashboardRepository(t *testing.T, chartDir string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.HelmRepository{}); err != nil {
		t.Fatal(err)
	}
	repository := model.HelmRepository{
		Name: "local",
		Type: model.HelmRepositoryTypeLocal,
		URL:  (&url.URL{Scheme: "file", Path: chartDir}).String(),
	}
	if err := db.Create(&repository).Error; err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		_ = sqlDB.Close()
	})
}

func saveDashboardChart(t *testing.T, dir, template string) {
	t.Helper()
	localChart := &chart.Chart{
		Metadata:  &chart.Metadata{APIVersion: chart.APIVersionV2, Name: chartName, Version: "1.0.0"},
		Templates: []*helmcommon.File{{Name: "templates/dashboards.yaml", Data: []byte(template)}},
	}
	if _, err := chartutil.Save(localChart, dir); err != nil {
		t.Fatal(err)
	}
}
