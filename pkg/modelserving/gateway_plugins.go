package modelserving

import (
	"github.com/qzytwway/mplatform/pkg/model"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// consumerHeader is where key-auth records the caller for the plugins
	// behind it. The published key-auth appends to a value the client sent
	// instead of replacing it, so the transformer drops that value first.
	consumerHeader = "x-mse-consumer"

	redisService = "redis-stack-server"
	redisPort    = 6379

	quotaPlugin = "ai-quota"
	// quotaAdminConsumer is no caller's name (it is not a DNS label). No
	// published route allows it, so its key only reaches the quota
	// administration path.
	quotaAdminConsumer = "_quota_admin"
	// The admin key travels in its own header: the API server proxy the
	// platform calls the gateway through keeps Authorization for itself.
	quotaAdminHeader  = "x-mplatform-quota-admin-key"
	quotaAdminIngress = "mplatform-quota-admin"
	// ai-quota serves its administration API under this fixed prefix plus
	// its admin_path.
	quotaAdminPrefix = "/v1/chat/completions"
	quotaAdminPath   = "/quota"
	// quotaIdlePath stands in for the token paths while no caller has a
	// quota: ai-quota then only serves its administration API.
	quotaIdlePath = "/_mplatform_quota_idle"
)

// tokenPaths are the OpenAI paths whose responses report token usage.
var tokenPaths = []any{"/v1/chat/completions", "/v1/completions", "/v1/embeddings"}

func redisConfig(gateway *model.ModelGateway) map[string]any {
	config := map[string]any{
		"service_name": redisService + "." + gatewayNamespace + ".svc.cluster.local",
		"service_port": int64(redisPort),
	}
	if gateway.RedisPassword != "" {
		config["password"] = string(gateway.RedisPassword)
	}
	return config
}

// pluginSpecs returns the spec of every gateway plugin but ai-quota, keyed by
// plugin name. A plugin without a spec is removed from the gateway.
func pluginSpecs(gateway *model.ModelGateway, callers []model.ModelCaller, published []publishedService) (map[string]map[string]any, error) {
	keyAuth, err := keyAuthSpec(string(gateway.QuotaAdminKey), callers, published)
	if err != nil {
		return nil, err
	}
	specs := map[string]map[string]any{
		"key-auth":     keyAuth,
		"model-router": {"defaultConfig": map[string]any{"modelToHeader": modelHeader}},
	}
	// A gateway that has not been upgraded has no Redis for the rest.
	if gateway.QuotaAdminKey == "" {
		return specs, nil
	}
	// Engines only report the usage of a streamed response when the request
	// asks for it. The transformer drops whatever the client chose, and
	// ai-proxy then asks for it on every streamed request.
	specs["transformer"] = map[string]any{"defaultConfig": map[string]any{
		"reroute": false,
		"reqRules": []any{map[string]any{
			"operate": "remove",
			"headers": []any{map[string]any{"key": consumerHeader}},
			"body":    []any{map[string]any{"key": "stream_options"}},
		}},
	}}
	specs["ai-proxy"] = map[string]any{"defaultConfig": map[string]any{"provider": map[string]any{"type": "vllm"}}}
	specs["ai-statistics"] = map[string]any{"defaultConfig": map[string]any{"enable_path_suffixes": tokenPaths}}
	if rateLimit := rateLimitSpec(gateway, callers); rateLimit != nil {
		specs["ai-token-ratelimit"] = rateLimit
	}
	return specs, nil
}

// keyAuthSpec requires a key on every route and lets each published service
// be called only by the callers allowed for its model. adminKey is empty for
// a gateway without quota administration.
func keyAuthSpec(adminKey string, callers []model.ModelCaller, published []publishedService) (map[string]any, error) {
	consumers := []any{}
	for _, caller := range callers {
		consumers = append(consumers, map[string]any{"name": caller.Name, "credential": credentialScheme + string(caller.APIKey)})
	}
	keys := []any{"Authorization"}
	rules := []any{}
	if adminKey != "" {
		consumers = append(consumers, map[string]any{"name": quotaAdminConsumer, "credential": adminKey})
		keys = append(keys, quotaAdminHeader)
		rules = append(rules, map[string]any{
			"ingress":       []any{gatewayNamespace + "/" + quotaAdminIngress},
			"configDisable": false,
			"config":        map[string]any{"allow": []any{quotaAdminConsumer}},
		})
	}
	if len(consumers) == 0 {
		key, err := newAPIKey()
		if err != nil {
			return nil, err
		}
		consumers = append(consumers, map[string]any{"name": unusedConsumer, "credential": credentialScheme + key})
	}
	for _, service := range published {
		allow := []any{}
		for _, caller := range callers {
			if caller.AllowAll || containsModel(caller.Models, service.Model) {
				allow = append(allow, caller.Name)
			}
		}
		if len(allow) == 0 {
			allow = append(allow, nobodyConsumer)
		}
		rules = append(rules, map[string]any{
			"ingress":       []any{service.Namespace + "/" + service.Name + ingressSuffix},
			"configDisable": false,
			"config":        map[string]any{"allow": allow},
		})
	}
	return map[string]any{
		"defaultConfig": map[string]any{
			"global_auth": true,
			"consumers":   consumers,
			"keys":        keys,
			"in_header":   true,
			"in_query":    false,
		},
		"matchRules": rules,
	}, nil
}

func containsModel(models []string, name string) bool {
	for _, allowed := range models {
		if allowed == name {
			return true
		}
	}
	return false
}

// rateLimitSpec limits the tokens each rate-limited caller uses per minute.
// It is nil when no caller is limited, as the plugin rejects an empty rule.
func rateLimitSpec(gateway *model.ModelGateway, callers []model.ModelCaller) map[string]any {
	limits := []any{}
	for _, caller := range callers {
		if caller.TokensPerMinute > 0 {
			limits = append(limits, map[string]any{"key": caller.Name, "token_per_minute": caller.TokensPerMinute})
		}
	}
	if len(limits) == 0 {
		return nil
	}
	return map[string]any{"defaultConfig": map[string]any{
		"rule_name":  "mplatform-callers",
		"rule_items": []any{map[string]any{"limit_by_consumer": "", "limit_keys": limits}},
		"redis":      redisConfig(gateway),
	}}
}

// quotaSpec checks the quota of every caller on the token paths when
// enforced, and only serves the administration API otherwise.
func quotaSpec(gateway *model.ModelGateway, enforced bool) map[string]any {
	paths := []any{quotaIdlePath}
	if enforced {
		paths = tokenPaths
	}
	return map[string]any{"defaultConfig": map[string]any{
		"admin_consumer":       quotaAdminConsumer,
		"admin_path":           quotaAdminPath,
		"enable_path_suffixes": paths,
		"redis":                redisConfig(gateway),
	}}
}

// buildQuotaAdminIngress is the route the platform administers quotas on,
// which exists whether or not a service is published. It only matches the
// paths ai-quota answers itself, so nothing reaches the backend.
func buildQuotaAdminIngress() *networkingv1.Ingress {
	paths := []networkingv1.HTTPIngressPath{}
	for _, suffix := range []string{"", "/refresh", "/delta"} {
		paths = append(paths, networkingv1.HTTPIngressPath{
			Path:     quotaAdminPrefix + quotaAdminPath + suffix,
			PathType: new(networkingv1.PathTypeExact),
			Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
				Name: redisService,
				Port: networkingv1.ServiceBackendPort{Number: redisPort},
			}},
		})
	}
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaAdminIngress,
			Namespace: gatewayNamespace,
			Labels:    map[string]string{managedByLabel: managedByValue},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new(ingressClass),
			Rules: []networkingv1.IngressRule{{IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
			}}},
		},
	}
}
