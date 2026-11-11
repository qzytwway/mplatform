package provision

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	ModeSingle     = "single"
	ModeKubeVIP    = "kube-vip"
	ModeExternalLB = "external-lb"

	RoleControlPlane = "control-plane"
	RoleWorker       = "worker"

	AuthKey      = "key"
	AuthPassword = "password"

	apiServerPort         = 6443
	defaultPodCIDR        = "10.244.0.0/16"
	defaultServiceCIDR    = "10.96.0.0/12"
	defaultDNSDomain      = "cluster.local"
	defaultImageRegistry  = "registry.k8s.io"
	haControlPlaneCount   = 3
	maxServiceCIDRBits    = 20
	maxPodCIDRPrefix      = 22
	maxDescriptionLength  = 1000
	maxRegistryMirrorRows = 20
)

var (
	clusterNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	dnsLabelPattern    = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	imageRepoPattern   = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]{1,5})?(/[a-z0-9._-]+)*$`)
)

// Spec is the request body of create and precheck. Secrets are omitted from
// JSON when empty, so a sanitized copy can be returned as-is.
type Spec struct {
	Name                    string           `json:"name"`
	Description             string           `json:"description"`
	KubernetesMinor         string           `json:"kubernetesMinor"`
	ControlPlane            ControlPlaneSpec `json:"controlPlane"`
	Network                 NetworkSpec      `json:"network"`
	Sources                 SourcesSpec      `json:"sources"`
	SSH                     SSHSpec          `json:"ssh"`
	Nodes                   []NodeSpec       `json:"nodes"`
	SchedulableControlPlane bool             `json:"schedulableControlPlane"`
	IsDefault               bool             `json:"isDefault"`
}

type ControlPlaneSpec struct {
	Mode         string `json:"mode"`
	Endpoint     string `json:"endpoint"`
	EndpointPort int    `json:"endpointPort"`
}

type NetworkSpec struct {
	PodCIDR     string `json:"podCIDR"`
	ServiceCIDR string `json:"serviceCIDR"`
	DNSDomain   string `json:"dnsDomain"`
}

type SourcesSpec struct {
	BinaryMirror    string           `json:"binaryMirror"`
	ImageRepository string           `json:"imageRepository"`
	RegistryMirrors []RegistryMirror `json:"registryMirrors"`
}

type RegistryMirror struct {
	Registry string `json:"registry"`
	Endpoint string `json:"endpoint"`
}

type SSHSpec struct {
	User       string `json:"user"`
	Port       int    `json:"port"`
	AuthType   string `json:"authType"`
	PrivateKey string `json:"privateKey,omitempty"`
	Password   string `json:"password,omitempty"`
}

type NodeSpec struct {
	IP       string   `json:"ip"`
	Hostname string   `json:"hostname"`
	Role     string   `json:"role"`
	SSH      *SSHSpec `json:"ssh"`
}

// Normalize trims input and fills documented defaults in place.
func (s *Spec) Normalize() {
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	s.KubernetesMinor = strings.TrimSpace(s.KubernetesMinor)
	if s.KubernetesMinor == "" {
		s.KubernetesMinor = DefaultMinor
	}
	s.ControlPlane.Mode = strings.TrimSpace(s.ControlPlane.Mode)
	s.ControlPlane.Endpoint = strings.TrimSpace(s.ControlPlane.Endpoint)
	if s.ControlPlane.Mode == ModeSingle {
		s.ControlPlane.Endpoint = ""
	}
	if s.ControlPlane.EndpointPort == 0 {
		s.ControlPlane.EndpointPort = apiServerPort
	}
	s.Network.PodCIDR = defaultString(strings.TrimSpace(s.Network.PodCIDR), defaultPodCIDR)
	s.Network.ServiceCIDR = defaultString(strings.TrimSpace(s.Network.ServiceCIDR), defaultServiceCIDR)
	s.Network.DNSDomain = defaultString(strings.ToLower(strings.TrimSpace(s.Network.DNSDomain)), defaultDNSDomain)
	s.Sources.BinaryMirror = strings.TrimRight(strings.TrimSpace(s.Sources.BinaryMirror), "/")
	s.Sources.ImageRepository = strings.TrimRight(strings.TrimSpace(s.Sources.ImageRepository), "/")
	mirrors := make([]RegistryMirror, 0, len(s.Sources.RegistryMirrors))
	for _, m := range s.Sources.RegistryMirrors {
		mirrors = append(mirrors, RegistryMirror{
			Registry: strings.ToLower(strings.TrimSpace(m.Registry)),
			Endpoint: strings.TrimRight(strings.TrimSpace(m.Endpoint), "/"),
		})
	}
	s.Sources.RegistryMirrors = mirrors
	s.SSH = normalizeSSH(s.SSH)
	for i := range s.Nodes {
		n := &s.Nodes[i]
		n.IP = strings.TrimSpace(n.IP)
		n.Hostname = strings.ToLower(strings.TrimSpace(n.Hostname))
		n.Role = strings.TrimSpace(n.Role)
		if n.SSH != nil {
			override := normalizeSSH(*n.SSH)
			n.SSH = &override
		}
	}
}

func normalizeSSH(s SSHSpec) SSHSpec {
	s.User = strings.TrimSpace(s.User)
	s.AuthType = strings.TrimSpace(s.AuthType)
	if s.Port == 0 {
		s.Port = 22
	}
	if s.AuthType == AuthKey {
		s.PrivateKey = strings.TrimSpace(s.PrivateKey) + "\n"
		s.Password = ""
	}
	if s.AuthType == AuthPassword {
		s.PrivateKey = ""
	}
	return s
}

// Validate checks a normalized spec. Error messages are shown to the user.
func (s Spec) Validate() error {
	if !clusterNamePattern.MatchString(s.Name) {
		return fmt.Errorf("name must consist of lowercase letters, digits and '-', start and end with a letter or digit, and be at most 63 characters")
	}
	if len(s.Description) > maxDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLength)
	}
	if !isSupportedMinor(s.KubernetesMinor) {
		return fmt.Errorf("kubernetesMinor %q is not supported; choose one of %s", s.KubernetesMinor, strings.Join(supportedMinors, ", "))
	}
	if err := s.validateNodes(); err != nil {
		return err
	}
	if err := s.validateControlPlane(); err != nil {
		return err
	}
	if len(s.Workers()) == 0 && !s.SchedulableControlPlane {
		return fmt.Errorf("a cluster without worker nodes requires schedulableControlPlane to be true")
	}
	if err := s.validateNetwork(); err != nil {
		return err
	}
	if err := s.validateSources(); err != nil {
		return err
	}
	if err := validateSSH("ssh", s.SSH, s.hasNodeWithoutSSHOverride()); err != nil {
		return err
	}
	for i, n := range s.Nodes {
		if n.SSH != nil {
			if err := validateSSH(fmt.Sprintf("nodes[%d].ssh", i), *n.SSH, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s Spec) validateNodes() error {
	if len(s.Nodes) == 0 {
		return fmt.Errorf("at least one node is required")
	}
	ips := map[string]bool{}
	hostnames := map[string]bool{}
	for i, n := range s.Nodes {
		ip := net.ParseIP(n.IP)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("nodes[%d].ip %q is not a valid IPv4 address", i, n.IP)
		}
		if ips[n.IP] {
			return fmt.Errorf("node IP %s is listed more than once", n.IP)
		}
		ips[n.IP] = true
		if n.Role != RoleControlPlane && n.Role != RoleWorker {
			return fmt.Errorf("nodes[%d].role must be %q or %q", i, RoleControlPlane, RoleWorker)
		}
		if n.Hostname == "" {
			continue
		}
		if !isDNSSubdomain(n.Hostname) {
			return fmt.Errorf("nodes[%d].hostname %q is not a valid node name (lowercase letters, digits, '-' and '.')", i, n.Hostname)
		}
		if hostnames[n.Hostname] {
			return fmt.Errorf("hostname %s is used by more than one node", n.Hostname)
		}
		hostnames[n.Hostname] = true
	}
	return nil
}

func (s Spec) validateControlPlane() error {
	cps := len(s.ControlPlanes())
	cp := s.ControlPlane
	if cp.EndpointPort < 1 || cp.EndpointPort > 65535 {
		return fmt.Errorf("controlPlane.endpointPort must be between 1 and 65535")
	}
	switch cp.Mode {
	case ModeSingle:
		if cps != 1 {
			return fmt.Errorf("single control plane mode requires exactly 1 control-plane node, got %d", cps)
		}
		if cp.EndpointPort != apiServerPort {
			return fmt.Errorf("single control plane mode serves the API on port %d", apiServerPort)
		}
	case ModeKubeVIP:
		if cps != haControlPlaneCount {
			return fmt.Errorf("kube-vip mode requires exactly %d control-plane nodes, got %d", haControlPlaneCount, cps)
		}
		ip := net.ParseIP(cp.Endpoint)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("controlPlane.endpoint must be the IPv4 virtual IP for kube-vip")
		}
		for _, n := range s.Nodes {
			if n.IP == cp.Endpoint {
				return fmt.Errorf("the virtual IP %s must not be a node IP", cp.Endpoint)
			}
		}
		if cp.EndpointPort != apiServerPort {
			return fmt.Errorf("kube-vip mode serves the API on port %d", apiServerPort)
		}
	case ModeExternalLB:
		if cps != haControlPlaneCount {
			return fmt.Errorf("external load balancer mode requires exactly %d control-plane nodes, got %d", haControlPlaneCount, cps)
		}
		if cp.Endpoint == "" {
			return fmt.Errorf("controlPlane.endpoint is required: the load balancer IP or DNS name")
		}
		if ip := net.ParseIP(cp.Endpoint); ip == nil && !isDNSSubdomain(strings.ToLower(cp.Endpoint)) {
			return fmt.Errorf("controlPlane.endpoint %q is not a valid IP address or DNS name", cp.Endpoint)
		}
	default:
		return fmt.Errorf("controlPlane.mode must be one of %s, %s, %s", ModeSingle, ModeKubeVIP, ModeExternalLB)
	}
	return nil
}

func (s Spec) validateNetwork() error {
	_, pod, err := net.ParseCIDR(s.Network.PodCIDR)
	if err != nil || pod.IP.To4() == nil {
		return fmt.Errorf("network.podCIDR %q is not a valid IPv4 CIDR", s.Network.PodCIDR)
	}
	_, svc, err := net.ParseCIDR(s.Network.ServiceCIDR)
	if err != nil || svc.IP.To4() == nil {
		return fmt.Errorf("network.serviceCIDR %q is not a valid IPv4 CIDR", s.Network.ServiceCIDR)
	}
	if ones, bits := pod.Mask.Size(); ones > maxPodCIDRPrefix || bits != 32 {
		return fmt.Errorf("network.podCIDR must be /%d or larger so every node gets a /24 block", maxPodCIDRPrefix)
	}
	if ones, bits := svc.Mask.Size(); bits-ones > maxServiceCIDRBits {
		return fmt.Errorf("network.serviceCIDR must not be larger than /%d", bits-maxServiceCIDRBits)
	}
	if cidrsOverlap(pod, svc) {
		return fmt.Errorf("network.podCIDR %s overlaps network.serviceCIDR %s", pod, svc)
	}
	for _, n := range s.Nodes {
		ip := net.ParseIP(n.IP)
		if pod.Contains(ip) || svc.Contains(ip) {
			return fmt.Errorf("node IP %s is inside the pod or service CIDR", n.IP)
		}
	}
	if !isDNSSubdomain(s.Network.DNSDomain) {
		return fmt.Errorf("network.dnsDomain %q is not a valid DNS domain", s.Network.DNSDomain)
	}
	return nil
}

func (s Spec) validateSources() error {
	if s.Sources.BinaryMirror != "" {
		if err := validateHTTPURL(s.Sources.BinaryMirror); err != nil {
			return fmt.Errorf("sources.binaryMirror: %w", err)
		}
	}
	if s.Sources.ImageRepository != "" && !imageRepoPattern.MatchString(s.Sources.ImageRepository) {
		return fmt.Errorf("sources.imageRepository %q must look like registry.example.com/path without a scheme", s.Sources.ImageRepository)
	}
	if len(s.Sources.RegistryMirrors) > maxRegistryMirrorRows {
		return fmt.Errorf("at most %d registry mirrors are supported", maxRegistryMirrorRows)
	}
	for i, m := range s.Sources.RegistryMirrors {
		if !isRegistryHost(m.Registry) {
			return fmt.Errorf("sources.registryMirrors[%d].registry %q must be a registry host such as docker.io", i, m.Registry)
		}
		if err := validateHTTPURL(m.Endpoint); err != nil {
			return fmt.Errorf("sources.registryMirrors[%d].endpoint: %w", i, err)
		}
	}
	return nil
}

func validateSSH(field string, s SSHSpec, required bool) error {
	if !required {
		return nil
	}
	if s.User == "" {
		return fmt.Errorf("%s.user is required", field)
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("%s.port must be between 1 and 65535", field)
	}
	switch s.AuthType {
	case AuthKey:
		if strings.TrimSpace(s.PrivateKey) == "" {
			return fmt.Errorf("%s.privateKey is required for key authentication", field)
		}
		if _, err := ssh.ParsePrivateKey([]byte(s.PrivateKey)); err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return fmt.Errorf("%s.privateKey is protected by a passphrase, which is not supported", field)
			}
			return fmt.Errorf("%s.privateKey is not a valid private key", field)
		}
	case AuthPassword:
		if s.Password == "" {
			return fmt.Errorf("%s.password is required for password authentication", field)
		}
	default:
		return fmt.Errorf("%s.authType must be %q or %q", field, AuthKey, AuthPassword)
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q must be an http(s) URL", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%q must not contain credentials, a query or a fragment", raw)
	}
	return nil
}

func (s Spec) hasNodeWithoutSSHOverride() bool {
	for _, n := range s.Nodes {
		if n.SSH == nil {
			return true
		}
	}
	return false
}

// ControlPlanes returns control-plane nodes in spec order; the first one is
// initialized with kubeadm init.
func (s Spec) ControlPlanes() []NodeSpec {
	return s.nodesWithRole(RoleControlPlane)
}

func (s Spec) Workers() []NodeSpec {
	return s.nodesWithRole(RoleWorker)
}

func (s Spec) nodesWithRole(role string) []NodeSpec {
	var nodes []NodeSpec
	for _, n := range s.Nodes {
		if n.Role == role {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// EndpointHost is the host of controlPlaneEndpoint: the VIP, the load
// balancer, or the only control plane's IP.
func (s Spec) EndpointHost() string {
	if s.ControlPlane.Mode == ModeSingle {
		if cps := s.ControlPlanes(); len(cps) > 0 {
			return cps[0].IP
		}
	}
	return s.ControlPlane.Endpoint
}

func (s Spec) EndpointAddress() string {
	return net.JoinHostPort(s.EndpointHost(), fmt.Sprint(s.ControlPlane.EndpointPort))
}

// ImageRepository is the effective registry for Kubernetes images.
func (s Spec) ImageRepository() string {
	return defaultString(s.Sources.ImageRepository, defaultImageRegistry)
}

// Sanitized returns a copy without any private key or password.
func (s Spec) Sanitized() Spec {
	out := s
	out.SSH.PrivateKey = ""
	out.SSH.Password = ""
	out.Nodes = make([]NodeSpec, len(s.Nodes))
	for i, n := range s.Nodes {
		out.Nodes[i] = n
		if n.SSH != nil {
			override := *n.SSH
			override.PrivateKey = ""
			override.Password = ""
			out.Nodes[i].SSH = &override
		}
	}
	out.Sources.RegistryMirrors = append([]RegistryMirror{}, s.Sources.RegistryMirrors...)
	return out
}

// EffectiveSSH returns the node override or the spec-level SSH settings.
func (s Spec) EffectiveSSH(n NodeSpec) SSHSpec {
	if n.SSH != nil {
		return *n.SSH
	}
	return s.SSH
}

func isDNSSubdomain(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// isRegistryHost accepts a DNS name or IP with an optional port. The value
// becomes a directory under /etc/containerd/certs.d, so nothing else passes.
func isRegistryHost(value string) bool {
	host := value
	if strings.Contains(value, ":") {
		h, port, err := net.SplitHostPort(value)
		if err != nil {
			return false
		}
		if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
			return false
		}
		host = h
	}
	return isDNSSubdomain(host) || net.ParseIP(host) != nil
}

func cidrsOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
