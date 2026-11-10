package metrics

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prommodel "github.com/prometheus/common/model"
	"github.com/qzytwway/mplatform/pkg/alertmanager"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
)

const alertmanagerResource = "alertmanager"

var silenceIDPattern = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type alertInstance struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    time.Time         `json:"activeAt"`
	Value       string            `json:"value"`
}

type alertRule struct {
	Group       string            `json:"group"`
	File        string            `json:"file"`
	Name        string            `json:"name"`
	Query       string            `json:"query"`
	Duration    float64           `json:"duration"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	Health      string            `json:"health"`
	LastError   string            `json:"lastError,omitempty"`
	Alerts      []alertInstance   `json:"alerts"`
}

func labelSetToMap(labels prommodel.LabelSet) map[string]string {
	result := make(map[string]string, len(labels))
	for name, value := range labels {
		result[string(name)] = string(value)
	}
	return result
}

// GetAlertRules returns the alerting rules loaded in Prometheus together with
// their pending and firing alerts.
func (h *Handler) GetAlertRules(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	// Alerts span every namespace, so require the same cluster-wide grant as PromQL.
	if !rbac.CanAccessAllNamespaces(user, "prometheus", string(common.VerbGet), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), "get", "prometheus", common.AllNamespaces, cs.Name)})
		return
	}
	if cs.PromClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Prometheus client not available"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	result, err := cs.PromClient.Rules(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	rules := []alertRule{}
	for _, group := range result.Groups {
		for _, r := range group.Rules {
			rule, ok := r.(v1.AlertingRule)
			if !ok {
				continue
			}
			alerts := make([]alertInstance, 0, len(rule.Alerts))
			for _, alert := range rule.Alerts {
				alerts = append(alerts, alertInstance{
					Labels:      labelSetToMap(alert.Labels),
					Annotations: labelSetToMap(alert.Annotations),
					State:       string(alert.State),
					ActiveAt:    alert.ActiveAt,
					Value:       alert.Value,
				})
			}
			rules = append(rules, alertRule{
				Group:       group.Name,
				File:        group.File,
				Name:        rule.Name,
				Query:       rule.Query,
				Duration:    rule.Duration,
				Labels:      labelSetToMap(rule.Labels),
				Annotations: labelSetToMap(rule.Annotations),
				State:       rule.State,
				Health:      string(rule.Health),
				LastError:   rule.LastError,
				Alerts:      alerts,
			})
		}
	}
	c.JSON(http.StatusOK, rules)
}

// alertmanagerClient enforces the cluster-wide alertmanager grant for verb and
// writes the error response itself when the request cannot proceed.
func alertmanagerClient(c *gin.Context, verb common.Verb) (*alertmanager.Client, bool) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if !rbac.CanAccessAllNamespaces(user, alertmanagerResource, string(verb), cs.Name) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(verb), alertmanagerResource, common.AllNamespaces, cs.Name)})
		return nil, false
	}
	if cs.AlertmanagerClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Alertmanager is not configured for this cluster"})
		return nil, false
	}
	return cs.AlertmanagerClient, true
}

func (h *Handler) ListSilences(c *gin.Context) {
	client, ok := alertmanagerClient(c, common.VerbGet)
	if !ok {
		return
	}
	silences, err := client.ListSilences(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if silences == nil {
		silences = []alertmanager.Silence{}
	}
	c.JSON(http.StatusOK, silences)
}

type createSilenceRequest struct {
	Matchers []alertmanager.Matcher `json:"matchers"`
	StartsAt time.Time              `json:"startsAt"`
	EndsAt   time.Time              `json:"endsAt"`
	Comment  string                 `json:"comment"`
}

func (r *createSilenceRequest) validate(now time.Time) error {
	if len(r.Matchers) == 0 {
		return errors.New("at least one matcher is required")
	}
	for i, matcher := range r.Matchers {
		if matcher.Name == "" {
			return errors.New("matcher name is required")
		}
		// Alertmanager treats a missing isEqual as an equality matcher.
		if matcher.IsEqual == nil {
			isEqual := true
			r.Matchers[i].IsEqual = &isEqual
		}
	}
	if r.Comment == "" {
		return errors.New("comment is required")
	}
	if r.StartsAt.IsZero() {
		r.StartsAt = now
	}
	if !r.EndsAt.After(r.StartsAt) || !r.EndsAt.After(now) {
		return errors.New("endsAt must be in the future and after startsAt")
	}
	return nil
}

func (h *Handler) CreateSilence(c *gin.Context) {
	client, ok := alertmanagerClient(c, common.VerbCreate)
	if !ok {
		return
	}
	var req createSilenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.validate(time.Now()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user := c.MustGet("user").(model.User)
	id, err := client.CreateSilence(c.Request.Context(), alertmanager.Silence{
		Matchers:  req.Matchers,
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
		CreatedBy: user.Key(),
		Comment:   req.Comment,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"silenceID": id})
}

func (h *Handler) ExpireSilence(c *gin.Context) {
	client, ok := alertmanagerClient(c, common.VerbDelete)
	if !ok {
		return
	}
	id := c.Param("id")
	// Raw-path routing decodes %2F, so only a UUID may reach the upstream path.
	if !silenceIDPattern.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid silence id"})
		return
	}
	if err := client.ExpireSilence(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
