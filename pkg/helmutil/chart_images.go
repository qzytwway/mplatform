package helmutil

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

type ChartImagesOptions struct {
	ReleaseName string
	Namespace   string
	// KubeVersion is the cluster version templates see, for charts whose
	// images depend on it; Helm's default otherwise.
	KubeVersion string
}

var (
	chartImagesMu    sync.Mutex
	chartImagesCache = map[string][]string{}
)

// ChartImages renders a chart without a cluster, with the values it is
// installed with, and lists the images it deploys, hooks included: those the
// image registry rewrite moves, which an offline registry must provide.
// Templates that check the cluster's API versions render as for a bare
// cluster. Results are kept per chart version, values and Kubernetes version.
func ChartImages(ctx context.Context, loadedChart *chart.Chart, values map[string]any, opts ChartImagesOptions) ([]string, error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	key := loadedChart.Metadata.Name + "@" + loadedChart.Metadata.Version + "@" + opts.KubeVersion + "@" + string(encoded)
	chartImagesMu.Lock()
	defer chartImagesMu.Unlock()
	if images, ok := chartImagesCache[key]; ok {
		return images, nil
	}

	install := action.NewInstall(action.NewConfiguration())
	install.ReleaseName = opts.ReleaseName
	install.Namespace = opts.Namespace
	install.DryRunStrategy = action.DryRunClient
	if opts.KubeVersion != "" {
		kubeVersion, err := common.ParseKubeVersion(opts.KubeVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid Kubernetes version %s: %w", opts.KubeVersion, err)
		}
		install.KubeVersion = kubeVersion
	}
	collector := &imageCollector{}
	install.PostRenderer = collector
	if _, err := install.RunWithContext(ctx, loadedChart, values); err != nil {
		return nil, fmt.Errorf("failed to render the %s chart: %w", loadedChart.Metadata.Name, err)
	}
	slices.Sort(collector.images)
	chartImagesCache[key] = collector.images
	return collector.images, nil
}
