package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/monitoring"
	"golang.org/x/sync/errgroup"
)

const (
	inferenceServiceMetricsTool = "get_inference_service_metrics"
	inferenceMetricsWindow      = 30 * time.Minute
	inferenceMetricsStep        = time.Minute
	// inferenceMetricsRateInterval is the $__rate_interval the console's
	// dashboard renderer derives for a 1m step (ui/src/components/observability/dashboard/variables.ts).
	inferenceMetricsRateInterval = "2m"
)

func inferenceServiceMetricsToolDefinition() agentToolDefinition {
	return agentToolDefinition{
		Name:        inferenceServiceMetricsTool,
		Description: "Read the inference metrics of one Inference Service over the last 30 minutes, from the panels of the built-in Inference Services monitoring dashboard: time to first token, inter-token latency (time per output token), token throughput, running and queued requests, and KV cache usage, per Role. Each series gives its value at the start of the window, its latest value, min and max. A panel without series has no data, such as the latency of a service that got no requests, unless it has an error, which means its Prometheus query failed and says nothing about the service. Use this to tell whether an Inference Service is slow or overloaded. Admin only.",
		Properties: map[string]any{
			"namespace": map[string]any{"type": "string", "description": "The namespace of the Inference Service."},
			"name":      map[string]any{"type": "string", "description": "The name of the Inference Service."},
		},
		Required: []string{"namespace", "name"},
	}
}

type inferenceMetricsSeries struct {
	Legend string  `json:"legend"`
	Start  float64 `json:"start"`
	Latest float64 `json:"latest"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type inferenceMetricsPanel struct {
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Unit        string                   `json:"unit"`
	Error       string                   `json:"error,omitempty"`
	Series      []inferenceMetricsSeries `json:"series"`
}

type inferenceMetrics struct {
	Namespace string                  `json:"namespace"`
	Name      string                  `json:"name"`
	Window    string                  `json:"window"`
	Panels    []inferenceMetricsPanel `json:"panels"`
}

// inferenceDashboard is the part of the dashboard definition the tool runs.
type inferenceDashboard struct {
	Panels []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		FieldConfig struct {
			Defaults struct {
				Unit string `json:"unit"`
			} `json:"defaults"`
		} `json:"fieldConfig"`
		Targets []struct {
			Expr         string `json:"expr"`
			LegendFormat string `json:"legendFormat"`
		} `json:"targets"`
	} `json:"panels"`
}

func executeInferenceServiceMetrics(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	if cs.PromClient == nil {
		return "Prometheus is not configured for this cluster, so inference metrics cannot be read. An administrator can set the Prometheus URL in the cluster settings.", false
	}
	detail, result, isError := lookupInferenceService(ctx, cs, args)
	if isError {
		return result, true
	}
	definition, err := monitoring.InferenceServiceDashboard()
	if err != nil {
		return "Error reading the inference services dashboard: " + err.Error(), true
	}
	var dashboard inferenceDashboard
	if err := json.Unmarshal(definition, &dashboard); err != nil {
		return "Error reading the inference services dashboard: " + err.Error(), true
	}
	// The dashboard with this service and all its roles selected; $service is
	// matched as a regular expression inside a PromQL string.
	variables := strings.NewReplacer(
		"$namespace", detail.Namespace,
		"$service", strings.ReplaceAll(regexp.QuoteMeta(detail.Name), `\`, `\\`),
		"$role", ".*",
		"$__rate_interval", inferenceMetricsRateInterval,
	)
	results, firstErr := queryDashboard(ctx, cs, &dashboard, variables)
	metrics := inferenceMetrics{Namespace: detail.Namespace, Name: detail.Name, Window: "30m"}
	hasData, queries, failed := false, 0, 0
	for i, panel := range dashboard.Panels {
		out := inferenceMetricsPanel{Title: panel.Title, Description: panel.Description, Unit: panel.FieldConfig.Defaults.Unit, Series: []inferenceMetricsSeries{}}
		var errs []string
		for _, result := range results[i] {
			queries++
			if result.err != nil {
				failed++
				errs = append(errs, result.err.Error())
				continue
			}
			out.Series = append(out.Series, result.series...)
		}
		if len(errs) > 0 {
			out.Error = "query failed: " + strings.Join(errs, "; ")
		}
		hasData = hasData || len(out.Series) > 0
		metrics.Panels = append(metrics.Panels, out)
	}
	if failed > 0 && failed == queries {
		return fmt.Sprintf("Error querying Prometheus for the inference metrics of %s/%s: %v", detail.Namespace, detail.Name, firstErr), true
	}
	if !hasData && failed == 0 {
		return fmt.Sprintf("Prometheus has no inference metrics of %s/%s in the last 30 minutes: its pods have not been scraped yet, or its Engine does not expose metrics.", detail.Namespace, detail.Name), false
	}
	return toolJSON(metrics)
}

type dashboardQueryResult struct {
	series []inferenceMetricsSeries
	err    error
}

// queryDashboard runs the queries of every panel over the last half hour,
// concurrently, and returns their results by panel and target, with the
// first error. A failed query does not stop the others.
func queryDashboard(ctx context.Context, cs *cluster.ClientSet, dashboard *inferenceDashboard, variables *strings.Replacer) ([][]dashboardQueryResult, error) {
	end := time.Now()
	window := v1.Range{Start: end.Add(-inferenceMetricsWindow), End: end, Step: inferenceMetricsStep}
	// Each query fills its own slot, so the panels keep the dashboard's order.
	results := make([][]dashboardQueryResult, len(dashboard.Panels))
	var group errgroup.Group
	group.SetLimit(maxConcurrentReads)
	for i, panel := range dashboard.Panels {
		results[i] = make([]dashboardQueryResult, len(panel.Targets))
		for j, target := range panel.Targets {
			group.Go(func() error {
				value, _, err := cs.PromClient.QueryRange(ctx, variables.Replace(target.Expr), window)
				if err != nil {
					results[i][j].err = err
					return err
				}
				matrix, _ := value.(model.Matrix)
				for _, stream := range matrix {
					if series, ok := summarizeSeries(stream, target.LegendFormat); ok {
						results[i][j].series = append(results[i][j].series, series)
					}
				}
				return nil
			})
		}
	}
	return results, group.Wait()
}

// summarizeSeries names a series by the panel's legend and reduces it to its
// first, latest, min and max values, leaving out the NaN and infinite samples
// of histogram quantiles without requests. It reports false for no samples.
func summarizeSeries(stream *model.SampleStream, legendFormat string) (inferenceMetricsSeries, bool) {
	series := inferenceMetricsSeries{Legend: legendFormat}
	for name, value := range stream.Metric {
		series.Legend = strings.ReplaceAll(series.Legend, "{{"+string(name)+"}}", string(value))
	}
	found := false
	for _, sample := range stream.Values {
		value := float64(sample.Value)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		if !found {
			series.Start, series.Min, series.Max, found = value, value, value, true
		}
		series.Latest = value
		series.Min = min(series.Min, value)
		series.Max = max(series.Max, value)
	}
	return series, found
}
