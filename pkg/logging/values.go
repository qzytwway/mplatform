package logging

import (
	"fmt"
	"strings"
)

const (
	Namespace           = "mplatform-logging"
	victoriaLogsRelease = "mplatform-victoria-logs"
	vectorRelease       = "mplatform-vector"
	victoriaLogsPort    = 9428

	clusterField   = "cluster"
	NamespaceField = "kubernetes.pod_namespace"
	PodField       = "kubernetes.pod_name"
	ContainerField = "kubernetes.container_name"
)

type storageOptions struct {
	StorageClass string
	StorageSize  string
	Retention    string
}

type collectorOptions struct {
	ClusterName string
}

// ServiceURL is the in-cluster address of a cluster's VictoriaLogs, which the
// platform reaches through the Kubernetes API proxy.
func ServiceURL() string {
	return fmt.Sprintf("http://%s.%s.svc:%d", victoriaLogsRelease, Namespace, victoriaLogsPort)
}

// victoriaLogsValues keeps VictoriaLogs inside its cluster: only the
// cluster's Vector agent and, through the API proxy, the platform reach it.
func victoriaLogsValues(opts storageOptions) map[string]any {
	persistentVolume := map[string]any{"enabled": true, "size": opts.StorageSize}
	if opts.StorageClass != "" {
		persistentVolume["storageClassName"] = opts.StorageClass
	}
	return map[string]any{
		"server": map[string]any{
			"fullnameOverride": victoriaLogsRelease,
			"retentionPeriod":  opts.Retention,
			"persistentVolume": persistentVolume,
		},
		"vector": map[string]any{"enabled": false},
	}
}

func vectorValues(opts collectorOptions) map[string]any {
	return map[string]any{
		"role":             "Agent",
		"fullnameOverride": vectorRelease,
		"service":          map[string]any{"enabled": false},
		// Collect logs from every node, including tainted control-plane nodes.
		"tolerations": []any{map[string]any{"operator": "Exists"}},
		"customConfig": map[string]any{
			"data_dir": "/vector-data-dir",
			"api":      map[string]any{"enabled": false},
			"sources": map[string]any{
				"k8s": map[string]any{"type": "kubernetes_logs"},
			},
			"transforms": map[string]any{
				"parser": map[string]any{
					"type":   "remap",
					"inputs": []any{"k8s"},
					"source": ".log = parse_json(.message) ?? .message\ndel(.message)\n",
				},
			},
			"sinks": map[string]any{
				"vlogs": map[string]any{
					"type":        "elasticsearch",
					"inputs":      []any{"parser"},
					"endpoints":   []any{ServiceURL() + "/insert/elasticsearch/"},
					"mode":        "bulk",
					"api_version": "v8",
					"compression": "gzip",
					"healthcheck": map[string]any{"enabled": false},
					"request": map[string]any{
						"headers": map[string]any{
							"AccountID":        "0",
							"ProjectID":        "0",
							"VL-Time-Field":    "timestamp",
							"VL-Msg-Field":     "message,msg,_msg,log.msg,log.message,log",
							"VL-Stream-Fields": strings.Join([]string{clusterField, NamespaceField, PodField, ContainerField}, ","),
							"VL-Extra-Fields":  clusterField + "=" + opts.ClusterName,
						},
					},
				},
			},
		},
	}
}
