package modelserving

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prommodel "github.com/prometheus/common/model"
	"github.com/qzytwway/mplatform/pkg/cluster"
)

const (
	// The counters the gateway's ai-statistics plugin keeps per route,
	// upstream, model and caller. Durations are in milliseconds.
	inputTokenMetric  = "route_upstream_model_consumer_metric_input_token"
	outputTokenMetric = "route_upstream_model_consumer_metric_output_token"
	requestMetric     = "route_upstream_model_consumer_metric_llm_duration_count"
	durationMetric    = "route_upstream_model_consumer_metric_llm_service_duration"
	callerLabel       = "ai_consumer"
	modelLabel        = "ai_model"
	// usageCallers leaves out requests without a caller and the gateway's
	// own consumers, none of which is a DNS label.
	usageCallers = `{ai_consumer!~"|none|_.*"}`

	usageQueryTimeout = 30 * time.Second
	maxUsageRange     = 90 * 24 * time.Hour
	maxUsagePoints    = 11000
	// minUsageRateWindow is in seconds.
	minUsageRateWindow = 120
)

type usageParams struct {
	Start *int64 `form:"start" binding:"required,gte=0"`
	End   *int64 `form:"end" binding:"required,gte=0"`
	Step  int64  `form:"step" binding:"gte=0,lte=86400"`
}

type usageItem struct {
	Caller        string  `json:"caller"`
	Model         string  `json:"model"`
	Requests      float64 `json:"requests"`
	InputTokens   float64 `json:"inputTokens"`
	OutputTokens  float64 `json:"outputTokens"`
	AvgDurationMs float64 `json:"avgDurationMs"`
}

type usageSeries struct {
	Caller string       `json:"caller"`
	Points [][2]float64 `json:"points"`
}

// bindUsage resolves the cluster and the queried period, writing the error
// response when either is invalid.
func (h *Handler) bindUsage(c *gin.Context) (*cluster.ClientSet, time.Time, time.Time, usageParams, bool) {
	var params usageParams
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err == nil {
		err = c.ShouldBindQuery(&params)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, time.Time{}, time.Time{}, params, false
	}
	start, end := time.Unix(*params.Start, 0), time.Unix(*params.End, 0)
	if !start.Before(end) || end.Sub(start) > maxUsageRange {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must precede end by at most 90 days"})
		return nil, time.Time{}, time.Time{}, params, false
	}
	return cs, start, end, params, true
}

func (h *Handler) GetUsage(c *gin.Context) {
	cs, start, end, _, ok := h.bindUsage(c)
	if !ok {
		return
	}
	if cs.PromClient == nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "items": []usageItem{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), usageQueryTimeout)
	defer cancel()
	type key struct{ caller, model string }
	items := map[key]*usageItem{}
	durations := map[key]float64{}
	for _, metric := range []string{inputTokenMetric, outputTokenMetric, requestMetric, durationMetric} {
		query := fmt.Sprintf("sum by (%s, %s) (increase(%s%s[%ds]))", callerLabel, modelLabel, metric, usageCallers, int64(end.Sub(start).Seconds()))
		result, _, err := cs.PromClient.Query(ctx, query, end)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		vector, _ := result.(prommodel.Vector)
		for _, sample := range vector {
			k := key{string(sample.Metric[callerLabel]), string(sample.Metric[modelLabel])}
			if items[k] == nil {
				items[k] = &usageItem{Caller: k.caller, Model: k.model}
			}
			// increase extrapolates, so counts come back fractional.
			value := math.Round(float64(sample.Value))
			switch metric {
			case inputTokenMetric:
				items[k].InputTokens = value
			case outputTokenMetric:
				items[k].OutputTokens = value
			case requestMetric:
				items[k].Requests = value
			case durationMetric:
				durations[k] = float64(sample.Value)
			}
		}
	}
	list := make([]usageItem, 0, len(items))
	for k, item := range items {
		// Counters that did not move in the period still come back as zero.
		if item.Requests == 0 && item.InputTokens == 0 && item.OutputTokens == 0 {
			continue
		}
		if item.Requests > 0 {
			item.AvgDurationMs = math.Round(durations[k] / item.Requests)
		}
		list = append(list, *item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Caller != list[j].Caller {
			return list[i].Caller < list[j].Caller
		}
		return list[i].Model < list[j].Model
	})
	c.JSON(http.StatusOK, gin.H{"configured": true, "items": list})
}

func (h *Handler) GetUsageSeries(c *gin.Context) {
	cs, start, end, params, ok := h.bindUsage(c)
	if !ok {
		return
	}
	if params.Step == 0 || (*params.End-*params.Start)/params.Step > maxUsagePoints {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("step must be a positive number of seconds that splits the period into at most %d points", maxUsagePoints)})
		return
	}
	if cs.PromClient == nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "series": []usageSeries{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), usageQueryTimeout)
	defer cancel()
	// Embedding requests never create the output counter, so the two are
	// queried apart and added here.
	tokens := map[string]map[float64]float64{}
	for _, metric := range []string{inputTokenMetric, outputTokenMetric} {
		// A step as short as the scrape interval holds too few samples to
		// measure an increase, so the rate over a wider window is scaled to it.
		window := max(params.Step, minUsageRateWindow)
		query := fmt.Sprintf("sum by (%s) (rate(%s%s[%ds])) * %d", callerLabel, metric, usageCallers, window, params.Step)
		result, _, err := cs.PromClient.QueryRange(ctx, query, v1.Range{Start: start, End: end, Step: time.Duration(params.Step) * time.Second})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		matrix, _ := result.(prommodel.Matrix)
		for _, stream := range matrix {
			caller := string(stream.Metric[callerLabel])
			if tokens[caller] == nil {
				tokens[caller] = map[float64]float64{}
			}
			for _, sample := range stream.Values {
				tokens[caller][float64(sample.Timestamp.Unix())] += float64(sample.Value)
			}
		}
	}
	series := make([]usageSeries, 0, len(tokens))
	for caller, byTime := range tokens {
		points := make([][2]float64, 0, len(byTime))
		for timestamp, value := range byTime {
			points = append(points, [2]float64{timestamp, math.Round(value)})
		}
		sort.Slice(points, func(i, j int) bool { return points[i][0] < points[j][0] })
		series = append(series, usageSeries{Caller: caller, Points: points})
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Caller < series[j].Caller })
	c.JSON(http.StatusOK, gin.H{"configured": true, "series": series})
}
