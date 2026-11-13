package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/logging"
	"k8s.io/klog/v2"
)

var errLoggingNotInstalled = errors.New("log alerts need logging, which is not installed in this cluster")

// checkLogRuleName refuses a name another log rule or a Prometheus rule
// already has, since alerts are told apart by name.
func checkLogRuleName(ctx context.Context, cs *cluster.ClientSet, rules []LogRule, name, previous string) error {
	for _, r := range rules {
		if r.Name == name && r.Name != previous {
			return fmt.Errorf("a log rule named %s already exists", name)
		}
	}
	if cs.PromClient == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, exprCheckTimeout)
	defer cancel()
	result, err := cs.PromClient.Rules(ctx)
	if err != nil {
		klog.V(2).Infof("Failed to check log rule %s against the Prometheus rules of cluster %s: %v", name, cs.Name, err)
		return nil
	}
	for _, group := range result.Groups {
		for _, r := range group.Rules {
			if rule, ok := r.(v1.AlertingRule); ok && rule.Name == name {
				return fmt.Errorf("a Prometheus rule named %s already exists", name)
			}
		}
	}
	return nil
}

// bindLogRule reads and checks a log rule request, and the cluster's log
// rules it changes. It writes the error response itself.
func bindLogRule(c *gin.Context, previous string) (LogRule, []LogRule, bool) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var req LogRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return req, nil, false
	}
	if err := req.normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return req, nil, false
	}
	ctx := c.Request.Context()
	installed, err := logging.StorageInstalled(cs)
	if err != nil {
		writeRuleError(c, err)
		return req, nil, false
	}
	if !installed {
		writeRuleError(c, errLoggingNotInstalled)
		return req, nil, false
	}
	rules, err := LoadLogRules(ctx, cs)
	if err != nil {
		writeRuleError(c, err)
		return req, nil, false
	}
	if err := checkLogRuleName(ctx, cs, rules, req.Name, previous); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return req, nil, false
	}
	return req, rules, true
}

// CreateLogAlertRule adds a log rule and deploys vmalert, which picks up the
// rule within a minute.
func CreateLogAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbCreate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	req, rules, ok := bindLogRule(c, "")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := ensureLogAlerter(ctx, cs); err != nil {
		writeRuleError(c, err)
		return
	}
	if err := saveLogRules(ctx, cs, append(rules, req)); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"name": req.Name})
}

func UpdateLogAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbUpdate) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	name := c.Param("name")
	req, rules, ok := bindLogRule(c, name)
	if !ok {
		return
	}
	index := slices.IndexFunc(rules, func(r LogRule) bool { return r.Name == name })
	if index < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	ctx := c.Request.Context()
	if err := ensureLogAlerter(ctx, cs); err != nil {
		writeRuleError(c, err)
		return
	}
	updated := slices.Clone(rules)
	updated[index] = req
	if err := saveLogRules(ctx, cs, updated); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rule updated"})
}

// DeleteLogAlertRule removes a log rule. vmalert keeps running with the
// rules that are left.
func DeleteLogAlertRule(c *gin.Context) {
	if !canWriteRules(c, common.VerbDelete) {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	ctx := c.Request.Context()
	rules, err := LoadLogRules(ctx, cs)
	if err != nil {
		writeRuleError(c, err)
		return
	}
	name := c.Param("name")
	remaining := slices.DeleteFunc(slices.Clone(rules), func(r LogRule) bool { return r.Name == name })
	if len(remaining) == len(rules) {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	if err := saveLogRules(ctx, cs, remaining); err != nil {
		writeRuleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}
