package modelserving

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	apiKeyPrefix = "sk-"
	apiKeyBytes  = 24
)

type callerRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     *bool    `json:"enabled"`
	AllowAll    bool     `json:"allowAll"`
	Models      []string `json:"models"`

	TokensPerMinute int64 `json:"tokensPerMinute"`
	QuotaLimited    bool  `json:"quotaLimited"`
	// Quota is the initial quota of a new quota-limited caller.
	Quota int64 `json:"quota"`
}

type createdCaller struct {
	model.ModelCaller
	APIKey string `json:"apiKey"`
}

func newAPIKey() (string, error) {
	buf := make([]byte, apiKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	return apiKeyPrefix + hex.EncodeToString(buf), nil
}

// parseCallerModels validates and deduplicates the model names a caller may
// call.
func parseCallerModels(models []string) (model.SliceString, error) {
	parsed := model.SliceString{}
	seen := map[string]bool{}
	for _, name := range models {
		name = strings.TrimSpace(name)
		if len(name) > maxModelNameLength || !modelNamePattern.MatchString(name) {
			return nil, fmt.Errorf("models must contain valid model names")
		}
		if !seen[name] {
			seen[name] = true
			parsed = append(parsed, name)
		}
	}
	return parsed, nil
}

// validateCallerLimits checks the rate limit and initial quota of a request.
func validateCallerLimits(req *callerRequest) error {
	if req.TokensPerMinute < 0 || req.TokensPerMinute > maxQuotaTokens {
		return fmt.Errorf("tokensPerMinute must be between 0 and %d", maxQuotaTokens)
	}
	if req.Quota < 0 || req.Quota > maxQuotaTokens {
		return fmt.Errorf("quota must be between 0 and %d", maxQuotaTokens)
	}
	if req.Quota > 0 && !req.QuotaLimited {
		return fmt.Errorf("quota needs quotaLimited to be set")
	}
	return nil
}

// callerView hides the empty element an empty model list is stored as.
func callerView(caller model.ModelCaller) model.ModelCaller {
	models := model.SliceString{}
	for _, name := range caller.Models {
		if name != "" {
			models = append(models, name)
		}
	}
	caller.Models = models
	return caller
}

func (h *Handler) ListCallers(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var callers []model.ModelCaller
	if err := model.DB.Where("cluster_name = ?", cs.Name).Order("name").Find(&callers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]model.ModelCaller, 0, len(callers))
	for _, caller := range callers {
		items = append(items, callerView(caller))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) CreateCaller(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req callerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(validation.IsDNS1123Label(name)) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be lowercase letters, digits or '-', and at most 63 characters"})
		return
	}
	models, err := parseCallerModels(req.Models)
	if err == nil {
		err = validateCallerLimits(&req)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admin, ok := limitsAdmin(c, cs, &req)
	if !ok {
		return
	}
	var count int64
	if err := model.DB.Model(&model.ModelCaller{}).Where("cluster_name = ? AND name = ?", cs.Name, name).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("caller %s already exists", name)})
		return
	}
	key, err := newAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	caller := model.ModelCaller{
		ClusterName: cs.Name,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Enabled:     true,
		APIKey:      model.SecretString(key),
		AllowAll:    req.AllowAll,
		Models:      models,

		TokensPerMinute: req.TokensPerMinute,
		QuotaLimited:    req.QuotaLimited,
	}
	if caller.QuotaLimited && !startQuota(c, admin, caller.Name, req.Quota) {
		return
	}
	if err := model.DB.Create(&caller).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !syncGatewayIfInstalled(c, cs) {
		return
	}
	c.JSON(http.StatusCreated, createdCaller{ModelCaller: callerView(caller), APIKey: key})
}

func (h *Handler) UpdateCaller(c *gin.Context) {
	cs, caller, ok := h.loadCaller(c)
	if !ok {
		return
	}
	var req callerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
		return
	}
	models, err := parseCallerModels(req.Models)
	if err == nil {
		err = validateCallerLimits(&req)
	}
	if err == nil && req.Quota != 0 {
		err = fmt.Errorf("quota can only be set when creating a caller; adjust the quota instead")
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admin, ok := limitsAdmin(c, cs, &req)
	if !ok {
		return
	}
	// A caller that becomes quota-limited starts without quota, whatever it
	// held before.
	if req.QuotaLimited && !caller.QuotaLimited && !startQuota(c, admin, caller.Name, 0) {
		return
	}
	updated := *caller
	updated.Description = strings.TrimSpace(req.Description)
	updated.Enabled = *req.Enabled
	updated.AllowAll = req.AllowAll
	updated.Models = models
	updated.TokensPerMinute = req.TokensPerMinute
	updated.QuotaLimited = req.QuotaLimited
	if err := model.DB.Save(&updated).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !syncGatewayIfInstalled(c, cs) {
		return
	}
	c.JSON(http.StatusOK, callerView(updated))
}

func (h *Handler) GetCallerKey(c *gin.Context) {
	_, caller, ok := h.loadCaller(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"apiKey": string(caller.APIKey)})
}

func (h *Handler) ResetCallerKey(c *gin.Context) {
	cs, caller, ok := h.loadCaller(c)
	if !ok {
		return
	}
	key, err := newAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := model.DB.Model(caller).Update("api_key", model.SecretString(key)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !syncGatewayIfInstalled(c, cs) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"apiKey": key})
}

func (h *Handler) DeleteCaller(c *gin.Context) {
	cs, caller, ok := h.loadCaller(c)
	if !ok {
		return
	}
	if err := model.DB.Delete(caller).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !syncGatewayIfInstalled(c, cs) {
		return
	}
	// ai-quota cannot delete a quota; an empty one reads and acts the same,
	// and keeps a later caller of this name from inheriting it.
	admin, err := newQuotaAdmin(cs)
	if err == nil {
		err = admin.set(c.Request.Context(), caller.Name, 0)
	}
	if err != nil && !errors.Is(err, errQuotaUnavailable) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "the caller was deleted, but clearing its quota on the gateway failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "caller deleted"})
}

// loadCaller resolves the cluster and the caller named in the route, writing
// the error response when either is missing.
func (h *Handler) loadCaller(c *gin.Context) (*cluster.ClientSet, *model.ModelCaller, bool) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, nil, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid caller id"})
		return nil, nil, false
	}
	var caller model.ModelCaller
	err = model.DB.Where("cluster_name = ?", cs.Name).First(&caller, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "caller not found"})
		return nil, nil, false
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, nil, false
	}
	return cs, &caller, true
}
