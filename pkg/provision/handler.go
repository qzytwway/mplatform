package provision

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/klog/v2"
)

const (
	defaultLogLimit         = 500
	maxLogLimit             = 1000
	standalonePrecheckLimit = 3 * time.Minute
)

var defaultRunner *Runner

// Start runs pending provisions in the background and fails the ones a
// previous platform process left running.
func Start(ctx context.Context) {
	defaultRunner = newRunner()
	defaultRunner.Start(ctx)
}

func wakeRunner() {
	if defaultRunner != nil {
		defaultRunner.Wake()
	}
}

func RegisterAdminRoutes(group *gin.RouterGroup) {
	g := group.Group("/provisions")
	g.GET("/versions", getVersions)
	g.POST("/precheck", requireUnmanaged, precheck)
	g.GET("", listProvisions)
	g.POST("", requireUnmanaged, createProvision)
	g.GET("/:id", getProvision)
	g.GET("/:id/logs", getProvisionLogs)
	g.POST("/:id/retry", requireUnmanaged, retry)
	g.POST("/:id/cancel", requireUnmanaged, cancel)
	g.DELETE("/:id", requireUnmanaged, remove)
}

// requireUnmanaged rejects writes while clusters come from the config file.
func requireUnmanaged(c *gin.Context) {
	if common.IsSectionManaged("clusters") {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": common.ManagedSectionError})
		return
	}
	c.Next()
}

func getVersions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"default": DefaultMinor, "versions": listVersions(c.Request.Context())})
}

func bindSpec(c *gin.Context) (Spec, bool) {
	var spec Spec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return spec, false
	}
	spec.Normalize()
	if err := spec.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return spec, false
	}
	return spec, true
}

func precheck(c *gin.Context) {
	spec, ok := bindSpec(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), standalonePrecheckLimit)
	defer cancel()
	version := versions.resolveForInstall(ctx, spec.KubernetesMinor)
	passed, nodes := standalonePrecheck(ctx, spec, version)
	c.JSON(http.StatusOK, gin.H{"passed": passed, "nodes": nodes})
}

func createProvision(c *gin.Context) {
	spec, ok := bindSpec(c)
	if !ok {
		return
	}
	user := c.MustGet("user").(model.User)
	version := versions.resolveForInstall(c.Request.Context(), spec.KubernetesMinor)
	p, err := createProvisionRecord(spec, version, user.Username)
	if err != nil {
		writeError(c, err)
		return
	}
	klog.Infof("Cluster provision %d (%s) created by %s: Kubernetes %s, %s, %d nodes", p.ID, p.Name, user.Username, version, spec.ControlPlane.Mode, len(spec.Nodes))
	wakeRunner()
	c.JSON(http.StatusCreated, gin.H{"id": p.ID})
}

type provisionSummary struct {
	ID                uint       `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	KubernetesMinor   string     `json:"kubernetesMinor"`
	KubernetesVersion string     `json:"kubernetesVersion"`
	ControlPlaneMode  string     `json:"controlPlaneMode"`
	Status            string     `json:"status"`
	CurrentStep       string     `json:"currentStep"`
	Error             string     `json:"error"`
	ControlPlaneCount int        `json:"controlPlaneCount"`
	WorkerCount       int        `json:"workerCount"`
	ClusterID         *uint      `json:"clusterId"`
	CreatedBy         string     `json:"createdBy"`
	CreatedAt         time.Time  `json:"createdAt"`
	StartedAt         *time.Time `json:"startedAt"`
	FinishedAt        *time.Time `json:"finishedAt"`
}

type provisionDetail struct {
	provisionSummary
	Spec  Spec        `json:"spec"`
	Steps []stepState `json:"steps"`
	Nodes []nodeView  `json:"nodes"`
}

type nodeView struct {
	ID            uint          `json:"id"`
	IP            string        `json:"ip"`
	Hostname      string        `json:"hostname"`
	Role          string        `json:"role"`
	Status        string        `json:"status"`
	CurrentAction string        `json:"currentAction"`
	Error         string        `json:"error"`
	Precheck      *NodePrecheck `json:"precheck"`
}

type logLine struct {
	ID     uint      `json:"id"`
	NodeID *uint     `json:"nodeId"`
	Node   string    `json:"node"`
	Step   string    `json:"step"`
	Time   time.Time `json:"time"`
	Text   string    `json:"text"`
}

func summarize(p *model.ClusterProvision) (provisionSummary, Spec) {
	var spec Spec
	_ = p.Spec.Unmarshal(&spec)
	if spec.Sources.RegistryMirrors == nil {
		spec.Sources.RegistryMirrors = []RegistryMirror{}
	}
	return provisionSummary{
		ID:                p.ID,
		Name:              p.Name,
		Description:       p.Description,
		KubernetesMinor:   p.KubernetesMinor,
		KubernetesVersion: p.KubernetesVersion,
		ControlPlaneMode:  p.ControlPlaneMode,
		Status:            p.Status,
		CurrentStep:       p.CurrentStep,
		Error:             p.Error,
		ControlPlaneCount: len(spec.ControlPlanes()),
		WorkerCount:       len(spec.Workers()),
		ClusterID:         p.ClusterID,
		CreatedBy:         p.CreatedBy,
		CreatedAt:         p.CreatedAt,
		StartedAt:         p.StartedAt,
		FinishedAt:        p.FinishedAt,
	}, spec
}

func listProvisions(c *gin.Context) {
	var rows []model.ClusterProvision
	if err := model.DB.Order("id DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]provisionSummary, len(rows))
	for i := range rows {
		result[i], _ = summarize(&rows[i])
	}
	c.JSON(http.StatusOK, result)
}

func getProvision(c *gin.Context) {
	id, ok := provisionID(c)
	if !ok {
		return
	}
	p, err := findProvision(model.DB, id)
	if err != nil {
		writeError(c, err)
		return
	}
	var nodes []model.ProvisionNode
	if err := model.DB.Where("provision_id = ?", id).Order("id").Find(&nodes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	summary, spec := summarize(p)
	detail := provisionDetail{provisionSummary: summary, Spec: spec, Steps: decodeSteps(p), Nodes: make([]nodeView, len(nodes))}
	for i, n := range nodes {
		view := nodeView{ID: n.ID, IP: n.IP, Hostname: n.Hostname, Role: n.Role, Status: n.Status, CurrentAction: n.CurrentAction, Error: n.Error}
		var result NodePrecheck
		if len(n.Precheck) > 0 && n.Precheck.Unmarshal(&result) == nil {
			view.Precheck = &result
		}
		detail.Nodes[i] = view
	}
	c.JSON(http.StatusOK, detail)
}

func getProvisionLogs(c *gin.Context) {
	id, ok := provisionID(c)
	if !ok {
		return
	}
	after, _ := strconv.ParseUint(c.Query("after"), 10, 64)
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLogLimit)))
	if err != nil || limit < 1 {
		limit = defaultLogLimit
	}
	limit = min(limit, maxLogLimit)
	query := model.DB.Where("provision_id = ? AND id > ?", id, after)
	if node := c.Query("node"); node != "" {
		nodeID, err := strconv.ParseUint(node, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
			return
		}
		query = query.Where("node_id = ?", nodeID)
	}
	var rows []model.ProvisionLog
	if err := query.Order("id").Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	lines := make([]logLine, len(rows))
	next := uint(after)
	for i, row := range rows {
		lines[i] = logLine{ID: row.ID, NodeID: row.NodeID, Node: row.Node, Step: row.Step, Time: row.CreatedAt, Text: row.Text}
		next = row.ID
	}
	c.JSON(http.StatusOK, gin.H{"lines": lines, "next": next})
}

func retry(c *gin.Context) {
	id, ok := provisionID(c)
	if !ok {
		return
	}
	if err := retryProvision(id); err != nil {
		writeError(c, err)
		return
	}
	wakeRunner()
	c.JSON(http.StatusAccepted, gin.H{"message": "retry scheduled"})
}

func cancel(c *gin.Context) {
	id, ok := provisionID(c)
	if !ok {
		return
	}
	if err := cancelProvision(id); err != nil {
		writeError(c, err)
		return
	}
	if defaultRunner != nil {
		defaultRunner.cancelLocal(id)
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "cancellation requested"})
}

func remove(c *gin.Context) {
	id, ok := provisionID(c)
	if !ok {
		return
	}
	reset := c.Query("reset") == "true"
	if err := deleteProvision(id, reset); err != nil {
		writeError(c, err)
		return
	}
	if reset {
		wakeRunner()
		c.JSON(http.StatusAccepted, gin.H{"message": "deletion scheduled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "installation deleted"})
}

func provisionID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid installation id"})
		return 0, false
	}
	return uint(id), true
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case isConflict(err):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, errDefaultCluster):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
