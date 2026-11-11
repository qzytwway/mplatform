package provision

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testPrivateKey(t *testing.T) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}

func validSpec(t *testing.T) Spec {
	t.Helper()
	return Spec{
		Name:            "dev-01",
		KubernetesMinor: "1.36",
		ControlPlane:    ControlPlaneSpec{Mode: ModeSingle},
		SSH:             SSHSpec{User: "root", AuthType: AuthKey, PrivateKey: testPrivateKey(t)},
		Nodes: []NodeSpec{
			{IP: "192.168.10.11", Role: RoleControlPlane},
			{IP: "192.168.10.21", Role: RoleWorker},
		},
	}
}

func haSpec(t *testing.T, mode, endpoint string) Spec {
	s := validSpec(t)
	s.ControlPlane = ControlPlaneSpec{Mode: mode, Endpoint: endpoint}
	s.Nodes = []NodeSpec{
		{IP: "192.168.10.11", Role: RoleControlPlane},
		{IP: "192.168.10.12", Role: RoleControlPlane},
		{IP: "192.168.10.13", Role: RoleControlPlane},
		{IP: "192.168.10.21", Role: RoleWorker},
	}
	return s
}

func TestSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		wantErr string
	}{
		{name: "valid single", mutate: func(*Spec) {}},
		{name: "uppercase name", mutate: func(s *Spec) { s.Name = "Dev" }, wantErr: "name must"},
		{name: "name ending with dash", mutate: func(s *Spec) { s.Name = "dev-" }, wantErr: "name must"},
		{name: "name too long", mutate: func(s *Spec) { s.Name = strings.Repeat("a", 64) }, wantErr: "name must"},
		{name: "unsupported minor", mutate: func(s *Spec) { s.KubernetesMinor = "1.30" }, wantErr: "not supported"},
		{name: "invalid ip", mutate: func(s *Spec) { s.Nodes[1].IP = "300.1.1.1" }, wantErr: "not a valid IPv4"},
		{name: "ipv6 ip", mutate: func(s *Spec) { s.Nodes[1].IP = "fd00::1" }, wantErr: "not a valid IPv4"},
		{name: "duplicate ip", mutate: func(s *Spec) { s.Nodes[1].IP = s.Nodes[0].IP }, wantErr: "more than once"},
		{name: "duplicate hostname", mutate: func(s *Spec) {
			s.Nodes[0].Hostname = "node-a"
			s.Nodes[1].Hostname = "node-a"
		}, wantErr: "more than one node"},
		{name: "invalid hostname", mutate: func(s *Spec) { s.Nodes[0].Hostname = "node_a" }, wantErr: "not a valid node name"},
		{name: "invalid role", mutate: func(s *Spec) { s.Nodes[1].Role = "master" }, wantErr: "role must"},
		{name: "two control planes in single mode", mutate: func(s *Spec) { s.Nodes[1].Role = RoleControlPlane }, wantErr: "exactly 1 control-plane"},
		{name: "zero workers without schedulable control plane", mutate: func(s *Spec) { s.Nodes = s.Nodes[:1] }, wantErr: "schedulableControlPlane"},
		{name: "zero workers with schedulable control plane", mutate: func(s *Spec) {
			s.Nodes = s.Nodes[:1]
			s.SchedulableControlPlane = true
		}},
		{name: "pod and service cidr overlap", mutate: func(s *Spec) {
			s.Network.PodCIDR = "10.96.0.0/16"
			s.Network.ServiceCIDR = "10.96.0.0/12"
		}, wantErr: "overlaps"},
		{name: "service cidr inside pod cidr", mutate: func(s *Spec) {
			s.Network.PodCIDR = "10.0.0.0/8"
			s.Network.ServiceCIDR = "10.96.0.0/12"
		}, wantErr: "overlaps"},
		{name: "invalid pod cidr", mutate: func(s *Spec) { s.Network.PodCIDR = "10.244.0.0" }, wantErr: "podCIDR"},
		{name: "pod cidr too small", mutate: func(s *Spec) { s.Network.PodCIDR = "10.244.0.0/24" }, wantErr: "podCIDR must be"},
		{name: "service cidr too large", mutate: func(s *Spec) { s.Network.ServiceCIDR = "10.0.0.0/8" }, wantErr: "serviceCIDR must not"},
		{name: "node inside pod cidr", mutate: func(s *Spec) { s.Nodes[1].IP = "10.244.1.5" }, wantErr: "inside the pod or service CIDR"},
		{name: "missing ssh user", mutate: func(s *Spec) { s.SSH.User = "" }, wantErr: "ssh.user"},
		{name: "invalid private key", mutate: func(s *Spec) { s.SSH.PrivateKey = "not a key" }, wantErr: "not a valid private key"},
		{name: "password auth without password", mutate: func(s *Spec) { s.SSH.AuthType = AuthPassword }, wantErr: "password is required"},
		{name: "node override replaces top-level ssh", mutate: func(s *Spec) {
			s.SSH = SSHSpec{}
			for i := range s.Nodes {
				s.Nodes[i].SSH = &SSHSpec{User: "ops", AuthType: AuthPassword, Password: "secret"}
			}
		}},
		{name: "invalid node override", mutate: func(s *Spec) {
			s.Nodes[1].SSH = &SSHSpec{User: "ops", AuthType: "token"}
		}, wantErr: "nodes[1].ssh.authType"},
		{name: "binary mirror without scheme", mutate: func(s *Spec) { s.Sources.BinaryMirror = "mirror.local" }, wantErr: "binaryMirror"},
		{name: "registry mirror with scheme in registry", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{{Registry: "https://docker.io", Endpoint: "https://m.local"}}
		}, wantErr: "registry host"},
		{name: "registry mirror dot", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{{Registry: ".", Endpoint: "https://m.local"}}
		}, wantErr: "registry host"},
		{name: "registry mirror dot dot", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{{Registry: "..", Endpoint: "https://m.local"}}
		}, wantErr: "registry host"},
		{name: "registry mirror with path", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{{Registry: "docker.io/../x", Endpoint: "https://m.local"}}
		}, wantErr: "registry host"},
		{name: "registry mirror with invalid port", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{{Registry: "registry.local:0", Endpoint: "https://m.local"}}
		}, wantErr: "registry host"},
		{name: "registry mirror host and ip with port", mutate: func(s *Spec) {
			s.Sources.RegistryMirrors = []RegistryMirror{
				{Registry: "registry.local:5000", Endpoint: "https://m.local"},
				{Registry: "10.0.0.1:5000", Endpoint: "https://m.local"},
			}
		}},
		{name: "image repository with scheme", mutate: func(s *Spec) { s.Sources.ImageRepository = "https://r.local/k8s" }, wantErr: "imageRepository"},
		{name: "invalid mode", mutate: func(s *Spec) { s.ControlPlane.Mode = "ha" }, wantErr: "controlPlane.mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(t)
			tt.mutate(&s)
			s.Normalize()
			assertValidation(t, s.Validate(), tt.wantErr)
		})
	}
}

func TestSpecValidateHighAvailability(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		endpoint string
		mutate   func(*Spec)
		wantErr  string
	}{
		{name: "kube-vip valid", mode: ModeKubeVIP, endpoint: "192.168.10.100"},
		{name: "kube-vip requires endpoint", mode: ModeKubeVIP, wantErr: "virtual IP"},
		{name: "kube-vip rejects dns endpoint", mode: ModeKubeVIP, endpoint: "api.example.com", wantErr: "virtual IP"},
		{name: "kube-vip vip equals node ip", mode: ModeKubeVIP, endpoint: "192.168.10.11", wantErr: "must not be a node IP"},
		{name: "kube-vip requires port 6443", mode: ModeKubeVIP, endpoint: "192.168.10.100", mutate: func(s *Spec) {
			s.ControlPlane.EndpointPort = 8443
		}, wantErr: "port 6443"},
		{name: "kube-vip with two control planes", mode: ModeKubeVIP, endpoint: "192.168.10.100", mutate: func(s *Spec) {
			s.Nodes[2].Role = RoleWorker
		}, wantErr: "exactly 3 control-plane"},
		{name: "external lb valid dns", mode: ModeExternalLB, endpoint: "api.example.com"},
		{name: "external lb custom port", mode: ModeExternalLB, endpoint: "10.0.0.5", mutate: func(s *Spec) {
			s.ControlPlane.EndpointPort = 8443
		}},
		{name: "external lb requires endpoint", mode: ModeExternalLB, wantErr: "endpoint is required"},
		{name: "external lb invalid endpoint", mode: ModeExternalLB, endpoint: "bad host!", wantErr: "not a valid IP address or DNS name"},
		{name: "external lb with one control plane", mode: ModeExternalLB, endpoint: "10.0.0.5", mutate: func(s *Spec) {
			s.Nodes[1].Role = RoleWorker
			s.Nodes[2].Role = RoleWorker
		}, wantErr: "exactly 3 control-plane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := haSpec(t, tt.mode, tt.endpoint)
			if tt.mutate != nil {
				tt.mutate(&s)
			}
			s.Normalize()
			assertValidation(t, s.Validate(), tt.wantErr)
		})
	}
}

func TestSpecNormalizeAndSanitize(t *testing.T) {
	s := validSpec(t)
	s.KubernetesMinor = ""
	s.ControlPlane.Endpoint = "ignored"
	s.Nodes[1].Hostname = " Worker-1 "
	s.Nodes[1].SSH = &SSHSpec{User: "ops", AuthType: AuthPassword, Password: "pw"}
	s.Sources.BinaryMirror = "https://mirror.local/"
	s.Normalize()

	if s.KubernetesMinor != DefaultMinor || s.ControlPlane.EndpointPort != 6443 || s.SSH.Port != 22 {
		t.Fatalf("defaults not applied: %+v", s)
	}
	if s.ControlPlane.Endpoint != "" || s.EndpointAddress() != "192.168.10.11:6443" {
		t.Fatalf("single mode endpoint = %q / %q", s.ControlPlane.Endpoint, s.EndpointAddress())
	}
	if s.Network.PodCIDR != defaultPodCIDR || s.Network.ServiceCIDR != defaultServiceCIDR || s.Network.DNSDomain != defaultDNSDomain {
		t.Fatalf("network defaults not applied: %+v", s.Network)
	}
	if s.Nodes[1].Hostname != "worker-1" || s.Sources.BinaryMirror != "https://mirror.local" {
		t.Fatalf("normalization failed: %q %q", s.Nodes[1].Hostname, s.Sources.BinaryMirror)
	}

	clean := s.Sanitized()
	if clean.SSH.PrivateKey != "" || clean.Nodes[1].SSH.Password != "" {
		t.Fatalf("sanitized spec still has secrets")
	}
	if s.SSH.PrivateKey == "" || s.Nodes[1].SSH.Password != "pw" {
		t.Fatalf("sanitize mutated the original spec")
	}
}

func assertValidation(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("Validate() error = %v, want containing %q", err, wantErr)
	}
}
