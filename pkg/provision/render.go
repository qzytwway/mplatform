package provision

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"path"
	"strings"
	"text/template"
)

//go:embed assets/*
var assetsFS embed.FS

// Paths on the nodes. Everything MPlatform writes for kubeadm lives under
// configDir, which kubeadm reset leaves alone.
const (
	configDir          = "/etc/kubernetes/mplatform"
	scriptDir          = "/var/lib/mplatform/scripts"
	auditPolicyPath    = configDir + "/audit-policy.yaml"
	encryptionPath     = configDir + "/encryption-config.yaml"
	initConfigPath     = configDir + "/kubeadm-init.yaml"
	joinConfigPath     = configDir + "/kubeadm-join.yaml"
	kubeletPatchPath   = configDir + "/patches/kubeletconfiguration+merge.yaml"
	kubeVIPPath        = configDir + "/kube-vip.yaml"
	kubeVIPInitPath    = configDir + "/kube-vip-init.yaml"
	calicoInstallPath  = configDir + "/calico/installation.yaml"
	adminConfPath      = "/etc/kubernetes/admin.conf"
	superAdminConfPath = "/etc/kubernetes/super-admin.conf"
	caCertPath         = "/etc/kubernetes/pki/ca.crt"
	// ownerMarkerPath records which provision prepared the node; deleting a
	// provision only resets nodes that carry its marker.
	ownerMarkerPath = "/var/lib/mplatform/owner"
)

// tlsCipherSuites keeps only ECDHE with AES-GCM or ChaCha20 for TLS 1.2.
var tlsCipherSuites = []string{
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
}

var (
	controlPlanePorts = []string{"6443/tcp", "2379-2380/tcp", "10250/tcp", "10256/tcp", "10257/tcp", "10259/tcp", "30000-32767/tcp"}
	workerPorts       = []string{"10250/tcp", "10256/tcp", "30000-32767/tcp"}
	// Calico VXLAN, Typha and BGP, which Calico uses for same-subnet routes
	// with VXLANCrossSubnet.
	calicoPorts = []string{"4789/udp", "5473/tcp", "179/tcp"}
)

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"json": func(v string) (string, error) {
		out, err := json.Marshal(v)
		return string(out), err
	},
	"shq":       shellQuote,
	"joinComma": func(v []string) string { return strings.Join(v, ",") },
}).ParseFS(assetsFS, "assets/*.tmpl"))

// clusterParams is what every node of a cluster renders from.
type clusterParams struct {
	Name              string
	KubernetesVersion string
	Minor             string
	Mode              string
	EndpointHost      string
	EndpointPort      int
	ImageRepository   string
	PodCIDR           string
	ServiceCIDR       string
	DNSDomain         string
	CertSANs          []string
	Schedulable       bool
	EncryptionKey     string
	BinaryMirror      string
	RegistryMirrors   []RegistryMirror
}

func (c clusterParams) EndpointAddress() string {
	return net.JoinHostPort(c.EndpointHost, fmt.Sprint(c.EndpointPort))
}

// nodeParams is what one node renders from; Arch, OSFamily, Interface and the
// sizes come from precheck.
type nodeParams struct {
	Name      string
	IP        string
	Role      string
	Arch      string
	OSFamily  string
	Interface string
	CPU       int
	MemoryMiB int
}

// newClusterParams builds render input from a spec. cpHostnames are the
// node names of the control planes, added to the API server certificate.
func newClusterParams(spec Spec, kubeVersion, encryptionKey string, cpHostnames []string) clusterParams {
	sans := []string{spec.EndpointHost()}
	for _, n := range spec.ControlPlanes() {
		sans = append(sans, n.IP)
	}
	sans = append(sans, cpHostnames...)
	sans = append(sans, "127.0.0.1", "localhost")
	return clusterParams{
		Name:              spec.Name,
		KubernetesVersion: kubeVersion,
		Minor:             spec.KubernetesMinor,
		Mode:              spec.ControlPlane.Mode,
		EndpointHost:      spec.EndpointHost(),
		EndpointPort:      spec.ControlPlane.EndpointPort,
		ImageRepository:   spec.ImageRepository(),
		PodCIDR:           spec.Network.PodCIDR,
		ServiceCIDR:       spec.Network.ServiceCIDR,
		DNSDomain:         spec.Network.DNSDomain,
		CertSANs:          uniqueNonEmpty(sans),
		Schedulable:       spec.SchedulableControlPlane,
		EncryptionKey:     encryptionKey,
		BinaryMirror:      spec.Sources.BinaryMirror,
		RegistryMirrors:   spec.Sources.RegistryMirrors,
	}
}

type kubeadmData struct {
	Cluster         clusterParams
	Node            nodeParams
	Token           string
	CertificateKey  string
	CACertHash      string
	TLSCipherSuites []string
}

func renderTemplate(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("failed to render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func readAsset(name string) []byte {
	data, err := assetsFS.ReadFile("assets/" + name)
	if err != nil {
		panic(fmt.Sprintf("missing embedded asset %s: %v", name, err))
	}
	return data
}

// renderInitConfig renders the multi-document kubeadm config of the first
// control plane. certKey is empty for a single control plane.
func renderInitConfig(c clusterParams, n nodeParams, token, certKey string) ([]byte, error) {
	return renderTemplate("kubeadm-init.yaml.tmpl", kubeadmData{
		Cluster: c, Node: n, Token: token, CertificateKey: certKey, TLSCipherSuites: tlsCipherSuites,
	})
}

func renderJoinConfig(c clusterParams, n nodeParams, token, caCertHash, certKey string) ([]byte, error) {
	return renderTemplate("kubeadm-join.yaml.tmpl", kubeadmData{
		Cluster: c, Node: n, Token: token, CACertHash: caCertHash, CertificateKey: certKey,
	})
}

type reservation struct {
	KubeCPU      int
	KubeMemory   int
	SystemCPU    int
	SystemMemory int
}

// nodeReservation follows GKE's allocatable tiers for the total reservation
// and splits it evenly between kubeReserved (kubelet, containerd) and
// systemReserved (OS daemons).
func nodeReservation(cpus, memoryMiB int) reservation {
	cpu := reservedCPUMillis(cpus)
	memory := reservedMemoryMiB(memoryMiB)
	return reservation{
		KubeCPU:      cpu - cpu/2,
		KubeMemory:   memory - memory/2,
		SystemCPU:    cpu / 2,
		SystemMemory: memory / 2,
	}
}

// reservedCPUMillis: 6% of the first core, 1% of the second, 0.5% of cores
// three and four, 0.25% of every core above four.
func reservedCPUMillis(cpus int) int {
	tiers := []struct{ cores, permille int }{{1, 60}, {1, 10}, {2, 5}}
	millis := 0.0
	remaining := cpus
	for _, tier := range tiers {
		take := min(remaining, tier.cores)
		millis += float64(take * tier.permille)
		remaining -= take
	}
	millis += float64(remaining) * 2.5
	return int(millis + 0.5)
}

// reservedMemoryMiB: 255MiB below 1GiB; otherwise 25% of the first 4GiB, 20%
// of the next 4GiB, 10% of the next 8GiB, 6% of the next 112GiB and 2% above.
func reservedMemoryMiB(memoryMiB int) int {
	if memoryMiB < 1024 {
		return 255
	}
	tiers := []struct{ mib, percent int }{{4096, 25}, {4096, 20}, {8192, 10}, {114688, 6}}
	reserved := 0
	remaining := memoryMiB
	for _, tier := range tiers {
		take := min(remaining, tier.mib)
		reserved += take * tier.percent / 100
		remaining -= take
	}
	return reserved + remaining*2/100
}

func renderKubeletPatch(n nodeParams) ([]byte, error) {
	return renderTemplate("kubelet-patch.yaml.tmpl", nodeReservation(n.CPU, n.MemoryMiB))
}

func renderEncryptionConfig(key string) ([]byte, error) {
	return renderTemplate("encryption-config.yaml.tmpl", map[string]string{"Key": key})
}

func renderContainerdConfig(c clusterParams) ([]byte, error) {
	return renderTemplate("containerd-config.toml.tmpl", map[string]string{"PauseImage": pauseImage(c.ImageRepository, c.Minor)})
}

// renderRegistryHosts returns one certs.d hosts.toml per mirrored registry,
// keyed by its path on the node.
func renderRegistryHosts(mirrors []RegistryMirror) (map[string][]byte, error) {
	endpoints := map[string][]string{}
	var order []string
	for _, m := range mirrors {
		if _, ok := endpoints[m.Registry]; !ok {
			order = append(order, m.Registry)
		}
		endpoints[m.Registry] = append(endpoints[m.Registry], m.Endpoint)
	}
	files := map[string][]byte{}
	for _, registry := range order {
		server := "https://" + registry
		if registry == "docker.io" {
			server = "https://registry-1.docker.io"
		}
		out, err := renderTemplate("hosts.toml.tmpl", map[string]any{"Server": server, "Endpoints": endpoints[registry]})
		if err != nil {
			return nil, err
		}
		files[path.Join("/etc/containerd/certs.d", registry, "hosts.toml")] = out
	}
	return files, nil
}

// renderKubeVIP renders the kube-vip static pod using the given kubeconfig
// on the host: super-admin.conf while the first control plane initializes,
// admin.conf afterwards.
func renderKubeVIP(c clusterParams, n nodeParams, kubeconfig string) ([]byte, error) {
	return renderTemplate("kube-vip.yaml.tmpl", map[string]string{
		"Image": kubeVIPImage, "Interface": n.Interface, "Address": c.EndpointHost, "Kubeconfig": kubeconfig,
	})
}

func renderCalicoInstallation(c clusterParams) ([]byte, error) {
	return renderTemplate("calico-installation.yaml.tmpl", map[string]string{"PodCIDR": c.PodCIDR})
}

type probe struct {
	Kind string // file: must download; registry: any HTTP answer
	URL  string // may contain @ARCH@
}

// precheckProbes lists the URLs every node must reach.
func precheckProbes(c clusterParams, controlPlane bool) []probe {
	const archPlaceholder = "@ARCH@"
	downloads := nodeDownloads(c.KubernetesVersion, c.Minor, archPlaceholder, c.BinaryMirror)
	probes := []probe{
		{Kind: "file", URL: downloads[downloadKubeadm].SumURL},
		{Kind: "file", URL: downloads[downloadContainerd].SumURL},
	}
	if controlPlane {
		crds, _ := calicoManifestURLs(c.BinaryMirror)
		probes = append(probes, probe{Kind: "file", URL: crds})
	}
	registryHost := strings.SplitN(c.ImageRepository, "/", 2)[0]
	return append(probes, probe{Kind: "registry", URL: "https://" + registryHost + "/v2/"})
}

func renderPrecheckScript(c clusterParams, controlPlane bool) ([]byte, error) {
	vip := ""
	if c.Mode == ModeKubeVIP && controlPlane {
		vip = c.EndpointHost
	}
	return renderTemplate("precheck.sh.tmpl", map[string]any{"Probes": precheckProbes(c, controlPlane), "VIP": vip})
}

func firewallPorts(role string) []string {
	ports := workerPorts
	if role == RoleControlPlane {
		ports = controlPlanePorts
	}
	return append(append([]string{}, ports...), calicoPorts...)
}

func renderPrepareScript(c clusterParams, n nodeParams) ([]byte, error) {
	downloads := nodeDownloads(c.KubernetesVersion, c.Minor, n.Arch, c.BinaryMirror)
	return renderTemplate("prepare.sh.tmpl", map[string]any{
		"OSFamily":          n.OSFamily,
		"PodCIDR":           c.PodCIDR,
		"ServiceCIDR":       c.ServiceCIDR,
		"Ports":             firewallPorts(n.Role),
		"Downloads":         downloads,
		"ContainerdArchive": downloads[downloadContainerd].Dest,
		"CNIArchive":        downloads[downloadCNI].Dest,
		"CrictlArchive":     downloads[downloadCrictl].Dest,
	})
}

func renderInitScript(c clusterParams) ([]byte, error) {
	return renderTemplate("init.sh.tmpl", map[string]bool{"KubeVIP": c.Mode == ModeKubeVIP, "UploadCerts": c.Mode != ModeSingle})
}

func renderJoinScript(c clusterParams, n nodeParams) ([]byte, error) {
	return renderTemplate("join.sh.tmpl", map[string]bool{"KubeVIP": c.Mode == ModeKubeVIP && n.Role == RoleControlPlane})
}

func renderCNIScript(c clusterParams) ([]byte, error) {
	crds, operator := calicoManifestURLs(c.BinaryMirror)
	return renderTemplate("cni.sh.tmpl", map[string]string{"CRDsURL": crds, "OperatorURL": operator})
}

// nodeFile is one file uploaded to a node.
type nodeFile struct {
	Path string
	Data []byte
	Mode uint32
}

// prepareFiles are the runtime and OS files every node needs before kubeadm.
func prepareFiles(c clusterParams, n nodeParams) ([]nodeFile, error) {
	containerd, err := renderContainerdConfig(c)
	if err != nil {
		return nil, err
	}
	hosts, err := renderRegistryHosts(c.RegistryMirrors)
	if err != nil {
		return nil, err
	}
	script, err := renderPrepareScript(c, n)
	if err != nil {
		return nil, err
	}
	files := []nodeFile{
		{Path: "/etc/modules-load.d/mplatform-kubernetes.conf", Data: readAsset("modules-load.conf"), Mode: 0o644},
		{Path: "/etc/sysctl.d/99-mplatform-kubernetes.conf", Data: readAsset("sysctl.conf"), Mode: 0o644},
		{Path: "/etc/containerd/config.toml", Data: containerd, Mode: 0o644},
		{Path: "/etc/crictl.yaml", Data: readAsset("crictl.yaml"), Mode: 0o644},
		{Path: "/etc/systemd/system/containerd.service", Data: readAsset("containerd.service"), Mode: 0o644},
		{Path: "/etc/systemd/system/kubelet.service", Data: readAsset("kubelet.service"), Mode: 0o644},
		{Path: "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf", Data: readAsset("10-kubeadm.conf"), Mode: 0o644},
		{Path: scriptDir + "/reset.sh", Data: readAsset("reset.sh"), Mode: 0o700},
		{Path: scriptDir + "/prepare.sh", Data: script, Mode: 0o700},
	}
	for p, data := range hosts {
		files = append(files, nodeFile{Path: p, Data: data, Mode: 0o644})
	}
	return files, nil
}

// controlPlaneFiles are needed on every control plane before init or join:
// the API server mounts the audit policy and the encryption config.
func controlPlaneFiles(c clusterParams, n nodeParams) ([]nodeFile, error) {
	encryption, err := renderEncryptionConfig(c.EncryptionKey)
	if err != nil {
		return nil, err
	}
	files := []nodeFile{
		{Path: auditPolicyPath, Data: readAsset("audit-policy.yaml"), Mode: 0o600},
		{Path: encryptionPath, Data: encryption, Mode: 0o600},
	}
	if c.Mode != ModeKubeVIP {
		return files, nil
	}
	vip, err := renderKubeVIP(c, n, adminConfPath)
	if err != nil {
		return nil, err
	}
	return append(files, nodeFile{Path: kubeVIPPath, Data: vip, Mode: 0o600}), nil
}

// ownerMarker is the content of ownerMarkerPath for a provision.
func ownerMarker(provisionID uint, name string) []byte {
	return []byte(fmt.Sprintf("provision=%d\ncluster=%s\n", provisionID, name))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
