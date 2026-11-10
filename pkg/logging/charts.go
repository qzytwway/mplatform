package logging

import (
	"bytes"
	_ "embed"
	"fmt"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

var (
	//go:embed charts/victoria-logs-single-0.13.12.tgz
	victoriaLogsChartArchive []byte
	//go:embed charts/vector-0.61.0.tgz
	vectorChartArchive []byte
)

func loadVictoriaLogsChart() (*chart.Chart, error) {
	return loadEmbeddedChart(victoriaLogsChartArchive)
}

func loadVectorChart() (*chart.Chart, error) {
	return loadEmbeddedChart(vectorChartArchive)
}

func loadEmbeddedChart(archive []byte) (*chart.Chart, error) {
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded chart: %w", err)
	}
	return loadedChart, nil
}

// requiredImages lists the images an offline registry must provide, before the
// cluster image registry rewrite is applied.
func requiredImages() ([]string, error) {
	victoriaLogsChart, err := loadVictoriaLogsChart()
	if err != nil {
		return nil, err
	}
	vectorChart, err := loadVectorChart()
	if err != nil {
		return nil, err
	}
	return []string{
		"victoriametrics/victoria-logs:" + victoriaLogsChart.Metadata.AppVersion,
		"timberio/vector:" + vectorChart.Metadata.AppVersion,
	}, nil
}
