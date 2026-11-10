package logging

import (
	"errors"
	"net/http"

	"github.com/qzytwway/mplatform/pkg/helmutil"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

const (
	victoriaLogsChartName = "victoria-logs-single"
	vectorChartName       = "vector"
)

// chartStatus reports whether a chart the logging stack needs is in the local
// chart repository, and the image an offline registry must provide for it
// before the cluster image registry rewrite is applied.
type chartStatus struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
}

func loadVictoriaLogsChart() (*chart.Chart, error) {
	return helmutil.LoadLocalChart(victoriaLogsChartName)
}

func loadVectorChart() (*chart.Chart, error) {
	return helmutil.LoadLocalChart(vectorChartName)
}

func chartStatuses() ([]chartStatus, error) {
	required := []struct{ name, imageRepository string }{
		{victoriaLogsChartName, "victoriametrics/victoria-logs"},
		{vectorChartName, "timberio/vector"},
	}
	statuses := make([]chartStatus, 0, len(required))
	for _, r := range required {
		status := chartStatus{Name: r.name}
		loadedChart, err := helmutil.LoadLocalChart(r.name)
		switch {
		case errors.Is(err, helmutil.ErrLocalChartNotFound):
		case err != nil:
			return nil, err
		default:
			status.Version = loadedChart.Metadata.Version
			status.Image = r.imageRepository + ":" + loadedChart.Metadata.AppVersion
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func chartErrorStatus(err error) int {
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
