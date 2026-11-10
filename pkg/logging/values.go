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
	authUsername        = "mplatform"

	// The platform writes the password to this Secret itself, so it never
	// appears in Helm values, which MPlatform shows and keeps in history.
	authSecretName    = "mplatform-logging-auth"
	authSecretKey     = "password"
	vectorPasswordEnv = "VL_PASSWORD"

	clusterField   = "cluster"
	namespaceField = "kubernetes.pod_namespace"
	podField       = "kubernetes.pod_name"
	containerField = "kubernetes.container_name"
)

type storageOptions struct {
	StorageClass string
	StorageSize  string
	Retention    string
	NodePort     int
}

type collectorOptions struct {
	ClusterName string
	WriteURL    string
}

func victoriaLogsServiceURL() string {
	return fmt.Sprintf("http://%s.%s.svc:%d", victoriaLogsRelease, Namespace, victoriaLogsPort)
}

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
			"service": map[string]any{
				"type":     "NodePort",
				"nodePort": opts.NodePort,
			},
			// The chart enables envflag with the VM_ prefix, so the password
			// stays out of the container arguments.
			"env": []any{
				map[string]any{"name": "VM_httpAuth_username", "value": authUsername},
				map[string]any{
					"name": "VM_httpAuth_password",
					"valueFrom": map[string]any{
						"secretKeyRef": map[string]any{"name": authSecretName, "key": authSecretKey},
					},
				},
			},
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
		"env": []any{
			map[string]any{
				"name": vectorPasswordEnv,
				"valueFrom": map[string]any{
					"secretKeyRef": map[string]any{"name": authSecretName, "key": authSecretKey},
				},
			},
		},
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
					"endpoints":   []any{opts.WriteURL + "/insert/elasticsearch/"},
					"mode":        "bulk",
					"api_version": "v8",
					"compression": "gzip",
					"healthcheck": map[string]any{"enabled": false},
					"auth": map[string]any{
						"strategy": "basic",
						"user":     authUsername,
						"password": "${" + vectorPasswordEnv + "}",
					},
					"request": map[string]any{
						"headers": map[string]any{
							"AccountID":        "0",
							"ProjectID":        "0",
							"VL-Time-Field":    "timestamp",
							"VL-Msg-Field":     "message,msg,_msg,log.msg,log.message,log",
							"VL-Stream-Fields": strings.Join([]string{clusterField, namespaceField, podField, containerField}, ","),
							"VL-Extra-Fields":  clusterField + "=" + opts.ClusterName,
						},
					},
				},
			},
		},
	}
}
