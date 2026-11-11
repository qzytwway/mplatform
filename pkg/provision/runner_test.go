package provision

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeHosts records the commands the fake executors run and fails or blocks
// on demand. Keys are "<ip> <command>".
type fakeHosts struct {
	mu       sync.Mutex
	commands map[string]int
	failures map[string]int
	blocks   map[string]chan struct{}
	markers  map[string]string
}

func newFakeHosts() *fakeHosts {
	return &fakeHosts{commands: map[string]int{}, failures: map[string]int{}, blocks: map[string]chan struct{}{}, markers: map[string]string{}}
}

// matching lists recorded commands on ip that contain text.
func (h *fakeHosts) matching(ip, text string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for key := range h.commands {
		if strings.HasPrefix(key, ip+" ") && strings.Contains(key, text) {
			out = append(out, key)
		}
	}
	return out
}

func (h *fakeHosts) count(ip, command string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commands[ip+" "+command]
}

type fakeExecutor struct {
	hosts *fakeHosts
	ip    string
}

func (e *fakeExecutor) Run(ctx context.Context, cmd string, out func(string)) error {
	key := e.ip + " " + cmd
	e.hosts.mu.Lock()
	e.hosts.commands[key]++
	fail := e.hosts.failures[key] > 0
	if fail {
		e.hosts.failures[key]--
	}
	block := e.hosts.blocks[key]
	delete(e.hosts.blocks, key)
	e.hosts.mu.Unlock()
	out("running " + cmd)
	if block != nil {
		close(block)
		<-ctx.Done()
		return ctx.Err()
	}
	if fail {
		return fmt.Errorf("exit status 1: %s failed", cmd)
	}
	return nil
}

func (e *fakeExecutor) Output(ctx context.Context, cmd string) ([]byte, error) {
	if err := e.Run(ctx, cmd, func(string) {}); err != nil {
		return nil, err
	}
	e.hosts.mu.Lock()
	defer e.hosts.mu.Unlock()
	if strings.HasPrefix(cmd, "cat "+ownerMarkerPath) {
		return []byte(e.hosts.markers[e.ip]), nil
	}
	return nil, nil
}

func (e *fakeExecutor) Upload(_ context.Context, path string, _ []byte, _ uint32) error {
	e.hosts.mu.Lock()
	defer e.hosts.mu.Unlock()
	e.hosts.commands[e.ip+" upload "+path]++
	return nil
}

func (e *fakeExecutor) Privilege() (string, error) { return privilegeRoot, nil }
func (e *fakeExecutor) Close() error               { return nil }

// fakeStep runs one command named after the step on the nodes the real step
// would touch, through the same node tracking the real steps use.
func fakeStep(name string) stepFunc {
	return func(ctx context.Context, r *run) error {
		nodes := r.nodes
		switch name {
		case stepInitControlPlane, stepInstallCNI, stepWaitReady, stepRegister:
			nodes = r.controlPlanes()[:1]
		case stepJoinControlPlanes:
			nodes = r.controlPlanes()[1:]
		case stepJoinWorkers:
			nodes = r.workers()
		}
		return r.forNodes(ctx, nodes, maxParallelWorkers, name, func(ctx context.Context, exec Executor, node *model.ProvisionNode) error {
			return exec.Run(ctx, name, func(line string) { r.log(node, line) })
		})
	}
}

func setupRunnerTest(t *testing.T) (*Runner, *fakeHosts) {
	t.Helper()
	originalDB, originalKey := model.DB, common.MPlatformEncryptKey
	common.MPlatformEncryptKey = "provision-runner-test-key"
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Every connection to :memory: is a separate database.
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Cluster{}, &model.ClusterProvision{}, &model.ProvisionNode{}, &model.ProvisionLog{}); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	model.DB = db
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB, common.MPlatformEncryptKey = originalDB, originalKey
	})

	hosts := newFakeHosts()
	rn := newRunner()
	rn.heartbeat = 20 * time.Millisecond
	rn.steps = map[string]stepFunc{}
	for _, name := range stepOrder {
		rn.steps[name] = fakeStep(name)
	}
	rn.connect = func(_ context.Context, _ *run, node *model.ProvisionNode) (Executor, error) {
		return &fakeExecutor{hosts: hosts, ip: node.IP}, nil
	}
	return rn, hosts
}

func createTestProvision(t *testing.T, spec Spec) *model.ClusterProvision {
	t.Helper()
	spec.Normalize()
	if err := spec.Validate(); err != nil {
		t.Fatalf("invalid test spec: %v", err)
	}
	p, err := createProvisionRecord(spec, "v1.36.6", "admin")
	if err != nil {
		t.Fatalf("creating provision: %v", err)
	}
	return p
}

// runOnce lets the runner pick up pending work and waits for it to finish.
func runOnce(t *testing.T, rn *Runner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rn.scan(ctx)
	rn.wg.Wait()
}

func loadProvision(t *testing.T, id uint) (*model.ClusterProvision, map[string]stepState, []model.ProvisionNode) {
	t.Helper()
	p, err := findProvision(model.DB, id)
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]stepState{}
	for _, s := range decodeSteps(p) {
		steps[s.Name] = s
	}
	var nodes []model.ProvisionNode
	if err := model.DB.Where("provision_id = ?", id).Order("id").Find(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	return p, steps, nodes
}

func expectSteps(t *testing.T, steps map[string]stepState, want map[string]string) {
	t.Helper()
	for name, status := range want {
		if steps[name].Status != status {
			t.Errorf("step %s = %s (%s), want %s", name, steps[name].Status, steps[name].Error, status)
		}
	}
}

func TestRunnerRunsEveryStep(t *testing.T) {
	rn, hosts := setupRunnerTest(t)
	p := createTestProvision(t, validSpec(t))

	runOnce(t, rn)

	got, steps, nodes := loadProvision(t, p.ID)
	if got.Status != model.ProvisionStatusSucceeded || got.Error != "" {
		t.Fatalf("status = %s (%s), want succeeded", got.Status, got.Error)
	}
	if got.StartedAt == nil || got.FinishedAt == nil || got.LockedBy != "" || got.LockUntil != nil {
		t.Fatalf("timestamps or lease not finalized: %+v", got)
	}
	expectSteps(t, steps, map[string]string{
		stepPrecheck: statusSucceeded, stepPrepare: statusSucceeded, stepInitControlPlane: statusSucceeded,
		stepJoinControlPlanes: statusSkipped, stepJoinWorkers: statusSucceeded, stepInstallCNI: statusSucceeded,
		stepWaitReady: statusSucceeded, stepRegister: statusSucceeded,
	})
	for _, n := range nodes {
		if n.Status != statusSucceeded || n.CurrentAction != "" {
			t.Errorf("node %s = %s/%q, want succeeded", n.IP, n.Status, n.CurrentAction)
		}
	}
	if hosts.count("192.168.10.21", stepJoinWorkers) != 1 || hosts.count("192.168.10.11", stepInitControlPlane) != 1 {
		t.Fatalf("unexpected commands: %v", hosts.commands)
	}
	if hosts.count("192.168.10.21", stepInitControlPlane) != 0 {
		t.Fatalf("worker ran init: %v", hosts.commands)
	}
	var logs int64
	model.DB.Model(&model.ProvisionLog{}).Where("provision_id = ?", p.ID).Count(&logs)
	if logs == 0 {
		t.Fatalf("no logs written")
	}
}

func TestRunnerRetryResumesFromFailedStep(t *testing.T) {
	rn, hosts := setupRunnerTest(t)
	p := createTestProvision(t, validSpec(t))
	hosts.failures["192.168.10.11 "+stepInitControlPlane] = 1

	runOnce(t, rn)

	got, steps, nodes := loadProvision(t, p.ID)
	if got.Status != model.ProvisionStatusFailed || !strings.Contains(got.Error, stepInitControlPlane) {
		t.Fatalf("status = %s (%s), want failed at init", got.Status, got.Error)
	}
	expectSteps(t, steps, map[string]string{
		stepPrecheck: statusSucceeded, stepPrepare: statusSucceeded, stepInitControlPlane: statusFailed,
		stepJoinWorkers: statusPending, stepRegister: statusPending,
	})
	if !strings.Contains(steps[stepInitControlPlane].Error, "init-control-plane failed") || steps[stepInitControlPlane].FinishedAt == nil {
		t.Fatalf("failed step not recorded: %+v", steps[stepInitControlPlane])
	}
	if nodes[0].Status != statusFailed || nodes[0].Error == "" || nodes[1].Status != statusSucceeded {
		t.Fatalf("node statuses = %s/%s, want failed control plane and untouched worker", nodes[0].Status, nodes[1].Status)
	}
	if err := retryProvision(p.ID); err != nil {
		t.Fatalf("retry = %v", err)
	}
	if err := retryProvision(p.ID); !isConflict(err) {
		t.Fatalf("retrying a pending installation = %v, want conflict", err)
	}

	runOnce(t, rn)

	got, steps, _ = loadProvision(t, p.ID)
	if got.Status != model.ProvisionStatusSucceeded {
		t.Fatalf("status after retry = %s (%s)", got.Status, got.Error)
	}
	expectSteps(t, steps, map[string]string{stepInitControlPlane: statusSucceeded, stepRegister: statusSucceeded})
	for _, ip := range []string{"192.168.10.11", "192.168.10.21"} {
		if hosts.count(ip, stepPrecheck) != 1 || hosts.count(ip, stepPrepare) != 1 {
			t.Fatalf("finished steps were rerun on %s: %v", ip, hosts.commands)
		}
	}
	if hosts.count("192.168.10.11", stepInitControlPlane) != 2 {
		t.Fatalf("init ran %d times, want 2", hosts.count("192.168.10.11", stepInitControlPlane))
	}
	if err := retryProvision(p.ID); !isConflict(err) {
		t.Fatalf("retrying a succeeded installation = %v, want conflict", err)
	}
}

func TestRunnerCancel(t *testing.T) {
	for _, local := range []bool{true, false} {
		t.Run(fmt.Sprintf("local=%v", local), func(t *testing.T) {
			rn, hosts := setupRunnerTest(t)
			p := createTestProvision(t, validSpec(t))
			started := make(chan struct{})
			hosts.blocks["192.168.10.21 "+stepPrepare] = started

			ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			rn.scan(ctx)
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("prepare never started")
			}
			if err := cancelProvision(p.ID); err != nil {
				t.Fatal(err)
			}
			// Without the local fast path the heartbeat notices the cancelled status.
			if local {
				rn.cancelLocal(p.ID)
			}
			rn.wg.Wait()

			got, steps, nodes := loadProvision(t, p.ID)
			if got.Status != model.ProvisionStatusCancelled || got.LockedBy != "" {
				t.Fatalf("status = %s locked by %q, want cancelled and released", got.Status, got.LockedBy)
			}
			expectSteps(t, steps, map[string]string{stepPrecheck: statusSucceeded, stepPrepare: statusFailed, stepInitControlPlane: statusPending})
			if steps[stepPrepare].Error != cancelledMessage {
				t.Fatalf("prepare error = %q", steps[stepPrepare].Error)
			}
			for _, n := range nodes {
				if n.Status == statusRunning {
					t.Fatalf("node %s still running after cancel", n.IP)
				}
			}
			if err := cancelProvision(p.ID); !isConflict(err) {
				t.Fatalf("cancelling twice = %v, want conflict", err)
			}

			if err := retryProvision(p.ID); err != nil {
				t.Fatal(err)
			}
			runOnce(t, rn)
			got, _, _ = loadProvision(t, p.ID)
			if got.Status != model.ProvisionStatusSucceeded || hosts.count("192.168.10.11", stepPrecheck) != 1 {
				t.Fatalf("retry after cancel = %s, precheck runs = %d", got.Status, hosts.count("192.168.10.11", stepPrecheck))
			}
		})
	}
}

func TestRunnerRecoversInterruptedProvisions(t *testing.T) {
	rn, _ := setupRunnerTest(t)
	interrupted := createTestProvision(t, validSpec(t))
	live := validSpec(t)
	live.Name = "live"
	alive := createTestProvision(t, live)

	now := time.Now()
	steps := initialSteps(validSpec(t))
	steps[0].Status = statusSucceeded
	steps[1].Status = statusRunning
	for id, lockUntil := range map[uint]time.Time{interrupted.ID: now.Add(-time.Minute), alive.ID: now.Add(time.Minute)} {
		if err := model.DB.Model(&model.ClusterProvision{}).Where("id = ?", id).Updates(map[string]any{
			"status": model.ProvisionStatusRunning, "locked_by": "dead-instance", "lock_until": lockUntil, "steps": encodeSteps(steps),
		}).Error; err != nil {
			t.Fatal(err)
		}
		model.DB.Model(&model.ProvisionNode{}).Where("provision_id = ?", id).Updates(map[string]any{"status": statusRunning, "current_action": "Installing"})
	}

	rn.recoverInterrupted(now)

	got, gotSteps, nodes := loadProvision(t, interrupted.ID)
	if got.Status != model.ProvisionStatusFailed || got.Error != interruptedMessage || got.LockedBy != "" {
		t.Fatalf("interrupted provision = %s (%s) locked by %q", got.Status, got.Error, got.LockedBy)
	}
	expectSteps(t, gotSteps, map[string]string{stepPrecheck: statusSucceeded, stepPrepare: statusFailed, stepInitControlPlane: statusPending})
	if gotSteps[stepPrepare].Error != interruptedMessage {
		t.Fatalf("prepare error = %q", gotSteps[stepPrepare].Error)
	}
	for _, n := range nodes {
		if n.Status != statusFailed || n.Error != interruptedMessage {
			t.Fatalf("node %s = %s (%s), want failed", n.IP, n.Status, n.Error)
		}
	}
	if got, _, _ := loadProvision(t, alive.ID); got.Status != model.ProvisionStatusRunning {
		t.Fatalf("provision with a live lease = %s, want running", got.Status)
	}

	if err := retryProvision(interrupted.ID); err != nil {
		t.Fatal(err)
	}
	runOnce(t, rn)
	if got, _, _ := loadProvision(t, interrupted.ID); got.Status != model.ProvisionStatusSucceeded {
		t.Fatalf("retry after restart = %s (%s)", got.Status, got.Error)
	}
}

func expectDeleted(t *testing.T, id uint) {
	t.Helper()
	var provisions, nodes, logs int64
	model.DB.Model(&model.ClusterProvision{}).Where("id = ?", id).Count(&provisions)
	model.DB.Model(&model.ProvisionNode{}).Where("provision_id = ?", id).Count(&nodes)
	model.DB.Model(&model.ProvisionLog{}).Where("provision_id = ?", id).Count(&logs)
	if provisions+nodes+logs != 0 {
		t.Fatalf("records left after delete: provisions=%d nodes=%d logs=%d", provisions, nodes, logs)
	}
}

func TestDeleteWithResetSkipsNodesWhenPrecheckFailed(t *testing.T) {
	rn, hosts := setupRunnerTest(t)
	p := createTestProvision(t, validSpec(t))
	// The control plane already belongs to another cluster.
	hosts.failures["192.168.10.11 "+stepPrecheck] = 1
	runOnce(t, rn)
	if got, _, _ := loadProvision(t, p.ID); got.Status != model.ProvisionStatusFailed {
		t.Fatalf("status = %s, want failed", got.Status)
	}

	if err := deleteProvision(p.ID, true); err != nil {
		t.Fatal(err)
	}
	runOnce(t, rn)

	for _, ip := range []string{"192.168.10.11", "192.168.10.21"} {
		if touched := append(hosts.matching(ip, "reset"), hosts.matching(ip, ownerMarkerPath)...); len(touched) > 0 {
			t.Fatalf("delete touched %s although nothing was installed: %v", ip, touched)
		}
	}
	expectDeleted(t, p.ID)
}

func TestDeleteWithResetOnlyResetsOwnedNodes(t *testing.T) {
	rn, hosts := setupRunnerTest(t)
	p := createTestProvision(t, validSpec(t))
	runOnce(t, rn)
	hosts.markers["192.168.10.11"] = string(ownerMarker(p.ID, p.Name))
	hosts.markers["192.168.10.21"] = string(ownerMarker(p.ID+100, "other"))

	if err := deleteProvision(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := loadProvision(t, p.ID); got.Status != model.ProvisionStatusDeleting {
		t.Fatalf("status = %s, want deleting", got.Status)
	}
	runOnce(t, rn)

	wantReset := fmt.Sprintf("192.168.10.11 bash %s/reset.sh delete %d 'dev-01'", scriptDir, p.ID)
	if hosts.count("192.168.10.11", strings.TrimPrefix(wantReset, "192.168.10.11 ")) != 1 {
		t.Fatalf("owned node was not reset: %v", hosts.matching("192.168.10.11", "reset"))
	}
	if reset := hosts.matching("192.168.10.21", "reset"); len(reset) > 0 {
		t.Fatalf("node owned by another installation was reset: %v", reset)
	}
	expectDeleted(t, p.ID)
}
