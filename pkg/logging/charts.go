package logging

import (
	"context"
	"errors"
	"net/http"

	"github.com/qzytwway/mplatform/pkg/helmutil"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

const (
	victoriaLogsChartName = "victoria-logs-single"
	vectorChartName       = "vector"
	// The chart versions the platform is verified with; the README lists
	// them too.
	testedVictoriaLogsVersion = "0.13.12"
	testedVectorVersion       = "0.61.0"

	// VMAlertImage runs the vmalert that evaluates log alert rules, which the
	// platform deploys without a chart when the first rule is saved.
	VMAlertImage = "victoriametrics/vmalert:v1.155.0"
)

// imageStatus is an image the logging stack deploys without a chart, which
// an offline registry must also provide.
type imageStatus struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

var chartlessImages = []imageStatus{{Name: "vmalert", Image: VMAlertImage}}

// chartStatus reports whether a chart the logging stack needs is in the local
// chart repository, the version the platform is verified with, and the images
// it deploys before the cluster image registry rewrite is applied, or why they
// cannot be listed.
type chartStatus struct {
	Name          string   `json:"name"`
	Version       string   `json:"version,omitempty"`
	TestedVersion string   `json:"testedVersion,omitempty"`
	Images        []string `json:"images,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func loadVictoriaLogsChart() (*chart.Chart, error) {
	return helmutil.LoadLocalChart(victoriaLogsChartName)
}

func loadVectorChart() (*chart.Chart, error) {
	return helmutil.LoadLocalChart(vectorChartName)
}

// chartStatuses lists the images each chart deploys with the values the
// platform installs it with; the cluster name only labels the logs.
func chartStatuses(ctx context.Context) ([]chartStatus, error) {
	required := []struct {
		name, release, testedVersion string
		values                       map[string]any
	}{
		{victoriaLogsChartName, victoriaLogsRelease, testedVictoriaLogsVersion, victoriaLogsValues(storageOptions{StorageSize: defaultStorageSize, Retention: defaultRetention})},
		{vectorChartName, vectorRelease, testedVectorVersion, vectorValues(collectorOptions{ClusterName: "cluster"})},
	}
	statuses := make([]chartStatus, 0, len(required))
	for _, r := range required {
		status := chartStatus{Name: r.name, TestedVersion: r.testedVersion}
		loadedChart, err := helmutil.LoadLocalChart(r.name)
		switch {
		case errors.Is(err, helmutil.ErrLocalChartNotFound):
		case err != nil:
			return nil, err
		default:
			status.Version = loadedChart.Metadata.Version
			status.Images, err = helmutil.ChartImages(ctx, loadedChart, r.values, helmutil.ChartImagesOptions{ReleaseName: r.release, Namespace: Namespace})
			if err != nil {
				status.Error = err.Error()
			}
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
