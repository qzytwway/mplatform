package helmutil

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRewriteImageRegistry(t *testing.T) {
	const registry = "harbor.example.com/mirror"
	testcases := []struct {
		image string
		want  string
	}{
		{"nginx", registry + "/nginx"},
		{"nginx:1.25", registry + "/nginx:1.25"},
		{"bitnami/redis:7.2", registry + "/redis:7.2"},
		{"quay.io/prometheus/node-exporter", registry + "/node-exporter"},
		{"registry.k8s.io/ingress-nginx/controller:v1.10.0@" + testDigest, registry + "/controller:v1.10.0@" + testDigest},
		{"localhost:5000/team/api@" + testDigest, registry + "/api@" + testDigest},
		{"harbor.example.com/library/app:1.0", "harbor.example.com/library/app:1.0"},
		{"Invalid Image", "Invalid Image"},
	}
	for _, tc := range testcases {
		require.Equal(t, tc.want, RewriteImageRegistry(tc.image, registry), tc.image)
	}
}

func TestValidateImageRegistry(t *testing.T) {
	for _, valid := range []string{"", "harbor.example.com", "harbor.example.com:8443/mirror", "localhost:5000"} {
		require.NoError(t, ValidateImageRegistry(valid), valid)
	}
	for _, invalid := range []string{"https://harbor.example.com", "mirror", "harbor.example.com/", "harbor.example.com/mirror:tag"} {
		require.Error(t, ValidateImageRegistry(invalid), invalid)
	}
}

func TestImageRegistryPostRendererRewritesOnlyContainerImages(t *testing.T) {
	rendered := `# Source: app/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  annotations:
    postrenderer.helm.sh/postrender-filename: app/templates/deployment.yaml
spec:
  template:
    spec:
      initContainers:
        - name: init
          image: busybox:1.36
      containers:
        - name: web
          image: nginx:1.25
          args:
            - --image=nginx
          env:
            - name: DRIVER_MANAGER_IMAGE
              value: nvcr.io/nvidia/cloud-native/k8s-driver-manager:v0.12.1
            - name: DEFAULT_IMAGE
              value: nginx
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: backup
spec:
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: registry.k8s.io/backup:v1
---
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
  annotations:
    helm.sh/hook: pre-install
spec:
  template:
    spec:
      containers:
        - name: migrate
          image: harbor.example.com/library/migrate:2
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  image: nginx
`
	out, err := imageRegistryPostRenderer{imageRegistry: "harbor.example.com/mirror"}.Run(bytes.NewBufferString(rendered))
	require.NoError(t, err)

	decoder := yaml.NewDecoder(out)
	var documents []map[string]any
	for {
		var document map[string]any
		if err := decoder.Decode(&document); err != nil {
			break
		}
		documents = append(documents, document)
	}
	require.Len(t, documents, 4)

	podSpec := dig(t, documents[0], "spec", "template", "spec")
	require.Equal(t, "harbor.example.com/mirror/busybox:1.36", firstImage(t, podSpec, "initContainers"))
	require.Equal(t, "harbor.example.com/mirror/nginx:1.25", firstImage(t, podSpec, "containers"))
	require.Equal(t, "--image=nginx", podSpec["containers"].([]any)[0].(map[string]any)["args"].([]any)[0])
	env := podSpec["containers"].([]any)[0].(map[string]any)["env"].([]any)
	require.Equal(t, "harbor.example.com/mirror/k8s-driver-manager:v0.12.1", env[0].(map[string]any)["value"])
	require.Equal(t, "nginx", env[1].(map[string]any)["value"])
	require.Equal(t,
		"app/templates/deployment.yaml",
		dig(t, documents[0], "metadata", "annotations")["postrenderer.helm.sh/postrender-filename"],
	)

	cronPodSpec := dig(t, documents[1], "spec", "jobTemplate", "spec", "template", "spec")
	require.Equal(t, "harbor.example.com/mirror/backup:v1", firstImage(t, cronPodSpec, "containers"))

	hookPodSpec := dig(t, documents[2], "spec", "template", "spec")
	require.Equal(t, "harbor.example.com/library/migrate:2", firstImage(t, hookPodSpec, "containers"))

	require.Equal(t, "nginx", documents[3]["data"].(map[string]any)["image"])
}

func TestImageRegistryPostRendererRewritesClusterPolicyRepositories(t *testing.T) {
	rendered := `apiVersion: nvidia.com/v1
kind: ClusterPolicy
metadata:
  name: cluster-policy
spec:
  driver:
    enabled: true
    repository: nvcr.io/nvidia
    image: driver
    version: "580.95.05"
    manager:
      repository: nvcr.io/nvidia/cloud-native
      image: k8s-driver-manager
      version: v0.9.0
  toolkit:
    repository: harbor.example.com/library
    image: container-toolkit
    version: v1.18.0
  devicePlugin:
    repository: ""
    image: nvcr.io/nvidia/k8s-device-plugin:v0.18.0
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  repository: nvcr.io/nvidia
  image: driver
`
	out, err := imageRegistryPostRenderer{imageRegistry: "harbor.example.com/mirror"}.Run(bytes.NewBufferString(rendered))
	require.NoError(t, err)

	decoder := yaml.NewDecoder(out)
	var policy, configMap map[string]any
	require.NoError(t, decoder.Decode(&policy))
	require.NoError(t, decoder.Decode(&configMap))

	driver := dig(t, policy, "spec", "driver")
	require.Equal(t, "harbor.example.com/mirror", driver["repository"])
	require.Equal(t, "driver", driver["image"])
	require.Equal(t, "580.95.05", driver["version"])
	manager := dig(t, policy, "spec", "driver", "manager")
	require.Equal(t, "harbor.example.com/mirror", manager["repository"])
	require.Equal(t, "k8s-driver-manager", manager["image"])
	toolkit := dig(t, policy, "spec", "toolkit")
	require.Equal(t, "harbor.example.com/library", toolkit["repository"])
	require.Equal(t, "container-toolkit", toolkit["image"])
	devicePlugin := dig(t, policy, "spec", "devicePlugin")
	require.Equal(t, "nvcr.io/nvidia/k8s-device-plugin:v0.18.0", devicePlugin["image"])

	require.Equal(t, "nvcr.io/nvidia", configMap["data"].(map[string]any)["repository"])
}

func dig(t *testing.T, document map[string]any, keys ...string) map[string]any {
	t.Helper()
	node := document
	for _, key := range keys {
		next, ok := node[key].(map[string]any)
		require.True(t, ok, key)
		node = next
	}
	return node
}

func firstImage(t *testing.T, podSpec map[string]any, key string) any {
	t.Helper()
	containers, ok := podSpec[key].([]any)
	require.True(t, ok, key)
	return containers[0].(map[string]any)["image"]
}

func TestInstallReleaseRewritesImagesInManifestAndHooks(t *testing.T) {
	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: common.DefaultCapabilities,
	}
	chartToInstall := &chart.Chart{
		Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "app", Version: "0.1.0"},
		Templates: []*common.File{
			{Name: "templates/deployment.yaml", Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
          image: nginx:1.25
`)},
			{Name: "templates/migrate.yaml", Data: []byte(`apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
  annotations:
    helm.sh/hook: pre-install
spec:
  template:
    spec:
      containers:
        - name: migrate
          image: busybox:1.36
`)},
		},
	}

	rel, err := InstallRelease(context.Background(), cfg, chartToInstall, map[string]interface{}{}, InstallReleaseOptions{
		ReleaseName:   "app",
		Namespace:     "default",
		DryRun:        true,
		ImageRegistry: "harbor.example.com/mirror",
	})
	require.NoError(t, err)

	require.Contains(t, rel.Manifest, "image: harbor.example.com/mirror/nginx:1.25")
	require.NotContains(t, rel.Manifest, "postrenderer.helm.sh")
	require.Len(t, rel.Hooks, 1)
	require.Contains(t, rel.Hooks[0].Manifest, "image: harbor.example.com/mirror/busybox:1.36")
}
