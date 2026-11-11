package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/klog/v2"
)

// Check names are fixed; the frontend translates them.
const (
	checkSSH       = "ssh"
	checkPrivilege = "privilege"
	checkOS        = "os"
	checkArch      = "arch"
	checkCgroup    = "cgroup"
	checkResources = "resources"
	checkDisk      = "disk"
	checkPorts     = "ports"
	checkResidue   = "residue"
	checkTimeSync  = "timeSync"
	checkDownload  = "download"
	checkUnique    = "unique"
	checkVIP       = "vip"

	checkPass = "pass"
	checkWarn = "warn"
	checkFail = "fail"
)

var checkOrder = []string{
	checkSSH, checkPrivilege, checkOS, checkArch, checkCgroup, checkResources, checkDisk,
	checkPorts, checkResidue, checkTimeSync, checkDownload, checkUnique, checkVIP,
}

const (
	precheckTimeout       = 2 * time.Minute
	minControlPlaneCPU    = 2
	minControlPlaneMemMiB = 1700
	recControlPlaneCPU    = 4
	recControlPlaneMemMiB = 7000
	minWorkerMemMiB       = 1000
	recWorkerCPU          = 2
	recWorkerMemMiB       = 1700
	minDiskGiB            = 10
	recDiskGiB            = 50
	maxClockSkew          = 60 * time.Second
	notApplicable         = "Not applicable"
	notChecked            = "Not checked"
)

var (
	controlPlaneListenPorts = []int{6443, 2379, 2380, 10250, 10256, 10257, 10259}
	workerListenPorts       = []int{10250, 10256}
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type NodePrecheck struct {
	IP        string  `json:"ip"`
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
	CPU       int     `json:"cpu"`
	MemoryMiB int     `json:"memoryMiB"`
	Checks    []Check `json:"checks"`
}

// nodeFacts is what later steps need from precheck; stored on the node.
type nodeFacts struct {
	OSFamily  string `json:"osFamily"`
	Arch      string `json:"arch"`
	Interface string `json:"interface"`
	CPU       int    `json:"cpu"`
	MemoryMiB int    `json:"memoryMiB"`
}

// hostReport is the outcome of checking one host.
type hostReport struct {
	result NodePrecheck
	facts  *nodeFacts
	raw    map[string]string
}

// precheckNodes checks every node in parallel, then checks uniqueness across
// nodes. dial opens a connection to nodes[i].
func precheckNodes(ctx context.Context, spec Spec, c clusterParams, nodes []NodeSpec, dial func(context.Context, int) (Executor, error)) []hostReport {
	reports := make([]hostReport, len(nodes))
	sem := make(chan struct{}, maxParallelNodes)
	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reports[i] = checkHost(ctx, spec, c, node, func(ctx context.Context) (Executor, error) { return dial(ctx, i) })
		}()
	}
	wg.Wait()
	checkUniqueness(reports)
	return reports
}

func checkHost(ctx context.Context, spec Spec, c clusterParams, node NodeSpec, dial func(context.Context) (Executor, error)) hostReport {
	ctx, cancel := context.WithTimeout(ctx, precheckTimeout)
	defer cancel()
	report := hostReport{result: NodePrecheck{IP: node.IP, Hostname: node.Hostname}}
	checks := map[string]Check{}
	exec, err := dial(ctx)
	if err != nil {
		checks[checkSSH] = Check{Status: checkFail, Message: err.Error()}
		report.result.Checks = orderedChecks(checks, notChecked+": SSH login failed")
		return report
	}
	defer func() { _ = exec.Close() }()
	privilege, err := exec.Privilege()
	if err != nil {
		checks[checkSSH] = Check{Status: checkPass, Message: "Connected"}
		checks[checkPrivilege] = Check{Status: checkFail, Message: err.Error()}
		report.result.Checks = orderedChecks(checks, notChecked+": root or sudo access is required")
		return report
	}
	checks[checkPrivilege] = Check{Status: checkPass, Message: privilegeMessage(privilege)}
	started := time.Now()
	script, err := renderPrecheckScript(c, node.Role == RoleControlPlane)
	var out []byte
	if err == nil {
		out, err = exec.Output(ctx, string(script))
	}
	if err != nil {
		checks[checkSSH] = Check{Status: checkFail, Message: "Connected, but collecting host facts failed: " + err.Error()}
		report.result.Checks = orderedChecks(checks, notChecked+": collecting host facts failed")
		return report
	}
	raw, probes := parseFacts(string(out))
	report.raw = raw
	evaluateHost(&report, checks, spec, node, raw, probes, started)
	report.result.Checks = orderedChecks(checks, notChecked)
	return report
}

func privilegeMessage(privilege string) string {
	switch privilege {
	case privilegeRoot:
		return "Logged in as root"
	case privilegeSudo:
		return "Passwordless sudo"
	default:
		return "sudo with the login password"
	}
}

func parseFacts(out string) (map[string]string, []string) {
	raw := map[string]string{}
	var probes []string
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if key == "probe" {
			probes = append(probes, value)
			continue
		}
		raw[key] = value
	}
	return raw, probes
}

func evaluateHost(report *hostReport, checks map[string]Check, spec Spec, node NodeSpec, raw map[string]string, probes []string, started time.Time) {
	res := &report.result
	if res.Hostname == "" {
		res.Hostname = strings.ToLower(raw["hostname"])
	}
	res.OS = raw["os_name"]
	res.CPU = atoi(raw["cpu"])
	res.MemoryMiB = atoi(raw["mem_kib"]) / 1024
	family, osCheck := checkOperatingSystem(raw)
	arch, archCheck := checkArchitecture(raw["arch"])
	res.Arch = arch
	iface, cidr := interfaceFor(raw["addrs"], node.IP)

	if iface == "" {
		checks[checkSSH] = Check{Status: checkFail, Message: fmt.Sprintf("Connected, but %s is not assigned to any interface on this host; use the host's own address", node.IP)}
	} else {
		checks[checkSSH] = Check{Status: checkPass, Message: fmt.Sprintf("Connected; %s is on %s", node.IP, iface)}
	}
	checks[checkOS] = osCheck
	checks[checkArch] = archCheck
	checks[checkCgroup] = checkCgroupVersion(raw["cgroup"])
	checks[checkResources] = checkHostResources(node.Role, res.CPU, res.MemoryMiB)
	checks[checkDisk] = checkDiskSpace(atoi(raw["var_avail_kib"]))
	checks[checkPorts] = checkListeningPorts(node.Role, raw["listening"])
	checks[checkResidue] = checkLeftovers(raw["residue_cluster"], raw["residue_other"])
	checks[checkTimeSync] = checkClock(raw["ntp"], raw["epoch"], started)
	checks[checkDownload] = checkDownloads(probes, spec.Sources.RegistryMirrors)
	checks[checkVIP] = checkVirtualIP(spec, node, iface, cidr, raw["vip_in_use"])
	if family != "" && arch != "" {
		report.facts = &nodeFacts{OSFamily: family, Arch: arch, Interface: iface, CPU: res.CPU, MemoryMiB: res.MemoryMiB}
	}
}

func checkOperatingSystem(raw map[string]string) (string, Check) {
	id, version, name := raw["os_id"], raw["os_version"], defaultString(raw["os_name"], "unknown OS")
	major, _, _ := strings.Cut(version, ".")
	switch id {
	case "ubuntu":
		if version == "22.04" || version == "24.04" {
			return "ubuntu", Check{Status: checkPass, Message: name}
		}
		return "", Check{Status: checkFail, Message: name + " is not supported; use Ubuntu 22.04 or 24.04"}
	case "rhel", "rocky", "almalinux", "centos":
		if major == "9" || major == "10" {
			return "rhel", Check{Status: checkPass, Message: name}
		}
		if major == "7" || major == "8" {
			return "", Check{Status: checkFail, Message: name + " uses cgroup v1, which kubelet 1.35 and later refuses to run on; use version 9 or 10"}
		}
		return "", Check{Status: checkFail, Message: name + " is not supported; use version 9 or 10"}
	}
	return "", Check{Status: checkFail, Message: name + " is not supported; supported systems are Ubuntu 22.04/24.04 and RHEL, Rocky Linux, AlmaLinux or CentOS Stream 9/10"}
}

func checkArchitecture(machine string) (string, Check) {
	switch machine {
	case "x86_64":
		return "amd64", Check{Status: checkPass, Message: "amd64"}
	case "aarch64", "arm64":
		return "arm64", Check{Status: checkPass, Message: "arm64"}
	}
	return "", Check{Status: checkFail, Message: defaultString(machine, "unknown") + " is not supported; use amd64 or arm64"}
}

func checkCgroupVersion(fs string) Check {
	switch fs {
	case "cgroup2fs":
		return Check{Status: checkPass, Message: "cgroup v2"}
	case "tmpfs":
		return Check{Status: checkFail, Message: "cgroup v1 is in use; kubelet 1.35 and later requires cgroup v2"}
	}
	return Check{Status: checkFail, Message: "Unknown cgroup filesystem " + defaultString(fs, "(none)")}
}

func checkHostResources(role string, cpu, memMiB int) Check {
	summary := fmt.Sprintf("%d CPU, %.1f GiB memory", cpu, float64(memMiB)/1024)
	if role == RoleControlPlane {
		if cpu < minControlPlaneCPU || memMiB < minControlPlaneMemMiB {
			return Check{Status: checkFail, Message: summary + "; a control plane needs at least 2 CPU and 2 GiB memory"}
		}
		if cpu < recControlPlaneCPU || memMiB < recControlPlaneMemMiB {
			return Check{Status: checkWarn, Message: summary + "; 4 CPU and 8 GiB memory are recommended for a control plane"}
		}
		return Check{Status: checkPass, Message: summary}
	}
	if cpu < 1 || memMiB < minWorkerMemMiB {
		return Check{Status: checkFail, Message: summary + "; a worker needs at least 1 CPU and 1 GiB memory"}
	}
	if cpu < recWorkerCPU || memMiB < recWorkerMemMiB {
		return Check{Status: checkWarn, Message: summary + "; 2 CPU and 2 GiB memory are recommended for a worker"}
	}
	return Check{Status: checkPass, Message: summary}
}

func checkDiskSpace(availKiB int) Check {
	gib := float64(availKiB) / 1024 / 1024
	message := fmt.Sprintf("%.1f GiB free under /var", gib)
	switch {
	case gib < minDiskGiB:
		return Check{Status: checkFail, Message: message + "; at least 10 GiB is required"}
	case gib < recDiskGiB:
		return Check{Status: checkWarn, Message: message + "; 50 GiB or more is recommended"}
	}
	return Check{Status: checkPass, Message: message}
}

func checkListeningPorts(role, listening string) Check {
	required := workerListenPorts
	if role == RoleControlPlane {
		required = controlPlaneListenPorts
	}
	open := map[int]bool{}
	for _, p := range strings.Split(listening, ",") {
		open[atoi(p)] = true
	}
	var busy []string
	for _, p := range required {
		if open[p] {
			busy = append(busy, strconv.Itoa(p))
		}
	}
	if len(busy) > 0 {
		return Check{Status: checkFail, Message: "Ports already in use: " + strings.Join(busy, ", ")}
	}
	return Check{Status: checkPass, Message: "Required ports are free"}
}

func checkLeftovers(cluster, other string) Check {
	if cluster != "" {
		return Check{Status: checkFail, Message: "This host already belongs to a Kubernetes cluster: " + strings.ReplaceAll(cluster, ",", ", ") + "; reset it first"}
	}
	if other != "" {
		return Check{Status: checkWarn, Message: "Existing files will be replaced: " + strings.ReplaceAll(other, ",", ", ")}
	}
	return Check{Status: checkPass, Message: "No previous installation found"}
}

func checkClock(ntp, epoch string, started time.Time) Check {
	remote, err := strconv.ParseInt(epoch, 10, 64)
	if err == nil {
		skew := time.Duration(math.Abs(float64(remote-started.Unix()))) * time.Second
		if skew > maxClockSkew {
			return Check{Status: checkFail, Message: fmt.Sprintf("Clock differs from the platform by %s; fix the time before installing", skew)}
		}
	}
	if ntp != "yes" {
		return Check{Status: checkWarn, Message: "Time is not synchronized yet; chrony is installed and enabled during preparation"}
	}
	return Check{Status: checkPass, Message: "Time is synchronized"}
}

// checkDownloads evaluates "kind code url" probe lines. Files must download;
// a registry only has to answer.
func checkDownloads(probes []string, mirrors []RegistryMirror) Check {
	var failed, warned []string
	for _, p := range probes {
		fields := strings.Fields(p)
		if len(fields) != 3 {
			continue
		}
		kind, code, url := fields[0], fields[1], fields[2]
		if code == "nocurl" {
			warned = append(warned, "curl is not installed; downloads are verified during preparation")
			break
		}
		status := atoi(code)
		ok := status == 200 || status == 206
		if kind == "registry" {
			ok = status >= 100 && status < 500
		}
		if ok {
			continue
		}
		message := fmt.Sprintf("%s (HTTP %s)", url, code)
		if kind == "registry" && hasMirrorFor(url, mirrors) {
			warned = append(warned, message+" is unreachable; its registry mirror will be used")
			continue
		}
		failed = append(failed, message)
	}
	switch {
	case len(failed) > 0:
		return Check{Status: checkFail, Message: "Cannot download from " + strings.Join(failed, ", ")}
	case len(warned) > 0:
		return Check{Status: checkWarn, Message: strings.Join(warned, "; ")}
	}
	return Check{Status: checkPass, Message: "Download sources are reachable"}
}

func hasMirrorFor(url string, mirrors []RegistryMirror) bool {
	host := strings.SplitN(strings.TrimPrefix(url, "https://"), "/", 2)[0]
	for _, m := range mirrors {
		if m.Registry == host {
			return true
		}
	}
	return false
}

func checkVirtualIP(spec Spec, node NodeSpec, iface, cidr, inUse string) Check {
	if spec.ControlPlane.Mode != ModeKubeVIP || node.Role != RoleControlPlane {
		return Check{Status: checkPass, Message: notApplicable}
	}
	vip := spec.ControlPlane.Endpoint
	if iface == "" {
		return Check{Status: checkFail, Message: "Cannot find the interface that carries " + node.IP}
	}
	if _, network, err := net.ParseCIDR(cidr); err != nil || !network.Contains(net.ParseIP(vip)) {
		return Check{Status: checkFail, Message: fmt.Sprintf("The virtual IP %s is not in the subnet %s of %s; kube-vip ARP mode needs the same subnet", vip, cidr, iface)}
	}
	switch inUse {
	case "yes":
		return Check{Status: checkFail, Message: fmt.Sprintf("The virtual IP %s already answers ping; choose an unused address", vip)}
	case "no":
		return Check{Status: checkPass, Message: fmt.Sprintf("%s is free; kube-vip will use %s", vip, iface)}
	}
	return Check{Status: checkWarn, Message: fmt.Sprintf("Could not verify that %s is unused (ping is not available); kube-vip will use %s", vip, iface)}
}

// interfaceFor finds the interface and CIDR that carry ip in "name=cidr,..." output.
func interfaceFor(addrs, ip string) (string, string) {
	for _, entry := range strings.Split(addrs, ",") {
		name, cidr, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if addr, _, err := net.ParseCIDR(cidr); err == nil && addr.String() == ip {
			return strings.TrimSuffix(name, ":"), cidr
		}
	}
	return "", ""
}

// checkUniqueness compares node names, product_uuid and MAC addresses across
// nodes, which kubeadm requires to be unique.
func checkUniqueness(reports []hostReport) {
	owners := map[string][]int{}
	for i, r := range reports {
		if r.raw == nil {
			continue
		}
		// A set per host: bonded NICs may share one MAC on the same host.
		keys := map[string]bool{"name:" + r.result.Hostname: true}
		if uuid := strings.ToLower(r.raw["product_uuid"]); uuid != "" {
			keys["product_uuid:"+uuid] = true
		}
		for _, mac := range strings.Split(r.raw["macs"], ",") {
			if mac = strings.ToLower(strings.TrimSpace(mac)); mac != "" {
				keys["MAC:"+mac] = true
			}
		}
		for key := range keys {
			owners[key] = append(owners[key], i)
		}
	}
	problems := make([][]string, len(reports))
	for key, idx := range owners {
		if len(idx) < 2 {
			continue
		}
		kind, value, _ := strings.Cut(key, ":")
		var ips []string
		for _, i := range idx {
			ips = append(ips, reports[i].result.IP)
		}
		for _, i := range idx {
			problems[i] = append(problems[i], fmt.Sprintf("%s %s is shared by %s", kind, value, strings.Join(ips, ", ")))
		}
	}
	for i := range reports {
		if reports[i].raw == nil {
			continue
		}
		check := Check{Name: checkUnique, Status: checkPass, Message: "Node name, product_uuid and MAC addresses are unique"}
		if name := reports[i].result.Hostname; !isDNSSubdomain(name) {
			problems[i] = append(problems[i], fmt.Sprintf("hostname %q is not a valid node name; set a hostname for this node", name))
		}
		if len(problems[i]) > 0 {
			sort.Strings(problems[i])
			check = Check{Name: checkUnique, Status: checkFail, Message: strings.Join(problems[i], "; ")}
		}
		for j := range reports[i].result.Checks {
			if reports[i].result.Checks[j].Name == checkUnique {
				reports[i].result.Checks[j] = check
			}
		}
	}
}

// orderedChecks returns all 13 checks in their fixed order; checks that did
// not run fail with the given message. Whenever checks are skipped, ssh or
// privilege carries the real failure.
func orderedChecks(checks map[string]Check, skipped string) []Check {
	out := make([]Check, 0, len(checkOrder))
	for _, name := range checkOrder {
		check, ok := checks[name]
		if !ok {
			check = Check{Status: checkFail, Message: skipped}
		}
		check.Name = name
		out = append(out, check)
	}
	return out
}

// failedChecks names the checks that failed for a real reason, leaving out
// those that were skipped because an earlier check failed.
func failedChecks(result NodePrecheck) []string {
	var names []string
	for _, c := range result.Checks {
		if c.Status == checkFail && !strings.HasPrefix(c.Message, notChecked) {
			names = append(names, c.Name)
		}
	}
	return names
}

// standalonePrecheck checks the nodes of a spec without storing anything or
// recording host keys.
func standalonePrecheck(ctx context.Context, spec Spec, kubeVersion string) (bool, []NodePrecheck) {
	redact := newRedactor()
	redact.Add(spec.SSH.Password, spec.SSH.PrivateKey)
	for _, n := range spec.Nodes {
		if n.SSH != nil {
			redact.Add(n.SSH.Password, n.SSH.PrivateKey)
		}
	}
	c := newClusterParams(spec, kubeVersion, "", nil)
	reports := precheckNodes(ctx, spec, c, spec.Nodes, func(ctx context.Context, i int) (Executor, error) {
		node := spec.Nodes[i]
		s := spec.EffectiveSSH(node)
		return dialSSH(ctx, sshTarget{IP: node.IP, Port: s.Port, User: s.User, AuthType: s.AuthType, PrivateKey: s.PrivateKey, Password: s.Password}, redact.Redact)
	})
	passed := true
	results := make([]NodePrecheck, len(reports))
	for i, r := range reports {
		results[i] = r.result
		if len(failedChecks(r.result)) > 0 {
			passed = false
		}
	}
	return passed, results
}

func runPrecheckStep(ctx context.Context, r *run) error {
	specHostnames := map[string]string{}
	for _, n := range r.spec.Nodes {
		specHostnames[n.IP] = n.Hostname
	}
	nodes := make([]NodeSpec, len(r.nodes))
	for i, n := range r.nodes {
		nodes[i] = NodeSpec{IP: n.IP, Hostname: specHostnames[n.IP], Role: n.Role}
		r.setNode(n, statusRunning, "Running precheck", "")
	}
	reports := precheckNodes(ctx, r.spec, r.clusterParams(), nodes, func(ctx context.Context, i int) (Executor, error) {
		return r.runner.connect(ctx, r, r.nodes[i])
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var failures []string
	for i, report := range reports {
		node := r.nodes[i]
		for _, c := range report.result.Checks {
			r.log(node, fmt.Sprintf("%s: %s - %s", c.Name, c.Status, c.Message))
		}
		if err := saveNodePrecheck(node, report); err != nil {
			return err
		}
		if failed := failedChecks(report.result); len(failed) > 0 {
			message := "Precheck failed: " + strings.Join(failed, ", ")
			r.setNode(node, statusFailed, "", message)
			failures = append(failures, fmt.Sprintf("%s (%s)", node.IP, strings.Join(failed, ", ")))
			continue
		}
		r.setNode(node, statusSucceeded, "", "")
	}
	if len(failures) > 0 {
		return fmt.Errorf("precheck failed on %d node(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

func saveNodePrecheck(node *model.ProvisionNode, report hostReport) error {
	result, err := json.Marshal(report.result)
	if err != nil {
		return err
	}
	updates := map[string]any{"precheck": model.JSONField(result)}
	if report.result.Hostname != "" {
		node.Hostname = report.result.Hostname
		updates["hostname"] = node.Hostname
	}
	if report.facts != nil {
		facts, err := json.Marshal(report.facts)
		if err != nil {
			return err
		}
		node.Facts = facts
		updates["facts"] = model.JSONField(facts)
	}
	node.Precheck = result
	if err := model.DB.Model(&model.ProvisionNode{}).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
		klog.Errorf("Failed to save precheck of provision node %d: %v", node.ID, err)
		return err
	}
	return nil
}

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
