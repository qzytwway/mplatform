package provision

import (
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type renderScenario struct {
	name    string
	cluster clusterParams
	first   nodeParams
	joining nodeParams
	worker  nodeParams
}

func renderScenarios(t *testing.T) []renderScenario {
	t.Helper()
	worker := nodeParams{Name: "worker-1", IP: "192.168.10.21", Role: RoleWorker, Arch: "arm64", OSFamily: "rhel", CPU: 16, MemoryMiB: 65536}
	single := validSpec(t)
	single.Sources.RegistryMirrors = []RegistryMirror{
		{Registry: "docker.io", Endpoint: "https://mirror.example.com"},
		{Registry: "docker.io", Endpoint: "https://mirror2.example.com"},
		{Registry: "quay.io", Endpoint: "https://quay-mirror.example.com"},
	}
	single.Normalize()

	vip := haSpec(t, ModeKubeVIP, "192.168.10.100")
	vip.SchedulableControlPlane = true
	vip.Sources.ImageRepository = "registry.example.com/k8s"
	vip.Normalize()

	lb := haSpec(t, ModeExternalLB, "api.example.com")
	lb.ControlPlane.EndpointPort = 8443
	lb.Normalize()

	cp := func(name, ip string) nodeParams {
		return nodeParams{Name: name, IP: ip, Role: RoleControlPlane, Arch: "amd64", OSFamily: "ubuntu", Interface: "eth0", CPU: 4, MemoryMiB: 8192}
	}
	cpNames := []string{"master-1", "master-2", "master-3"}
	return []renderScenario{
		{
			name:    "single",
			cluster: newClusterParams(single, "v1.36.6", "c2VjcmV0", []string{"master-1"}),
			first:   cp("master-1", "192.168.10.11"),
			worker:  worker,
		},
		{
			name:    "kube-vip",
			cluster: newClusterParams(vip, "v1.36.6", "c2VjcmV0", cpNames),
			first:   cp("master-1", "192.168.10.11"),
			joining: cp("master-2", "192.168.10.12"),
			worker:  worker,
		},
		{
			name:    "external-lb",
			cluster: newClusterParams(lb, "v1.37.2", "c2VjcmV0", cpNames),
			first:   cp("master-1", "192.168.10.11"),
			joining: cp("master-2", "192.168.10.12"),
			worker:  worker,
		},
	}
}

func TestRenderInitConfig(t *testing.T) {
	wantEndpoint := map[string]string{
		"single":      "192.168.10.11:6443",
		"kube-vip":    "192.168.10.100:6443",
		"external-lb": "api.example.com:8443",
	}
	for _, sc := range renderScenarios(t) {
		t.Run(sc.name, func(t *testing.T) {
			certKey := ""
			if sc.cluster.Mode != ModeSingle {
				certKey = strings.Repeat("ab", 32)
			}
			out, err := renderInitConfig(sc.cluster, sc.first, "abcdef.0123456789abcdef", certKey)
			if err != nil {
				t.Fatal(err)
			}
			docs := parseDocs(t, out)
			init, cluster, kubelet, proxy := docs["InitConfiguration"], docs["ClusterConfiguration"], docs["KubeletConfiguration"], docs["KubeProxyConfiguration"]

			expect(t, "init.nodeRegistration.name", get(init, "nodeRegistration", "name"), "master-1")
			expect(t, "init.criSocket", get(init, "nodeRegistration", "criSocket"), "unix:///run/containerd/containerd.sock")
			expect(t, "init.kubeletExtraArgs", get(init, "nodeRegistration", "kubeletExtraArgs"), []any{map[string]any{"name": "node-ip", "value": "192.168.10.11"}})
			expect(t, "init.advertiseAddress", get(init, "localAPIEndpoint", "advertiseAddress"), "192.168.10.11")
			expect(t, "init.token ttl", get(init, "bootstrapTokens").([]any)[0].(map[string]any)["ttl"], "2h0m0s")
			expect(t, "init.patches", get(init, "patches", "directory"), "/etc/kubernetes/mplatform/patches")
			if certKey == "" {
				expect(t, "init.certificateKey", get(init, "certificateKey"), nil)
			} else {
				expect(t, "init.certificateKey", get(init, "certificateKey"), certKey)
			}
			_, hasTaints := get(init, "nodeRegistration").(map[string]any)["taints"]
			if hasTaints != sc.cluster.Schedulable {
				t.Errorf("taints present = %v, want %v", hasTaints, sc.cluster.Schedulable)
			}

			expect(t, "kubernetesVersion", get(cluster, "kubernetesVersion"), sc.cluster.KubernetesVersion)
			expect(t, "controlPlaneEndpoint", get(cluster, "controlPlaneEndpoint"), wantEndpoint[sc.name])
			expect(t, "imageRepository", get(cluster, "imageRepository"), sc.cluster.ImageRepository)
			expect(t, "podSubnet", get(cluster, "networking", "podSubnet"), "10.244.0.0/16")
			expect(t, "serviceSubnet", get(cluster, "networking", "serviceSubnet"), "10.96.0.0/12")
			sans := toStrings(get(cluster, "apiServer", "certSANs"))
			for _, want := range append([]string{sc.cluster.EndpointHost, "192.168.10.11", "master-1", "127.0.0.1", "localhost"}, sc.cluster.CertSANs...) {
				if !contains(sans, want) {
					t.Errorf("certSANs %v missing %s", sans, want)
				}
			}
			if sc.name != "single" && (!contains(sans, "192.168.10.13") || !contains(sans, "master-3")) {
				t.Errorf("certSANs %v missing other control planes", sans)
			}

			apiArgs := args(t, get(cluster, "apiServer", "extraArgs"))
			wantAPIArgs := map[string]string{
				"audit-policy-file":          "/etc/kubernetes/mplatform/audit-policy.yaml",
				"audit-log-path":             "/var/log/kubernetes/audit/audit.log",
				"audit-log-maxage":           "30",
				"audit-log-maxbackup":        "10",
				"audit-log-maxsize":          "100",
				"encryption-provider-config": "/etc/kubernetes/mplatform/encryption-config.yaml",
				"profiling":                  "false",
				"tls-min-version":            "VersionTLS12",
				"tls-cipher-suites":          strings.Join(tlsCipherSuites, ","),
			}
			expect(t, "apiServer.extraArgs", apiArgs, wantAPIArgs)
			if _, ok := apiArgs["anonymous-auth"]; ok {
				t.Errorf("anonymous-auth must keep its default for kubeadm join discovery")
			}
			volumes := map[string]string{}
			for _, v := range get(cluster, "apiServer", "extraVolumes").([]any) {
				m := v.(map[string]any)
				volumes[m["hostPath"].(string)] = m["mountPath"].(string)
			}
			expect(t, "apiServer.extraVolumes", volumes, map[string]string{
				auditPolicyPath:             auditPolicyPath,
				"/var/log/kubernetes/audit": "/var/log/kubernetes/audit",
				encryptionPath:              encryptionPath,
			})
			expect(t, "controllerManager.extraArgs", args(t, get(cluster, "controllerManager", "extraArgs")), map[string]string{"profiling": "false", "terminated-pod-gc-threshold": "1000"})
			expect(t, "scheduler.extraArgs", args(t, get(cluster, "scheduler", "extraArgs")), map[string]string{"profiling": "false"})
			expect(t, "etcd.extraArgs", args(t, get(cluster, "etcd", "local", "extraArgs")), map[string]string{"quota-backend-bytes": "8589934592"})

			for key, want := range map[string]any{
				"cgroupDriver":                    "systemd",
				"failSwapOn":                      true,
				"rotateCertificates":              true,
				"readOnlyPort":                    float64(0),
				"protectKernelDefaults":           true,
				"podPidsLimit":                    float64(4096),
				"containerLogMaxSize":             "50Mi",
				"containerLogMaxFiles":            float64(5),
				"serializeImagePulls":             false,
				"maxParallelImagePulls":           float64(5),
				"shutdownGracePeriod":             "60s",
				"shutdownGracePeriodCriticalPods": "20s",
				"tlsMinVersion":                   "VersionTLS12",
			} {
				expect(t, "kubelet."+key, get(kubelet, key), want)
			}
			expect(t, "kubelet.anonymous", get(kubelet, "authentication", "anonymous", "enabled"), false)
			expect(t, "kubelet.webhook", get(kubelet, "authentication", "webhook", "enabled"), true)
			expect(t, "kubelet.authorization", get(kubelet, "authorization", "mode"), "Webhook")
			expect(t, "kubelet.evictionHard", get(kubelet, "evictionHard"), map[string]any{
				"memory.available": "500Mi", "nodefs.available": "10%", "nodefs.inodesFree": "5%", "imagefs.available": "15%",
			})
			expect(t, "kubelet.tlsCipherSuites", toStrings(get(kubelet, "tlsCipherSuites")), tlsCipherSuites)
			expect(t, "kubeProxy.mode", get(proxy, "mode"), "iptables")
		})
	}
}

func TestRenderJoinConfig(t *testing.T) {
	for _, sc := range renderScenarios(t)[1:] {
		t.Run(sc.name, func(t *testing.T) {
			certKey := strings.Repeat("cd", 32)
			out, err := renderJoinConfig(sc.cluster, sc.joining, "abcdef.0123456789abcdef", "sha256:1234", certKey)
			if err != nil {
				t.Fatal(err)
			}
			join := parseDocs(t, out)["JoinConfiguration"]
			expect(t, "apiServerEndpoint", get(join, "discovery", "bootstrapToken", "apiServerEndpoint"), sc.cluster.EndpointAddress())
			expect(t, "token", get(join, "discovery", "bootstrapToken", "token"), "abcdef.0123456789abcdef")
			expect(t, "caCertHashes", get(join, "discovery", "bootstrapToken", "caCertHashes"), []any{"sha256:1234"})
			expect(t, "controlPlane.advertiseAddress", get(join, "controlPlane", "localAPIEndpoint", "advertiseAddress"), "192.168.10.12")
			expect(t, "controlPlane.certificateKey", get(join, "controlPlane", "certificateKey"), certKey)
			expect(t, "patches", get(join, "patches", "directory"), "/etc/kubernetes/mplatform/patches")
			if sc.cluster.Schedulable {
				expect(t, "taints", get(join, "nodeRegistration", "taints"), []any{})
			}

			out, err = renderJoinConfig(sc.cluster, sc.worker, "abcdef.0123456789abcdef", "sha256:1234", "")
			if err != nil {
				t.Fatal(err)
			}
			worker := parseDocs(t, out)["JoinConfiguration"]
			expect(t, "worker.controlPlane", get(worker, "controlPlane"), nil)
			expect(t, "worker.taints", get(worker, "nodeRegistration", "taints"), nil)
			expect(t, "worker.node-ip", get(worker, "nodeRegistration", "kubeletExtraArgs"), []any{map[string]any{"name": "node-ip", "value": "192.168.10.21"}})
		})
	}
}

func TestRenderKubeletReservation(t *testing.T) {
	tests := []struct {
		cpu, memoryMiB int
		want           map[string]any
	}{
		// 80m CPU; 1024 + 819 = 1843Mi memory.
		{cpu: 4, memoryMiB: 8192, want: map[string]any{"cpu": "40m", "memory": "922Mi"}},
		// 110m CPU; 1024 + 819 + 819 + 2949 = 5611Mi memory.
		{cpu: 16, memoryMiB: 65536, want: map[string]any{"cpu": "55m", "memory": "2806Mi"}},
		{cpu: 1, memoryMiB: 900, want: map[string]any{"cpu": "30m", "memory": "128Mi"}},
	}
	for _, tt := range tests {
		out, err := renderKubeletPatch(nodeParams{CPU: tt.cpu, MemoryMiB: tt.memoryMiB})
		if err != nil {
			t.Fatal(err)
		}
		var patch map[string]any
		if err := yaml.Unmarshal(out, &patch); err != nil {
			t.Fatalf("parsing kubelet patch: %v\n%s", err, out)
		}
		expect(t, "kubeReserved", patch["kubeReserved"], tt.want)
		if patch["systemReserved"] == nil {
			t.Fatalf("systemReserved missing:\n%s", out)
		}
	}
}

func TestRenderNodeFiles(t *testing.T) {
	for _, sc := range renderScenarios(t) {
		t.Run(sc.name, func(t *testing.T) {
			files, err := prepareFiles(sc.cluster, sc.worker)
			if err != nil {
				t.Fatal(err)
			}
			byPath := map[string]string{}
			for _, f := range files {
				byPath[f.Path] = string(f.Data)
			}
			containerd := byPath["/etc/containerd/config.toml"]
			for _, want := range []string{"version = 3", "SystemdCgroup = true", "config_path = '/etc/containerd/certs.d'", `sandbox = "` + sc.cluster.ImageRepository + `/pause:`} {
				if !strings.Contains(containerd, want) {
					t.Errorf("containerd config missing %q:\n%s", want, containerd)
				}
			}
			prepare := byPath[scriptDir+"/prepare.sh"]
			for _, want := range []string{"OS_FAMILY='rhel'", "kubeadm.sha256", "runc.arm64", "linux-arm64", "'10250/tcp'", "'4789/udp'"} {
				if !strings.Contains(prepare, want) {
					t.Errorf("prepare script missing %q", want)
				}
			}

			docker, quay := byPath["/etc/containerd/certs.d/docker.io/hosts.toml"], byPath["/etc/containerd/certs.d/quay.io/hosts.toml"]
			if sc.name == "single" {
				for _, want := range []string{`server = "https://registry-1.docker.io"`, `[host."https://mirror.example.com"]`, `[host."https://mirror2.example.com"]`} {
					if !strings.Contains(docker, want) {
						t.Errorf("docker.io hosts.toml missing %q:\n%s", want, docker)
					}
				}
				if !strings.Contains(quay, `[host."https://quay-mirror.example.com"]`) || !strings.Contains(quay, `server = "https://quay.io"`) {
					t.Errorf("quay.io hosts.toml unexpected:\n%s", quay)
				}
			} else if docker != "" || quay != "" {
				t.Errorf("hosts.toml rendered without mirrors")
			}

			cpFiles, err := controlPlaneFiles(sc.cluster, sc.first)
			if err != nil {
				t.Fatal(err)
			}
			var vip string
			for _, f := range cpFiles {
				if f.Path == kubeVIPPath {
					vip = string(f.Data)
				}
			}
			if (vip != "") != (sc.cluster.Mode == ModeKubeVIP) {
				t.Fatalf("kube-vip manifest present = %v for mode %s", vip != "", sc.cluster.Mode)
			}
			if vip != "" {
				pod := map[string]any{}
				if err := yaml.Unmarshal([]byte(vip), &pod); err != nil {
					t.Fatalf("parsing kube-vip manifest: %v", err)
				}
				container := get(pod, "spec", "containers").([]any)[0].(map[string]any)
				env := map[string]any{}
				for _, e := range container["env"].([]any) {
					m := e.(map[string]any)
					env[m["name"].(string)] = m["value"]
				}
				expect(t, "kube-vip address", env["address"], "192.168.10.100")
				expect(t, "kube-vip interface", env["vip_interface"], "eth0")
				expect(t, "kube-vip arp", env["vip_arp"], "true")
				expect(t, "kube-vip kubeconfig", get(pod, "spec", "volumes").([]any)[0].(map[string]any)["hostPath"].(map[string]any)["path"], adminConfPath)
				initVIP, err := renderKubeVIP(sc.cluster, sc.first, superAdminConfPath)
				if err != nil || !strings.Contains(string(initVIP), superAdminConfPath) {
					t.Fatalf("kube-vip init manifest does not use super-admin.conf: %v", err)
				}
			}

			script, err := renderInitScript(sc.cluster)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(script), "--upload-certs") != (sc.cluster.Mode != ModeSingle) {
				t.Errorf("init script --upload-certs mismatch for %s:\n%s", sc.cluster.Mode, script)
			}
			if strings.Contains(string(script), "kube-vip") != (sc.cluster.Mode == ModeKubeVIP) {
				t.Errorf("init script kube-vip mismatch for %s:\n%s", sc.cluster.Mode, script)
			}

			calico, err := renderCalicoInstallation(sc.cluster)
			if err != nil {
				t.Fatal(err)
			}
			installation := map[string]any{}
			if err := yaml.Unmarshal(calico, &installation); err != nil {
				t.Fatal(err)
			}
			pool := get(installation, "spec", "calicoNetwork", "ipPools").([]any)[0].(map[string]any)
			expect(t, "calico cidr", pool["cidr"], "10.244.0.0/16")
			expect(t, "calico encapsulation", pool["encapsulation"], "VXLANCrossSubnet")
		})
	}
}

func TestDownloadURLWithMirror(t *testing.T) {
	got := nodeDownloads("v1.36.6", "1.36", "amd64", "https://mirror.example.com/k8s")
	expect(t, "kubeadm", got[downloadKubeadm].URL, "https://mirror.example.com/k8s/dl.k8s.io/release/v1.36.6/bin/linux/amd64/kubeadm")
	expect(t, "containerd sum", got[downloadContainerd].SumURL, "https://mirror.example.com/k8s/github.com/containerd/containerd/releases/download/v2.4.2/containerd-2.4.2-linux-amd64.tar.gz.sha256sum")
	expect(t, "crictl", got[downloadCrictl].URL, "https://mirror.example.com/k8s/github.com/kubernetes-sigs/cri-tools/releases/download/v1.36.0/crictl-v1.36.0-linux-amd64.tar.gz")
	crds, _ := calicoManifestURLs("")
	expect(t, "calico crds", crds, "https://raw.githubusercontent.com/projectcalico/calico/v3.33.0/manifests/operator-crds.yaml")
}

func parseDocs(t *testing.T, out []byte) map[string]map[string]any {
	t.Helper()
	docs := map[string]map[string]any{}
	for _, doc := range strings.Split(string(out), "\n---\n") {
		m := map[string]any{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
			t.Fatalf("parsing rendered YAML: %v\n%s", err, doc)
		}
		docs[m["kind"].(string)] = m
	}
	return docs
}

func get(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func args(t *testing.T, v any) map[string]string {
	t.Helper()
	out := map[string]string{}
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("extraArgs is %T, want a list of name/value pairs", v)
	}
	for _, item := range list {
		m := item.(map[string]any)
		out[m["name"].(string)] = m["value"].(string)
	}
	return out
}

func toStrings(v any) []string {
	var out []string
	for _, item := range v.([]any) {
		out = append(out, item.(string))
	}
	return out
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func expect(t *testing.T, field string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", field, got, want)
	}
}
