package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qzytwway/mplatform/pkg/prometheus"
)

// fakePrometheus answers every range query with the series that matrix returns
// for it, and records the queries it got.
type fakePrometheus struct {
	queries []string
	matrix  func(query string) []map[string]any
}

func newFakePrometheus(t *testing.T, matrix func(query string) []map[string]any) (*fakePrometheus, *prometheus.Client) {
	t.Helper()
	fp := &fakePrometheus{matrix: matrix}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.FormValue("query")
		fp.queries = append(fp.queries, query)
		result := []map[string]any{}
		if fp.matrix != nil {
			result = fp.matrix(query)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": result},
		})
	}))
	t.Cleanup(server.Close)
	client, err := prometheus.NewClientWithRoundTripper(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fp, client
}

// decodeSeries is one series of the decode role of qwen that went from 1 to
// 3 over the last half hour, with a NaN sample from an idle moment.
func decodeSeries(string) []map[string]any {
	now := time.Now().Unix()
	return []map[string]any{{
		"metric": map[string]string{"inference_service": "qwen", "inference_role": "decode", "pod": "qwen-decode-0"},
		"values": []any{
			[]any{now - 1800, "1"},
			[]any{now - 1200, "NaN"},
			[]any{now - 600, "4"},
			[]any{now, "3"},
		},
	}}
}

type metricsResult struct {
	Panels []struct {
		Title  string `json:"title"`
		Unit   string `json:"unit"`
		Series []struct {
			Legend string  `json:"legend"`
			Start  float64 `json:"start"`
			Latest float64 `json:"latest"`
			Min    float64 `json:"min"`
			Max    float64 `json:"max"`
		} `json:"series"`
	} `json:"panels"`
}

func TestInferenceServiceMetricsReturnsEachPanelPerRole(t *testing.T) {
	fc := newInferenceServiceCluster(t, lwsService("qwen", "single", 1, 1, 1))
	_, client := newFakePrometheus(t, decodeSeries)
	fc.cs.PromClient = client

	result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs,
		"get_inference_service_metrics", map[string]interface{}{"namespace": "default", "name": "qwen"})

	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	var metrics metricsResult
	if err := json.Unmarshal([]byte(result), &metrics); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, result)
	}
	legends := map[string][]string{}
	for _, panel := range metrics.Panels {
		for _, series := range panel.Series {
			legends[panel.Title] = append(legends[panel.Title], series.Legend)
			if series.Start != 1 || series.Latest != 3 || series.Min != 1 || series.Max != 4 {
				t.Errorf("%s %s: got %+v, want start 1, latest 3, min 1, max 4", panel.Title, series.Legend, series)
			}
		}
	}
	want := map[string][]string{
		"Time to First Token":         {"p50 qwen decode", "p95 qwen decode"},
		"Inter-Token Latency":         {"p50 qwen decode", "p95 qwen decode"},
		"Token Throughput (tokens/s)": {"input qwen decode", "output qwen decode"},
		"Running and Queued Requests": {"running qwen decode", "queued qwen decode"},
		"KV Cache Usage":              {"qwen-decode-0"},
	}
	for title, legend := range want {
		if !slices.Equal(legends[title], legend) {
			t.Errorf("%s: legends %v, want %v", title, legends[title], legend)
		}
	}
}

// TestInferenceServiceMetricsRunsTheDashboardQueries reads the queries from
// the dashboard definition itself, so editing a query there must change what
// the tool sends to Prometheus.
func TestInferenceServiceMetricsRunsTheDashboardQueries(t *testing.T) {
	fc := newInferenceServiceCluster(t, lwsService("qwen.v2", "single", 1, 1, 1))
	fp, client := newFakePrometheus(t, decodeSeries)
	fc.cs.PromClient = client
	file, err := os.ReadFile("../monitoring/dashboards/inference-services.json")
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		Panels []struct {
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(file, &dashboard); err != nil {
		t.Fatal(err)
	}
	// The dashboard with namespace default, service qwen.v2 and all roles
	// selected, over the 1m steps of the last half hour.
	values := strings.NewReplacer("$namespace", "default", "$service", `qwen\\.v2`, "$role", ".*", "$__rate_interval", "2m")
	var want []string
	for _, panel := range dashboard.Panels {
		for _, target := range panel.Targets {
			want = append(want, values.Replace(target.Expr))
		}
	}

	if result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs,
		"get_inference_service_metrics", map[string]interface{}{"namespace": "default", "name": "qwen.v2"}); isError {
		t.Fatalf("unexpected error: %s", result)
	}

	if !slices.Equal(fp.queries, want) {
		t.Fatalf("queries:\n%s\nwant:\n%s", strings.Join(fp.queries, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(fp.queries[0], `namespace="default", inference_service=~"qwen\\.v2", inference_role=~".*"`) {
		t.Fatalf("first query does not select the service: %s", fp.queries[0])
	}
}

func TestInferenceServiceMetricsIsAdminOnly(t *testing.T) {
	fc := newInferenceServiceCluster(t, lwsService("qwen", "single", 1, 1, 1))
	fp, client := newFakePrometheus(t, nil)
	fc.cs.PromClient = client

	result, isError := ExecuteTool(context.Background(), toolContext(readOnlyUser()), fc.cs,
		"get_inference_service_metrics", map[string]interface{}{"namespace": "default", "name": "qwen"})

	if !isError || !strings.HasPrefix(result, "Forbidden:") {
		t.Fatalf("want a Forbidden result, got isError=%v result=%q", isError, result)
	}
	if fc.reads != 0 || len(fp.queries) != 0 {
		t.Fatalf("a non-admin call read the cluster %d times and Prometheus %d times", fc.reads, len(fp.queries))
	}
}

func TestInferenceServiceMetricsWithoutData(t *testing.T) {
	fc := newInferenceServiceCluster(t, lwsService("qwen", "single", 1, 1, 1))
	_, client := newFakePrometheus(t, nil)
	tests := map[string]struct {
		prometheus *prometheus.Client
		want       string
	}{
		"no prometheus": {want: "Prometheus is not configured for this cluster"},
		"no data":       {prometheus: client, want: "Prometheus has no inference metrics of default/qwen in the last 30 minutes"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fc.cs.PromClient = tc.prometheus

			result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs,
				"get_inference_service_metrics", map[string]interface{}{"namespace": "default", "name": "qwen"})

			if isError || !strings.HasPrefix(result, tc.want) {
				t.Fatalf("want a result starting %q, got isError=%v result=%q", tc.want, isError, result)
			}
		})
	}
}
