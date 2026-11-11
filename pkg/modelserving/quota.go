package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
)

const (
	gatewayService = "higress-gateway"

	// ai-quota rejects every caller without a positive quota, so callers
	// that are not quota-limited hold unlimitedQuota while it is enforced.
	unlimitedQuota int64 = 1_000_000_000_000_000_000
	// maxQuotaTokens bounds what an administrator grants at once, far below
	// unlimitedQuota.
	maxQuotaTokens int64 = 1_000_000_000_000_000

	quotaAdminTimeout = 10 * time.Second
	// quotaSyncTimeout bounds the quota part of a sync, which makes a call
	// per caller that is not quota-limited.
	quotaSyncTimeout = 30 * time.Second
)

var errQuotaUnavailable = errors.New("install or upgrade the gateway first: it cannot enforce rate limits or quotas yet")

type quotaRequest struct {
	Action string `json:"action"`
	Tokens int64  `json:"tokens"`
}

type callerQuota struct {
	ID        uint  `json:"id"`
	Remaining int64 `json:"remaining"`
}

// quotaAdmin calls the administration API of the gateway's ai-quota plugin
// through the API server's service proxy.
type quotaAdmin struct {
	cs  *cluster.ClientSet
	key model.SecretString
}

// newQuotaAdmin fails with errQuotaUnavailable when the cluster's gateway
// cannot hold quotas.
func newQuotaAdmin(cs *cluster.ClientSet) (*quotaAdmin, error) {
	gateway, err := loadGateway(cs.Name)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && gateway.QuotaAdminKey == "") {
		return nil, errQuotaUnavailable
	} else if err != nil {
		return nil, err
	}
	return &quotaAdmin{cs: cs, key: gateway.QuotaAdminKey}, nil
}

func (a *quotaAdmin) request(ctx context.Context, verb, suffix string, query, form url.Values) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, quotaAdminTimeout)
	defer cancel()
	req := a.cs.K8sClient.ClientSet.CoreV1().RESTClient().Verb(verb).
		Namespace(gatewayNamespace).
		Resource("services").
		Name(utilnet.JoinSchemeNamePort("http", gatewayService, "80")).
		SubResource("proxy").
		Suffix(quotaAdminPrefix + quotaAdminPath + suffix)
	for name, values := range query {
		req = req.Param(name, values[0])
	}
	if form != nil {
		req = req.Body([]byte(form.Encode())).SetHeader("Content-Type", "application/x-www-form-urlencoded")
	}
	body, err := req.SetHeader(quotaAdminHeader, string(a.key)).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("the gateway's quota administration failed: %w", err)
	}
	return body, nil
}

func (a *quotaAdmin) remaining(ctx context.Context, caller string) (int64, error) {
	body, err := a.request(ctx, http.MethodGet, "", url.Values{"consumer": {caller}}, nil)
	if err != nil {
		return 0, err
	}
	var result struct {
		Quota int64 `json:"quota"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unexpected reply from the gateway's quota administration: %w", err)
	}
	return result.Quota, nil
}

func (a *quotaAdmin) set(ctx context.Context, caller string, tokens int64) error {
	_, err := a.request(ctx, http.MethodPost, "/refresh", nil, url.Values{"consumer": {caller}, "quota": {strconv.FormatInt(tokens, 10)}})
	return err
}

func (a *quotaAdmin) add(ctx context.Context, caller string, tokens int64) error {
	_, err := a.request(ctx, http.MethodPost, "/delta", nil, url.Values{"consumer": {caller}, "value": {strconv.FormatInt(tokens, 10)}})
	return err
}

// syncQuotas turns quota enforcement on when a caller is quota-limited,
// after giving every other caller the quota that exempts it. apply applies
// the ai-quota plugin with or without enforcement.
func syncQuotas(ctx context.Context, cs *cluster.ClientSet, gateway *model.ModelGateway, callers []model.ModelCaller, apply func(enforced bool) error) error {
	ctx, cancel := context.WithTimeout(ctx, quotaSyncTimeout)
	defer cancel()
	enforced := false
	for _, caller := range callers {
		enforced = enforced || caller.QuotaLimited
	}
	if !enforced {
		return apply(false)
	}
	// The plugin has to serve its administration API before it enforces.
	plugin := &unstructured.Unstructured{}
	plugin.SetGroupVersionKind(wasmPluginGVK)
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: gatewayNamespace, Name: pluginName(quotaPlugin)}, plugin)
	if apierrors.IsNotFound(err) {
		err = apply(false)
	}
	if err != nil {
		return err
	}
	admin := &quotaAdmin{cs: cs, key: gateway.QuotaAdminKey}
	for _, caller := range callers {
		if caller.QuotaLimited {
			continue
		}
		if err := admin.set(ctx, caller.Name, unlimitedQuota); err != nil {
			return fmt.Errorf("failed to exempt caller %s from quotas: %w", caller.Name, err)
		}
	}
	return apply(true)
}

// limitsAdmin returns the gateway's quota administration for a request that
// limits a caller, and nil for one that does not. It writes the error
// response and returns false when the gateway cannot enforce the limits.
func limitsAdmin(c *gin.Context, cs *cluster.ClientSet, req *callerRequest) (*quotaAdmin, bool) {
	if req.TokensPerMinute == 0 && !req.QuotaLimited {
		return nil, true
	}
	admin, err := newQuotaAdmin(cs)
	if errors.Is(err, errQuotaUnavailable) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return admin, true
}

// startQuota gives a caller its first quota before it is saved as
// quota-limited, so the gateway never sees it limited but without a quota of
// its own. It writes the error response and returns false on failure.
func startQuota(c *gin.Context, admin *quotaAdmin, caller string, tokens int64) bool {
	if err := admin.set(c.Request.Context(), caller, tokens); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "nothing was saved, because setting the caller's quota failed: " + err.Error()})
		return false
	}
	return true
}

func (h *Handler) ListCallerQuotas(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var callers []model.ModelCaller
	if err := model.DB.Where("cluster_name = ? AND quota_limited = ?", cs.Name, true).Order("name").Find(&callers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]callerQuota, 0, len(callers))
	admin, err := newQuotaAdmin(cs)
	for i := 0; err == nil && i < len(callers); i++ {
		var remaining int64
		remaining, err = admin.remaining(c.Request.Context(), callers[i].Name)
		items = append(items, callerQuota{ID: callers[i].ID, Remaining: remaining})
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []callerQuota{}, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) AdjustCallerQuota(c *gin.Context) {
	cs, caller, ok := h.loadCaller(c)
	if !ok {
		return
	}
	var req quotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Action != "set" && req.Action != "add" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be set or add"})
		return
	}
	if req.Tokens < 0 || req.Tokens > maxQuotaTokens {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("tokens must be between 0 and %d", maxQuotaTokens)})
		return
	}
	if !caller.QuotaLimited {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the caller is not quota-limited"})
		return
	}
	admin, err := newQuotaAdmin(cs)
	if errors.Is(err, errQuotaUnavailable) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	change := admin.set
	if req.Action == "add" {
		change = admin.add
	}
	err = change(ctx, caller.Name, req.Tokens)
	var remaining int64
	if err == nil {
		remaining, err = admin.remaining(ctx, caller.Name)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"remaining": remaining})
}
