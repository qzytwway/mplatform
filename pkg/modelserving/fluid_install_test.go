package modelserving

import (
	"context"
	"testing"

	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

// An offline registry must also hold the images Fluid's controllers start
// cache and FUSE pods from, which the chart only names in their environment.
func TestFluidChartListsRuntimeImages(t *testing.T) {
	loadedChart, err := loader.Load("../../chart-packages/fluid-" + testedFluidVersion + ".tgz")
	require.NoError(t, err)
	require.Equal(t, testedFluidVersion, loadedChart.Metadata.Version)

	images, err := helmutil.ChartImages(context.Background(), loadedChart, fluidValues(loadedChart), helmutil.ChartImagesOptions{ReleaseName: fluidReleaseName, Namespace: fluidNamespace})

	require.NoError(t, err)
	assert.Contains(t, images, "fluidcloudnative/dataset-controller:v1.0.8-6533239")
	assert.Contains(t, images, "juicedata/mount:ce-v1.3.0")
	assert.Contains(t, images, "alluxio/alluxio-dev:2.9.0")
}
