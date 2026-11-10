package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	queryTimeout      = 30 * time.Second
	defaultQueryLimit = 200
	maxQueryLimit     = 1000
	histogramBuckets  = 60
	maxLogLineBytes   = 1 << 20
	maxErrorBodyBytes = 1024
)

var (
	transportMu     sync.Mutex
	cachedTransport struct {
		cs  *cluster.ClientSet
		url string
		rt  http.RoundTripper
	}
)

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	g := group.Group("/logging")
	g.GET("/status", h.QueryStatus)
	g.GET("/query", h.Query)
	g.GET("/hits", h.Hits)
}

type logQuery struct {
	query        string
	extraFilters string
	start        time.Time
	end          time.Time
}

func (h *Handler) QueryStatus(c *gin.Context) {
	_, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configured": true})
}

func (h *Handler) Query(c *gin.Context) {
	q, ok := parseLogQuery(c)
	if !ok {
		return
	}
	if q == nil {
		c.JSON(http.StatusOK, gin.H{"items": []map[string]string{}})
		return
	}
	limit := defaultQueryLimit
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		limit = min(parsed, maxQueryLimit)
	}
	params := q.params()
	params.Set("query", fmt.Sprintf("%s | sort by (_time desc) | limit %d", q.query, limit))

	resp, err := h.victoriaLogsGet(c.Request.Context(), "/select/logsql/query", params)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	items := []map[string]string{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item map[string]string
		if err := json.Unmarshal(line, &item); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to decode log entry: %v", err)})
			return
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to read logs: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) Hits(c *gin.Context) {
	q, ok := parseLogQuery(c)
	if !ok {
		return
	}
	if q == nil {
		c.JSON(http.StatusOK, gin.H{"timestamps": []string{}, "values": []int64{}, "total": 0})
		return
	}
	step := histogramStep(q.start, q.end)
	params := q.params()
	params.Set("query", q.query)
	params.Set("step", fmt.Sprintf("%ds", step))

	resp, err := h.victoriaLogsGet(c.Request.Context(), "/select/logsql/hits", params)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Hits []struct {
			Timestamps []string `json:"timestamps"`
			Values     []int64  `json:"values"`
			Total      int64    `json:"total"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to decode hits: %v", err)})
		return
	}
	response := gin.H{"timestamps": []string{}, "values": []int64{}, "total": 0, "step": step}
	if len(result.Hits) > 0 {
		hits := result.Hits[0]
		response = gin.H{"timestamps": hits.Timestamps, "values": hits.Values, "total": hits.Total, "step": step}
	}
	c.JSON(http.StatusOK, response)
}

// parseLogQuery validates the request and builds the query with the caller's
// RBAC scope in extra_filters, which VictoriaLogs enforces on every subquery.
// It writes the error response itself; a nil query with ok means the caller
// may not read logs in any namespace.
func parseLogQuery(c *gin.Context) (q *logQuery, ok bool) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be an RFC3339 time"})
		return nil, false
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil || !end.After(start) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end must be an RFC3339 time after start"})
		return nil, false
	}

	namespace := strings.TrimSpace(c.Query("namespace"))
	filters := map[string]any{clusterField: cs.Name}
	switch {
	case namespace != "" && namespace != common.AllNamespaces:
		if !rbac.CanAccess(user, string(common.Pods), string(common.VerbLog), cs.Name, namespace) {
			c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(common.VerbLog), string(common.Pods), namespace, cs.Name)})
			return nil, false
		}
		filters[namespaceField] = []string{namespace}
	case !rbac.CanAccessAllNamespaces(user, string(common.Pods), string(common.VerbLog), cs.Name):
		namespaces, err := readableNamespaces(c.Request.Context(), cs, user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return nil, false
		}
		if len(namespaces) == 0 {
			return nil, true
		}
		filters[namespaceField] = namespaces
	}
	if container := strings.TrimSpace(c.Query("container")); container != "" {
		filters[containerField] = container
	}
	extraFilters, err := json.Marshal(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}

	return &logQuery{
		query:        buildLogsQL(strings.TrimSpace(c.Query("q")), strings.TrimSpace(c.Query("pod"))),
		extraFilters: string(extraFilters),
		start:        start,
		end:          end,
	}, true
}

func readableNamespaces(ctx context.Context, cs *cluster.ClientSet, user model.User) ([]string, error) {
	list, err := cs.K8sClient.ClientSet.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	namespaces := []string{}
	for _, namespace := range list.Items {
		if rbac.CanAccess(user, string(common.Pods), string(common.VerbLog), cs.Name, namespace.Name) {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	return namespaces, nil
}

// buildLogsQL turns the keyword and pod prefix into quoted LogsQL filters, so
// user input can never add pipes, operators or other filters.
func buildLogsQL(keyword, podPrefix string) string {
	var filters []string
	if podPrefix != "" {
		filters = append(filters, podField+":="+strconv.Quote(podPrefix)+"*")
	}
	if keyword != "" {
		filters = append(filters, strconv.Quote(keyword))
	}
	if len(filters) == 0 {
		return "*"
	}
	return strings.Join(filters, " ")
}

func histogramStep(start, end time.Time) int64 {
	return max(int64(end.Sub(start).Seconds())/histogramBuckets, 1)
}

func (q *logQuery) params() url.Values {
	return url.Values{
		"start":         {q.start.Format(time.RFC3339)},
		"end":           {q.end.Format(time.RFC3339)},
		"extra_filters": {q.extraFilters},
	}
}

func (h *Handler) victoriaLogsGet(ctx context.Context, path string, params url.Values) (*http.Response, error) {
	storage, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("log storage is not installed")
	} else if err != nil {
		return nil, err
	}
	cs, err := h.cm.GetClientSet(storage.ClusterName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, storage.WriteURL+path+"?"+params.Encode(), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	// The Kubernetes API proxy strips Authorization, so the platform queries
	// the NodePort address, through the cluster agent tunnel when needed.
	req.SetBasicAuth(storage.Username, string(storage.Password))
	resp, err := (&http.Client{Transport: storageTransport(cs, storage.WriteURL)}).Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("victorialogs request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("victorialogs request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// storageTransport reuses the transport while the storage cluster's client set
// is unchanged, so agent tunnels keep their connection pool.
func storageTransport(cs *cluster.ClientSet, writeURL string) http.RoundTripper {
	transportMu.Lock()
	defer transportMu.Unlock()
	if cachedTransport.cs != cs || cachedTransport.url != writeURL {
		cachedTransport.cs = cs
		cachedTransport.url = writeURL
		cachedTransport.rt = cluster.ServiceTransport(cs, "VictoriaLogs", writeURL)
	}
	return cachedTransport.rt
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}
