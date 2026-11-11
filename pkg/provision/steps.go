package provision

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qzytwway/mplatform/pkg/model"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

const (
	prepareTimeout     = 30 * time.Minute
	kubeadmTimeout     = 20 * time.Minute
	cniTimeout         = 15 * time.Minute
	resetTimeout       = 10 * time.Minute
	commandTimeout     = 2 * time.Minute
	maxParallelNodes   = 10
	maxParallelWorkers = 5
	joinCredentialTTL  = 90 * time.Minute
	tokenAlphabet      = "abcdefghijklmnopqrstuvwxyz0123456789"
	uploadCertsPath    = configDir + "/kubeadm-upload-certs.yaml"
)

var bootstrapTokenPattern = regexp.MustCompile(`[a-z0-9]{6}\.[a-z0-9]{16}`)

func defaultSteps() map[string]stepFunc {
	return map[string]stepFunc{
		stepPrecheck:          runPrecheckStep,
		stepPrepare:           runPrepareStep,
		stepInitControlPlane:  runInitStep,
		stepJoinControlPlanes: runJoinControlPlanesStep,
		stepJoinWorkers:       runJoinWorkersStep,
		stepInstallCNI:        runInstallCNIStep,
		stepWaitReady:         runWaitReadyStep,
		stepRegister:          runRegisterStep,
	}
}

// sshConnect dials a node, recording its host key on first contact and
// refusing a changed key afterwards.
func sshConnect(ctx context.Context, r *run, node *model.ProvisionNode) (Executor, error) {
	exec, err := dialSSH(ctx, sshTarget{
		IP:           node.IP,
		Port:         node.SSHPort,
		User:         node.SSHUser,
		AuthType:     node.SSHAuthType,
		PrivateKey:   string(node.SSHPrivateKey),
		Password:     string(node.SSHPassword),
		KnownHostKey: node.HostKey,
	}, r.redact.Redact)
	if err != nil {
		return nil, err
	}
	if node.HostKey == "" {
		node.HostKey = exec.HostKey()
		if err := model.DB.Model(&model.ProvisionNode{}).Where("id = ?", node.ID).Update("host_key", node.HostKey).Error; err != nil {
			_ = exec.Close()
			return nil, fmt.Errorf("record host key: %w", err)
		}
		if key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(node.HostKey)); err == nil {
			r.log(node, "Recorded SSH host key "+ssh.FingerprintSHA256(key))
		}
	}
	return exec, nil
}

// connect returns an executor that can run commands as root.
func (r *run) connect(ctx context.Context, node *model.ProvisionNode) (Executor, error) {
	exec, err := r.runner.connect(ctx, r, node)
	if err != nil {
		return nil, err
	}
	if _, err := exec.Privilege(); err != nil {
		_ = exec.Close()
		return nil, err
	}
	return exec, nil
}

// forNodes runs fn on nodes with at most limit at a time and tracks each
// node's status. It fails when any node fails.
func (r *run) forNodes(ctx context.Context, nodes []*model.ProvisionNode, limit int, action string, fn func(context.Context, Executor, *model.ProvisionNode) error) error {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	for _, node := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if err := r.onNode(ctx, node, action, fn); err != nil {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%s: %s", nodeLabel(node), r.redact.Redact(err.Error())))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d nodes failed: %s", len(failures), len(nodes), strings.Join(failures, "; "))
	}
	return nil
}

// onNode connects to one node and runs fn with status tracking.
func (r *run) onNode(ctx context.Context, node *model.ProvisionNode, action string, fn func(context.Context, Executor, *model.ProvisionNode) error) error {
	r.setNode(node, statusRunning, action, "")
	err := func() error {
		exec, err := r.connect(ctx, node)
		if err != nil {
			return err
		}
		defer func() { _ = exec.Close() }()
		return fn(ctx, exec, node)
	}()
	if err != nil {
		if ctx.Err() == nil {
			message := r.redact.Redact(err.Error())
			r.setNode(node, statusFailed, "", message)
			r.log(node, "Error: "+message)
		}
		return err
	}
	r.setNode(node, statusSucceeded, "", "")
	return nil
}

func (r *run) upload(ctx context.Context, exec Executor, files []nodeFile) error {
	for _, f := range files {
		if err := exec.Upload(ctx, f.Path, f.Data, f.Mode); err != nil {
			return err
		}
	}
	return nil
}

// runScript uploads a script and streams its output into the logs.
func (r *run) runScript(ctx context.Context, exec Executor, node *model.ProvisionNode, name string, script []byte, timeout time.Duration) error {
	p := scriptDir + "/" + name
	if err := exec.Upload(ctx, p, script, 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return exec.Run(ctx, "bash "+p, func(line string) { r.log(node, line) })
}

func runPrepareStep(ctx context.Context, r *run) error {
	c := r.clusterParams()
	return r.forNodes(ctx, r.nodes, maxParallelNodes, "Installing the container runtime and Kubernetes binaries",
		func(ctx context.Context, exec Executor, node *model.ProvisionNode) error {
			n, err := r.nodeParams(node)
			if err != nil {
				return err
			}
			files, err := prepareFiles(c, n)
			if err != nil {
				return err
			}
			// The marker goes first so a node is claimed before anything changes.
			marker := nodeFile{Path: ownerMarkerPath, Data: ownerMarker(r.prov.ID, r.prov.Name), Mode: 0o600}
			if err := r.upload(ctx, exec, append([]nodeFile{marker}, files...)); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, prepareTimeout)
			defer cancel()
			return exec.Run(ctx, "bash "+scriptDir+"/prepare.sh", func(line string) { r.log(node, line) })
		})
}

func runInitStep(ctx context.Context, r *run) error {
	first := r.firstControlPlane()
	return r.onNode(ctx, first, "Initializing the first control plane", func(ctx context.Context, exec Executor, node *model.ProvisionNode) error {
		c := r.clusterParams()
		n, err := r.nodeParams(node)
		if err != nil {
			return err
		}
		token, err := newBootstrapToken()
		if err != nil {
			return err
		}
		certKey := ""
		if c.Mode != ModeSingle {
			if certKey, err = newCertificateKey(); err != nil {
				return err
			}
		}
		r.redact.Add(token, tokenSecret(token), certKey)
		files, err := controlPlaneFiles(c, n)
		if err != nil {
			return err
		}
		initConfig, err := renderInitConfig(c, n, token, certKey)
		if err != nil {
			return err
		}
		patch, err := renderKubeletPatch(n)
		if err != nil {
			return err
		}
		files = append(files, nodeFile{Path: initConfigPath, Data: initConfig, Mode: 0o600}, nodeFile{Path: kubeletPatchPath, Data: patch, Mode: 0o644})
		if c.Mode == ModeKubeVIP {
			vip, err := renderKubeVIP(c, n, superAdminConfPath)
			if err != nil {
				return err
			}
			files = append(files, nodeFile{Path: kubeVIPInitPath, Data: vip, Mode: 0o600})
		}
		script, err := renderInitScript(c)
		if err != nil {
			return err
		}
		if err := r.upload(ctx, exec, files); err != nil {
			return err
		}
		if err := r.runScript(ctx, exec, node, "init.sh", script, kubeadmTimeout); err != nil {
			return err
		}
		now := time.Now()
		r.mu.Lock()
		r.token, r.tokenAt, r.certKey, r.certKeyAt = token, now, certKey, now
		r.mu.Unlock()
		return markJoined(node)
	})
}

// Control planes join one at a time to keep etcd quorum changes safe.
func runJoinControlPlanesStep(ctx context.Context, r *run) error {
	for _, node := range r.controlPlanes()[1:] {
		if node.Joined {
			continue
		}
		if err := r.onNode(ctx, node, "Joining as a control plane", r.join); err != nil {
			return fmt.Errorf("%s: %w", nodeLabel(node), err)
		}
	}
	return nil
}

func runJoinWorkersStep(ctx context.Context, r *run) error {
	var pending []*model.ProvisionNode
	for _, node := range r.workers() {
		if !node.Joined {
			pending = append(pending, node)
		}
	}
	return r.forNodes(ctx, pending, maxParallelWorkers, "Joining as a worker", r.join)
}

func (r *run) join(ctx context.Context, exec Executor, node *model.ProvisionNode) error {
	controlPlane := node.Role == RoleControlPlane
	token, caHash, certKey, err := r.joinCredentials(ctx, controlPlane)
	if err != nil {
		return fmt.Errorf("prepare join credentials: %w", err)
	}
	c := r.clusterParams()
	n, err := r.nodeParams(node)
	if err != nil {
		return err
	}
	var files []nodeFile
	if controlPlane {
		if files, err = controlPlaneFiles(c, n); err != nil {
			return err
		}
	}
	joinConfig, err := renderJoinConfig(c, n, token, caHash, certKey)
	if err != nil {
		return err
	}
	patch, err := renderKubeletPatch(n)
	if err != nil {
		return err
	}
	script, err := renderJoinScript(c, n)
	if err != nil {
		return err
	}
	files = append(files, nodeFile{Path: joinConfigPath, Data: joinConfig, Mode: 0o600}, nodeFile{Path: kubeletPatchPath, Data: patch, Mode: 0o644})
	if err := r.upload(ctx, exec, files); err != nil {
		return err
	}
	if err := r.runScript(ctx, exec, node, "join.sh", script, kubeadmTimeout); err != nil {
		return err
	}
	return markJoined(node)
}

// joinCredentials returns a bootstrap token, the CA hash and, for control
// planes, a certificate key, regenerating them on the first control plane
// when missing or close to their two-hour expiry.
func (r *run) joinCredentials(ctx context.Context, controlPlane bool) (token, caHash, certKey string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	needToken := r.token == "" || time.Since(r.tokenAt) > joinCredentialTTL
	needKey := controlPlane && (r.certKey == "" || time.Since(r.certKeyAt) > joinCredentialTTL)
	if needToken || needKey || r.caHash == "" {
		first := r.firstControlPlane()
		exec, err := r.connect(ctx, first)
		if err != nil {
			return "", "", "", fmt.Errorf("connect to %s: %w", nodeLabel(first), err)
		}
		defer func() { _ = exec.Close() }()
		cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		if r.caHash == "" {
			ca, err := exec.Output(cmdCtx, "cat "+caCertPath)
			if err != nil {
				return "", "", "", fmt.Errorf("read the cluster CA: %w", err)
			}
			if r.caHash, err = caCertHash(ca); err != nil {
				return "", "", "", err
			}
		}
		if needToken {
			if err := r.createToken(cmdCtx, exec); err != nil {
				return "", "", "", err
			}
		}
		if needKey {
			if err := r.uploadCerts(cmdCtx, exec, first); err != nil {
				return "", "", "", err
			}
		}
	}
	if controlPlane {
		certKey = r.certKey
	}
	return r.token, r.caHash, certKey, nil
}

func (r *run) createToken(ctx context.Context, exec Executor) error {
	out, err := exec.Output(ctx, "kubeadm token create --ttl 2h0m0s --kubeconfig "+adminConfPath)
	if err != nil {
		return fmt.Errorf("create a bootstrap token: %w", err)
	}
	token := bootstrapTokenPattern.FindString(string(out))
	if token == "" {
		return fmt.Errorf("kubeadm token create returned no token")
	}
	r.redact.Add(token, tokenSecret(token))
	r.token, r.tokenAt = token, time.Now()
	r.log(nil, "Created a new bootstrap token")
	return nil
}

// uploadCerts re-uploads the control plane certificates encrypted with a new
// key. The key goes in a config file, never on the command line.
func (r *run) uploadCerts(ctx context.Context, exec Executor, first *model.ProvisionNode) error {
	key, err := newCertificateKey()
	if err != nil {
		return err
	}
	r.redact.Add(key)
	n, err := r.nodeParams(first)
	if err != nil {
		return err
	}
	config, err := renderInitConfig(r.clusterParams(), n, r.token, key)
	if err != nil {
		return err
	}
	if err := exec.Upload(ctx, uploadCertsPath, config, 0o600); err != nil {
		return err
	}
	if _, err := exec.Output(ctx, "kubeadm init phase upload-certs --upload-certs --config "+uploadCertsPath); err != nil {
		return fmt.Errorf("upload control plane certificates: %w", err)
	}
	r.certKey, r.certKeyAt = key, time.Now()
	r.log(nil, "Uploaded control plane certificates with a new certificate key")
	return nil
}

func runInstallCNIStep(ctx context.Context, r *run) error {
	return r.onNode(ctx, r.firstControlPlane(), "Installing Calico", func(ctx context.Context, exec Executor, node *model.ProvisionNode) error {
		c := r.clusterParams()
		installation, err := renderCalicoInstallation(c)
		if err != nil {
			return err
		}
		script, err := renderCNIScript(c)
		if err != nil {
			return err
		}
		if err := exec.Upload(ctx, calicoInstallPath, installation, 0o644); err != nil {
			return err
		}
		return r.runScript(ctx, exec, node, "cni.sh", script, cniTimeout)
	})
}

// resetNodes runs kubeadm reset and cleanup, best-effort, on every node that
// carries this provision's ownership marker.
func resetNodes(ctx context.Context, r *run) {
	sem := make(chan struct{}, maxParallelNodes)
	var wg sync.WaitGroup
	for _, node := range r.nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := func() error {
				exec, err := r.connect(ctx, node)
				if err != nil {
					return err
				}
				defer func() { _ = exec.Close() }()
				ctx, cancel := context.WithTimeout(ctx, resetTimeout)
				defer cancel()
				marker, err := exec.Output(ctx, "cat "+ownerMarkerPath+" 2>/dev/null || true")
				if err != nil {
					return err
				}
				if strings.TrimSpace(string(marker)) != strings.TrimSpace(string(ownerMarker(r.prov.ID, r.prov.Name))) {
					r.log(node, "The node was not prepared by this installation; leaving it untouched")
					return nil
				}
				r.log(node, "Resetting the node")
				if err := exec.Upload(ctx, scriptDir+"/reset.sh", readAsset("reset.sh"), 0o700); err != nil {
					return err
				}
				// reset.sh checks the marker again before changing anything.
				cmd := fmt.Sprintf("bash %s/reset.sh delete %d %s", scriptDir, r.prov.ID, shellQuote(r.prov.Name))
				return exec.Run(ctx, cmd, func(line string) { r.log(node, line) })
			}()
			if err != nil {
				r.log(node, "Reset failed, continuing: "+r.redact.Redact(err.Error()))
				klog.Warningf("Cluster provision %d: reset of %s failed: %s", r.prov.ID, node.IP, r.redact.Redact(err.Error()))
			}
		}()
	}
	wg.Wait()
}

func markJoined(node *model.ProvisionNode) error {
	node.Joined = true
	return model.DB.Model(&model.ProvisionNode{}).Where("id = ?", node.ID).Update("joined", true).Error
}

func newBootstrapToken() (string, error) {
	id, err := randomString(6)
	if err != nil {
		return "", err
	}
	secret, err := randomString(16)
	if err != nil {
		return "", err
	}
	return id + "." + secret, nil
}

func tokenSecret(token string) string {
	if _, secret, ok := strings.Cut(token, "."); ok {
		return secret
	}
	return ""
}

func randomString(n int) (string, error) {
	out := make([]byte, n)
	limit := big.NewInt(int64(len(tokenAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = tokenAlphabet[v.Int64()]
	}
	return string(out), nil
}

func newCertificateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// caCertHash is the discovery hash kubeadm expects: sha256 of the CA's
// Subject Public Key Info.
func caCertHash(caPEM []byte) (string, error) {
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return "", fmt.Errorf("the cluster CA certificate is not PEM encoded")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse the cluster CA certificate: %w", err)
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
