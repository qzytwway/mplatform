package provision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMinor = "1.36"

	containerdVersion = "2.4.2"
	runcVersion       = "1.5.2"
	cniPluginsVersion = "1.10.0"
	calicoVersion     = "3.33.0"
	kubeVIPImage      = "ghcr.io/kube-vip/kube-vip:v1.3.1"

	versionCacheTTL     = time.Hour
	versionErrorTTL     = time.Minute
	versionFetchTimeout = 5 * time.Second
	releaseBaseURL      = "https://dl.k8s.io/release"
)

// supportedMinors is ordered newest first, which is how the versions API
// lists them.
var supportedMinors = []string{"1.37", "1.36", "1.35"}

// fallbackPatches is used when stable-<minor>.txt cannot be fetched.
var fallbackPatches = map[string]string{
	"1.37": "v1.37.2",
	"1.36": "v1.36.6",
	"1.35": "v1.35.10",
}

// pauseVersions matches kubeadm's constants.PauseVersion for each minor.
var pauseVersions = map[string]string{
	"1.37": "3.10.2",
	"1.36": "3.10.2",
	"1.35": "3.10.1",
}

// crictlVersions follows cri-tools, which releases one version per minor.
var crictlVersions = map[string]string{
	"1.37": "v1.37.0",
	"1.36": "v1.36.0",
	"1.35": "v1.35.0",
}

var patchPattern = regexp.MustCompile(`^v1\.\d+\.\d+$`)

func isSupportedMinor(minor string) bool {
	_, ok := fallbackPatches[minor]
	return ok
}

type cachedPatch struct {
	patch     string
	err       error
	fetchedAt time.Time
}

type versionResolver struct {
	mu      sync.Mutex
	cache   map[string]cachedPatch
	client  *http.Client
	baseURL string
}

var versions = &versionResolver{
	cache:   map[string]cachedPatch{},
	client:  &http.Client{Timeout: versionFetchTimeout},
	baseURL: releaseBaseURL,
}

// latestPatch returns the newest patch release of a minor, cached for an hour.
// Failures are cached briefly so an offline platform does not stall every
// request.
func (r *versionResolver) latestPatch(ctx context.Context, minor string) (string, error) {
	r.mu.Lock()
	cached, ok := r.cache[minor]
	r.mu.Unlock()
	ttl := versionCacheTTL
	if cached.err != nil {
		ttl = versionErrorTTL
	}
	if ok && time.Since(cached.fetchedAt) < ttl {
		return cached.patch, cached.err
	}
	patch, err := r.fetch(ctx, minor)
	r.mu.Lock()
	r.cache[minor] = cachedPatch{patch: patch, err: err, fetchedAt: time.Now()}
	r.mu.Unlock()
	return patch, err
}

func (r *versionResolver) fetch(ctx context.Context, minor string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/stable-%s.txt", r.baseURL, minor), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to resolve Kubernetes %s: %w", minor, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to resolve Kubernetes %s: HTTP %d", minor, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", fmt.Errorf("failed to resolve Kubernetes %s: %w", minor, err)
	}
	patch := strings.TrimSpace(string(body))
	if !patchPattern.MatchString(patch) || !strings.HasPrefix(patch, "v"+minor+".") {
		return "", fmt.Errorf("failed to resolve Kubernetes %s: unexpected version %q", minor, patch)
	}
	return patch, nil
}

// resolveForInstall returns the patch to install, falling back to the
// built-in version when resolution fails.
func (r *versionResolver) resolveForInstall(ctx context.Context, minor string) string {
	if patch, err := r.latestPatch(ctx, minor); err == nil {
		return patch
	}
	return fallbackPatches[minor]
}

// downloadURL maps an official download URL onto the binary mirror. The
// mirror keeps the official host and path under its root:
//
//	<binaryMirror>/<original-host>/<original-path>
//
// for example
//
//	https://mirror.example.com/dl.k8s.io/release/v1.36.6/bin/linux/amd64/kubeadm
//	https://mirror.example.com/github.com/containerd/containerd/releases/download/v2.4.2/containerd-2.4.2-linux-amd64.tar.gz
//	https://mirror.example.com/raw.githubusercontent.com/projectcalico/calico/v3.33.0/manifests/tigera-operator.yaml
//
// Checksum files live next to the binaries under the same layout.
func downloadURL(mirror, original string) string {
	if mirror == "" {
		return original
	}
	u, err := url.Parse(original)
	if err != nil {
		return original
	}
	return strings.TrimRight(mirror, "/") + "/" + u.Host + u.EscapedPath()
}

// download is one file fetched on a node and verified with sha256. SumName
// is the entry to look up in a multi-file checksum list.
type download struct {
	URL     string
	SumURL  string
	SumName string
	Dest    string
}

// Positions in the list returned by nodeDownloads.
const (
	downloadKubeadm = iota
	downloadKubelet
	downloadKubectl
	downloadContainerd
	downloadRunc
	downloadCNI
	downloadCrictl
)

// nodeDownloads lists every binary prepare installs for one architecture.
func nodeDownloads(kubeVersion, minor, arch, mirror string) []download {
	k8s := func(name string) download {
		u := fmt.Sprintf("%s/%s/bin/linux/%s/%s", releaseBaseURL, kubeVersion, arch, name)
		return download{URL: downloadURL(mirror, u), SumURL: downloadURL(mirror, u+".sha256"), SumName: name, Dest: name}
	}
	containerd := fmt.Sprintf("containerd-%s-linux-%s.tar.gz", containerdVersion, arch)
	containerdURL := fmt.Sprintf("https://github.com/containerd/containerd/releases/download/v%s/%s", containerdVersion, containerd)
	runcURL := fmt.Sprintf("https://github.com/opencontainers/runc/releases/download/v%s/runc.%s", runcVersion, arch)
	runcSumURL := fmt.Sprintf("https://github.com/opencontainers/runc/releases/download/v%s/runc.sha256sum", runcVersion)
	cni := fmt.Sprintf("cni-plugins-linux-%s-v%s.tgz", arch, cniPluginsVersion)
	cniURL := fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/v%s/%s", cniPluginsVersion, cni)
	crictl := fmt.Sprintf("crictl-%s-linux-%s.tar.gz", crictlVersions[minor], arch)
	crictlURL := fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/%s", crictlVersions[minor], crictl)
	return []download{
		k8s("kubeadm"),
		k8s("kubelet"),
		k8s("kubectl"),
		{URL: downloadURL(mirror, containerdURL), SumURL: downloadURL(mirror, containerdURL+".sha256sum"), SumName: containerd, Dest: containerd},
		{URL: downloadURL(mirror, runcURL), SumURL: downloadURL(mirror, runcSumURL), SumName: "runc." + arch, Dest: "runc"},
		{URL: downloadURL(mirror, cniURL), SumURL: downloadURL(mirror, cniURL+".sha256"), SumName: cni, Dest: cni},
		{URL: downloadURL(mirror, crictlURL), SumURL: downloadURL(mirror, crictlURL+".sha256"), SumName: crictl, Dest: crictl},
	}
}

// calicoManifestURLs returns the CRD bundle and the operator manifest. The
// CRDs are split out since Calico 3.30 and must be applied first.
func calicoManifestURLs(mirror string) (crds, operator string) {
	base := fmt.Sprintf("https://raw.githubusercontent.com/projectcalico/calico/v%s/manifests", calicoVersion)
	return downloadURL(mirror, base+"/operator-crds.yaml"), downloadURL(mirror, base+"/tigera-operator.yaml")
}

func pauseImage(imageRepository, minor string) string {
	return fmt.Sprintf("%s/pause:%s", imageRepository, pauseVersions[minor])
}

type versionInfo struct {
	Minor string `json:"minor"`
	Patch string `json:"patch"`
	Error string `json:"error,omitempty"`
}

func listVersions(ctx context.Context) []versionInfo {
	result := make([]versionInfo, len(supportedMinors))
	var wg sync.WaitGroup
	for i, minor := range supportedMinors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			patch, err := versions.latestPatch(ctx, minor)
			info := versionInfo{Minor: minor, Patch: patch}
			if err != nil {
				info.Patch = ""
				info.Error = fmt.Sprintf("%v; installations will use %s", err, fallbackPatches[minor])
			}
			result[i] = info
		}()
	}
	wg.Wait()
	return result
}
