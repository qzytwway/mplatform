package logging

import (
	"context"
	"io"
	"testing"

	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

func TestParseInstallRequest(t *testing.T) {
	opts, err := parseInstallRequest(installRequest{StorageClass: " fast "})
	require.NoError(t, err)
	require.Equal(t, storageOptions{StorageClass: "fast", StorageSize: "20Gi", Retention: "7d"}, opts)

	for _, req := range []installRequest{
		{StorageSize: "-1Gi"},
		{Retention: "7 days"},
		{StorageClass: "Bad_Class"},
	} {
		_, err := parseInstallRequest(req)
		require.Error(t, err, req)
	}
}

// dryRunInstall renders a chart the way Helm installs it. The client dry run
// resets capabilities, so the Kubernetes version is set on the action.
func dryRunInstall(t *testing.T, loadedChart *chart.Chart, name string, values map[string]any) string {
	t.Helper()
	kubeVersion, err := common.ParseKubeVersion("v1.30.0")
	require.NoError(t, err)
	install := action.NewInstall(&action.Configuration{
		Releases:   storage.Init(driver.NewMemory()),
		KubeClient: &kubefake.PrintingKubeClient{Out: io.Discard},
	})
	install.ReleaseName = name
	install.Namespace = Namespace
	install.DryRunStrategy = action.DryRunClient
	install.KubeVersion = kubeVersion
	releaser, err := install.RunWithContext(context.Background(), loadedChart, values)
	require.NoError(t, err)
	rel, err := helmutil.ReleaseFromReleaser(releaser)
	require.NoError(t, err)
	return rel.Manifest
}

func TestVictoriaLogsChartRendersWithValues(t *testing.T) {
	loadedChart, err := loader.LoadFile("testdata/victoria-logs-single-0.13.12.tgz")
	require.NoError(t, err)

	manifest := dryRunInstall(t, loadedChart, victoriaLogsRelease, victoriaLogsValues(storageOptions{
		StorageClass: "fast",
		StorageSize:  "50Gi",
		Retention:    "14d",
	}))

	require.Contains(t, manifest, "name: "+victoriaLogsRelease+"\n")
	require.Contains(t, manifest, "retentionPeriod=14d")
	require.Contains(t, manifest, "storageClassName: fast")
	require.Contains(t, manifest, "storage: 50Gi")
	require.Contains(t, manifest, "image: victoriametrics/victoria-logs:v1.53.0")
	require.NotContains(t, manifest, "type: NodePort")
	require.NotContains(t, manifest, "httpAuth")
	require.NotContains(t, manifest, "kind: DaemonSet")
	require.NotContains(t, manifest, "kind: Secret")
}

func TestVectorChartRendersWithValues(t *testing.T) {
	loadedChart, err := loader.LoadFile("testdata/vector-0.61.0.tgz")
	require.NoError(t, err)

	manifest := dryRunInstall(t, loadedChart, vectorRelease, vectorValues(collectorOptions{ClusterName: "prod"}))

	require.Contains(t, manifest, "kind: DaemonSet")
	require.Contains(t, manifest, "http://mplatform-victoria-logs.mplatform-logging.svc:9428/insert/elasticsearch/")
	require.Contains(t, manifest, "VL-Extra-Fields: cluster=prod")
	require.Contains(t, manifest, "image: \"docker.io/timberio/vector:0.61.0-distroless-libc\"")
	require.Contains(t, manifest, "operator: Exists")
	require.Contains(t, manifest, "name: VECTOR_SELF_NODE_NAME")
	require.NotContains(t, manifest, "auth:")
	require.NotContains(t, manifest, "kind: Secret")
}
