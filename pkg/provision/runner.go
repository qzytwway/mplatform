package provision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	defaultLockDuration = 2 * time.Minute
	defaultHeartbeat    = 30 * time.Second
	defaultScanInterval = 5 * time.Second

	interruptedMessage = "Interrupted by a platform restart; retry is available"
	cancelledMessage   = "Cancelled by user"
)

// Step names, in execution order.
const (
	stepPrecheck          = "precheck"
	stepPrepare           = "prepare"
	stepInitControlPlane  = "init-control-plane"
	stepJoinControlPlanes = "join-control-planes"
	stepJoinWorkers       = "join-workers"
	stepInstallCNI        = "install-cni"
	stepWaitReady         = "wait-ready"
	stepRegister          = "register"
)

var stepOrder = []string{
	stepPrecheck, stepPrepare, stepInitControlPlane, stepJoinControlPlanes,
	stepJoinWorkers, stepInstallCNI, stepWaitReady, stepRegister,
}

// Step and node statuses.
const (
	statusPending   = "pending"
	statusRunning   = "running"
	statusSucceeded = "succeeded"
	statusFailed    = "failed"
	statusSkipped   = "skipped"
)

var (
	errCancelled = errors.New("cancelled by user")
	errLeaseLost = errors.New("another platform instance took over this installation")
)

type stepState struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	Error      string     `json:"error"`
}

// initialSteps marks the join steps skipped when there is nothing to join.
func initialSteps(spec Spec) []stepState {
	steps := make([]stepState, len(stepOrder))
	for i, name := range stepOrder {
		status := statusPending
		if (name == stepJoinControlPlanes && len(spec.ControlPlanes()) < 2) || (name == stepJoinWorkers && len(spec.Workers()) == 0) {
			status = statusSkipped
		}
		steps[i] = stepState{Name: name, Status: status}
	}
	return steps
}

func decodeSteps(p *model.ClusterProvision) []stepState {
	var steps []stepState
	if err := p.Steps.Unmarshal(&steps); err != nil || len(steps) != len(stepOrder) {
		var spec Spec
		_ = p.Spec.Unmarshal(&spec)
		return initialSteps(spec)
	}
	return steps
}

func encodeSteps(steps []stepState) model.JSONField {
	data, err := json.Marshal(steps)
	if err != nil {
		klog.Errorf("Failed to encode provision steps: %v", err)
	}
	return data
}

type stepFunc func(ctx context.Context, r *run) error

type connectFunc func(ctx context.Context, r *run, node *model.ProvisionNode) (Executor, error)

// Runner executes provisions. A DB lease (locked_by/lock_until) makes sure
// only one platform instance works on a provision at a time.
type Runner struct {
	instanceID   string
	lockDuration time.Duration
	heartbeat    time.Duration
	scanInterval time.Duration
	pollInterval time.Duration
	steps        map[string]stepFunc
	connect      connectFunc
	kubeClient   func(kubeconfig []byte) (kubernetes.Interface, error)

	wake   chan struct{}
	mu     sync.Mutex
	active map[uint]context.CancelCauseFunc
	wg     sync.WaitGroup
}

func newRunner() *Runner {
	hostname, _ := os.Hostname()
	return &Runner{
		instanceID:   fmt.Sprintf("%s-%d", hostname, os.Getpid()),
		lockDuration: defaultLockDuration,
		heartbeat:    defaultHeartbeat,
		scanInterval: defaultScanInterval,
		pollInterval: readyPollInterval,
		steps:        defaultSteps(),
		connect:      sshConnect,
		kubeClient:   newKubeClient,
		wake:         make(chan struct{}, 1),
		active:       map[uint]context.CancelCauseFunc{},
	}
}

func (rn *Runner) Start(ctx context.Context) {
	rn.recoverInterrupted(time.Now())
	go rn.loop(ctx)
}

// Wake makes the runner scan now instead of at the next tick.
func (rn *Runner) Wake() {
	select {
	case rn.wake <- struct{}{}:
	default:
	}
}

// cancelLocal stops a run of this instance right away; other instances
// notice the cancelled status at their next heartbeat.
func (rn *Runner) cancelLocal(id uint) {
	rn.mu.Lock()
	cancel := rn.active[id]
	rn.mu.Unlock()
	if cancel != nil {
		cancel(errCancelled)
	}
}

func (rn *Runner) loop(ctx context.Context) {
	ticker := time.NewTicker(rn.scanInterval)
	defer ticker.Stop()
	rn.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-rn.wake:
		}
		rn.scan(ctx)
	}
}

func (rn *Runner) scan(ctx context.Context) {
	now := time.Now()
	rn.recoverInterrupted(now)
	var rows []model.ClusterProvision
	if err := model.DB.Select("id", "status").
		Where("status IN ?", []string{model.ProvisionStatusPending, model.ProvisionStatusDeleting}).
		Where("lock_until IS NULL OR lock_until < ?", now).
		Find(&rows).Error; err != nil {
		klog.Errorf("Failed to list cluster provisions: %v", err)
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		next := model.ProvisionStatusRunning
		if row.Status == model.ProvisionStatusDeleting {
			next = model.ProvisionStatusDeleting
		}
		if !rn.acquire(row.ID, row.Status, next, now) {
			continue
		}
		rn.wg.Add(1)
		go func(id uint, deleting bool) {
			defer rn.wg.Done()
			if deleting {
				rn.executeDelete(ctx, id)
			} else {
				rn.execute(ctx, id)
			}
		}(row.ID, row.Status == model.ProvisionStatusDeleting)
	}
}

func (rn *Runner) acquire(id uint, from, to string, now time.Time) bool {
	updates := map[string]any{
		"status":     to,
		"locked_by":  rn.instanceID,
		"lock_until": now.Add(rn.lockDuration),
	}
	if to == model.ProvisionStatusRunning {
		updates["started_at"] = gorm.Expr("COALESCE(started_at, ?)", now)
		updates["finished_at"] = nil
	}
	result := model.DB.Model(&model.ClusterProvision{}).
		Where("id = ? AND status = ? AND (lock_until IS NULL OR lock_until < ?)", id, from, now).
		Updates(updates)
	if result.Error != nil {
		klog.Errorf("Failed to lock cluster provision %d: %v", id, result.Error)
		return false
	}
	return result.RowsAffected == 1
}

// recoverInterrupted fails running provisions whose runner died, i.e. that
// have no live lease. Pending provisions never hold a lease.
func (rn *Runner) recoverInterrupted(now time.Time) {
	var rows []model.ClusterProvision
	if err := model.DB.
		Where("status = ? AND (lock_until IS NULL OR lock_until < ?)", model.ProvisionStatusRunning, now).
		Find(&rows).Error; err != nil {
		klog.Errorf("Failed to list interrupted cluster provisions: %v", err)
		return
	}
	for i := range rows {
		p := &rows[i]
		steps := decodeSteps(p)
		for j := range steps {
			if steps[j].Status == statusRunning {
				steps[j].Status = statusFailed
				steps[j].Error = interruptedMessage
				steps[j].FinishedAt = &now
			}
		}
		result := model.DB.Model(&model.ClusterProvision{}).
			Where("id = ? AND status = ? AND (lock_until IS NULL OR lock_until < ?)", p.ID, p.Status, now).
			Updates(map[string]any{
				"status":      model.ProvisionStatusFailed,
				"error":       interruptedMessage,
				"steps":       encodeSteps(steps),
				"finished_at": now,
				"locked_by":   "",
				"lock_until":  nil,
			})
		if result.Error != nil || result.RowsAffected != 1 {
			continue
		}
		failRunningNodes(p.ID, interruptedMessage)
		klog.Warningf("Cluster provision %d (%s) was interrupted and marked failed", p.ID, p.Name)
	}
}

func failRunningNodes(provisionID uint, message string) {
	if err := model.DB.Model(&model.ProvisionNode{}).
		Where("provision_id = ? AND status = ?", provisionID, statusRunning).
		Updates(map[string]any{"status": statusFailed, "current_action": "", "error": message}).Error; err != nil {
		klog.Errorf("Failed to update nodes of cluster provision %d: %v", provisionID, err)
	}
}

// track registers the cancel function of a run and starts its heartbeat.
func (rn *Runner) track(ctx context.Context, id uint) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	rn.mu.Lock()
	rn.active[id] = cancel
	rn.mu.Unlock()
	done := make(chan struct{})
	go rn.keepLease(ctx, id, cancel, done)
	return ctx, func() {
		close(done)
		rn.mu.Lock()
		delete(rn.active, id)
		rn.mu.Unlock()
		cancel(nil)
	}
}

// keepLease renews the lease and stops the run when it was cancelled or
// taken over elsewhere.
func (rn *Runner) keepLease(ctx context.Context, id uint, cancel context.CancelCauseFunc, done <-chan struct{}) {
	ticker := time.NewTicker(rn.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		result := model.DB.Model(&model.ClusterProvision{}).
			Where("id = ? AND locked_by = ? AND status IN ?", id, rn.instanceID, []string{model.ProvisionStatusRunning, model.ProvisionStatusDeleting}).
			Update("lock_until", time.Now().Add(rn.lockDuration))
		if result.Error != nil {
			klog.Errorf("Failed to renew the lease of cluster provision %d: %v", id, result.Error)
			continue
		}
		if result.RowsAffected == 1 {
			continue
		}
		if currentStatus(id) == model.ProvisionStatusCancelled {
			cancel(errCancelled)
		} else {
			cancel(errLeaseLost)
		}
		return
	}
}

func currentStatus(id uint) string {
	var p model.ClusterProvision
	if err := model.DB.Select("status").First(&p, id).Error; err != nil {
		return ""
	}
	return p.Status
}

func (rn *Runner) execute(parent context.Context, id uint) {
	ctx, stop := rn.track(parent, id)
	defer stop()
	r, err := rn.loadRun(id)
	if err != nil {
		klog.Errorf("Failed to load cluster provision %d: %v", id, err)
		rn.finish(id, model.ProvisionStatusFailed, err.Error(), nil)
		return
	}
	defer r.logs.close()
	steps := decodeSteps(&r.prov)
	for i, name := range stepOrder {
		if steps[i].Status == statusSucceeded || steps[i].Status == statusSkipped {
			continue
		}
		if ctx.Err() == nil && currentStatus(id) == model.ProvisionStatusCancelled {
			rn.stopped(r, steps, i, errCancelled)
			return
		}
		if ctx.Err() != nil {
			rn.stopped(r, steps, i, context.Cause(ctx))
			return
		}
		started := time.Now()
		steps[i] = stepState{Name: name, Status: statusRunning, StartedAt: &started}
		r.step = name
		rn.saveSteps(id, steps, name)
		r.logs.write(nil, name, fmt.Sprintf("Step %s started", name))
		err := rn.steps[name](ctx, r)
		finished := time.Now()
		steps[i].FinishedAt = &finished
		if err != nil {
			if ctx.Err() != nil {
				err = context.Cause(ctx)
			}
			rn.stopped(r, steps, i, err)
			return
		}
		steps[i].Status = statusSucceeded
		rn.saveSteps(id, steps, name)
		r.logs.write(nil, name, fmt.Sprintf("Step %s succeeded in %s", name, finished.Sub(started).Round(time.Second)))
	}
	rn.finish(id, model.ProvisionStatusSucceeded, "", steps)
	r.logs.write(nil, stepRegister, "Cluster installed")
}

// stopped records why step i ended the run.
func (rn *Runner) stopped(r *run, steps []stepState, i int, err error) {
	now := time.Now()
	steps[i].Status = statusFailed
	if steps[i].FinishedAt == nil {
		steps[i].FinishedAt = &now
	}
	switch {
	case errors.Is(err, errLeaseLost):
		klog.Warningf("Cluster provision %d: %v", r.prov.ID, err)
	case errors.Is(err, errCancelled):
		steps[i].Error = cancelledMessage
		failRunningNodes(r.prov.ID, cancelledMessage)
		r.logs.write(nil, steps[i].Name, cancelledMessage)
		rn.finish(r.prov.ID, model.ProvisionStatusCancelled, cancelledMessage, steps)
	case errors.Is(err, context.Canceled):
		steps[i].Error = interruptedMessage
		failRunningNodes(r.prov.ID, interruptedMessage)
		rn.finish(r.prov.ID, model.ProvisionStatusFailed, interruptedMessage, steps)
	default:
		message := r.redact.Redact(err.Error())
		steps[i].Error = message
		failRunningNodes(r.prov.ID, message)
		r.logs.write(nil, steps[i].Name, "Step failed: "+message)
		rn.finish(r.prov.ID, model.ProvisionStatusFailed, fmt.Sprintf("%s: %s", steps[i].Name, message), steps)
	}
}

// installStarted reports whether precheck passed, after which prepare
// changes the nodes. Retry never resets a succeeded precheck, so this holds
// even when a later retry put prepare back to pending.
func installStarted(p *model.ClusterProvision) bool {
	for _, s := range decodeSteps(p) {
		if s.Name == stepPrecheck {
			return s.Status == statusSucceeded
		}
	}
	return false
}

func (rn *Runner) saveSteps(id uint, steps []stepState, current string) {
	if err := model.DB.Model(&model.ClusterProvision{}).
		Where("id = ? AND locked_by = ?", id, rn.instanceID).
		Updates(map[string]any{"steps": encodeSteps(steps), "current_step": current}).Error; err != nil {
		klog.Errorf("Failed to save steps of cluster provision %d: %v", id, err)
	}
}

// finish writes the final status and releases the lease. A cancelled
// provision keeps its status; only the step details are written.
func (rn *Runner) finish(id uint, status, message string, steps []stepState) {
	updates := map[string]any{
		"finished_at": time.Now(),
		"locked_by":   "",
		"lock_until":  nil,
	}
	if steps != nil {
		updates["steps"] = encodeSteps(steps)
	}
	query := model.DB.Model(&model.ClusterProvision{}).Where("id = ? AND locked_by = ?", id, rn.instanceID)
	if status != model.ProvisionStatusCancelled {
		updates["status"] = status
		updates["error"] = message
		query = query.Where("status = ?", model.ProvisionStatusRunning)
	}
	if err := query.Updates(updates).Error; err != nil {
		klog.Errorf("Failed to finish cluster provision %d: %v", id, err)
	}
	// A cancel that lands after the last step leaves the status update above
	// without effect; release the lease anyway so a retry is possible.
	if err := model.DB.Model(&model.ClusterProvision{}).Where("id = ? AND locked_by = ?", id, rn.instanceID).
		Updates(map[string]any{"locked_by": "", "lock_until": nil}).Error; err != nil {
		klog.Errorf("Failed to release cluster provision %d: %v", id, err)
	}
}

func (rn *Runner) loadRun(id uint) (*run, error) {
	var p model.ClusterProvision
	if err := model.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	var spec Spec
	if err := p.Spec.Unmarshal(&spec); err != nil {
		return nil, fmt.Errorf("invalid stored spec: %w", err)
	}
	var nodes []*model.ProvisionNode
	if err := model.DB.Where("provision_id = ?", id).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	redact := newRedactor()
	redact.Add(string(p.EncryptionKey))
	for _, n := range nodes {
		redact.Add(string(n.SSHPassword), string(n.SSHPrivateKey))
	}
	return &run{
		runner: rn,
		prov:   p,
		spec:   spec,
		nodes:  nodes,
		redact: redact,
		logs:   newLogSink(id, redact),
	}, nil
}

// executeDelete resets the nodes best-effort, then removes the cluster and
// all records of the provision. Nodes are only reset once precheck passed:
// before that nothing was installed, and a node that failed precheck may
// belong to another cluster.
func (rn *Runner) executeDelete(parent context.Context, id uint) {
	ctx, stop := rn.track(parent, id)
	defer stop()
	r, err := rn.loadRun(id)
	if err != nil {
		klog.Errorf("Failed to load cluster provision %d for deletion: %v", id, err)
		return
	}
	r.step = "delete"
	if installStarted(&r.prov) {
		resetNodes(ctx, r)
	} else {
		r.log(nil, "Nothing was installed on the nodes; skipping node reset")
	}
	r.logs.close()
	if ctx.Err() != nil {
		return
	}
	if err := deleteProvisionRecords(r.prov.ID, r.prov.ClusterID, rn.instanceID); err != nil {
		klog.Errorf("Failed to delete cluster provision %d: %v", id, err)
		return
	}
	klog.Infof("Cluster provision %d (%s) deleted", id, r.prov.Name)
}

// run is the state of one execution of a provision.
type run struct {
	runner *Runner
	prov   model.ClusterProvision
	spec   Spec
	nodes  []*model.ProvisionNode
	redact *redactor
	logs   *logSink
	step   string

	mu        sync.Mutex
	token     string
	tokenAt   time.Time
	certKey   string
	certKeyAt time.Time
	caHash    string
}

func (r *run) log(node *model.ProvisionNode, text string) {
	r.logs.write(node, r.step, text)
}

// setNode updates the node row and logs the action it starts.
func (r *run) setNode(node *model.ProvisionNode, status, action, message string) {
	node.Status, node.CurrentAction, node.Error = status, action, message
	if err := model.DB.Model(&model.ProvisionNode{}).Where("id = ?", node.ID).
		Updates(map[string]any{"status": status, "current_action": action, "error": message}).Error; err != nil {
		klog.Errorf("Failed to update provision node %d: %v", node.ID, err)
	}
	if action != "" {
		r.log(node, action)
	}
}

func (r *run) controlPlanes() []*model.ProvisionNode {
	return r.nodesWithRole(RoleControlPlane)
}

func (r *run) workers() []*model.ProvisionNode {
	return r.nodesWithRole(RoleWorker)
}

func (r *run) nodesWithRole(role string) []*model.ProvisionNode {
	var out []*model.ProvisionNode
	for _, n := range r.nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func (r *run) firstControlPlane() *model.ProvisionNode {
	return r.controlPlanes()[0]
}

// clusterParams uses node names detected by precheck for the certificate SANs.
func (r *run) clusterParams() clusterParams {
	var names []string
	for _, n := range r.controlPlanes() {
		names = append(names, n.Hostname)
	}
	return newClusterParams(r.spec, r.prov.KubernetesVersion, string(r.prov.EncryptionKey), names)
}

func (r *run) nodeParams(node *model.ProvisionNode) (nodeParams, error) {
	var facts nodeFacts
	if err := node.Facts.Unmarshal(&facts); err != nil || facts.Arch == "" {
		return nodeParams{}, fmt.Errorf("precheck facts of %s are missing; retry from precheck", node.IP)
	}
	return nodeParams{
		Name:      node.Hostname,
		IP:        node.IP,
		Role:      node.Role,
		Arch:      facts.Arch,
		OSFamily:  facts.OSFamily,
		Interface: facts.Interface,
		CPU:       facts.CPU,
		MemoryMiB: facts.MemoryMiB,
	}, nil
}
