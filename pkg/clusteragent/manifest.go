package clusteragent

import (
	"fmt"
	"strings"
)

// GenerateManifest builds the Kubernetes manifest (Secret, ServiceAccount,
// ClusterRoleBinding, Deployment) used to deploy the Cluster Agent inside a
// target cluster. The image and server URL are injected so the manifest is
// always consistent with the platform configuration.
func GenerateManifest(serverURL, token, publicKey, image string) string {
	serverURL = strings.TrimSpace(serverURL)
	token = strings.TrimSpace(token)
	publicKey = strings.TrimSpace(publicKey)
	image = strings.TrimSpace(image)
	// JSON-encode the string values so the YAML is always valid regardless
	// of special characters.
	tokenJSON := fmt.Sprintf("%q", token)
	publicKeyJSON := fmt.Sprintf("%q", publicKey)
	serverJSON := fmt.Sprintf("%q", serverURL)
	imageJSON := fmt.Sprintf("%q", image)
	tokenHashJSON := fmt.Sprintf("%q", tokenHash(token))
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: mplatform-cluster-agent-token
  namespace: kube-system
type: Opaque
stringData:
  token: %s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mplatform-cluster-agent
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mplatform-cluster-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: mplatform-cluster-agent
    namespace: kube-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mplatform-cluster-agent
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: mplatform-cluster-agent
  template:
    metadata:
      annotations:
        mplatform.kubernetes.io/cluster-agent-token-hash: %s
      labels:
        app.kubernetes.io/name: mplatform-cluster-agent
    spec:
      serviceAccountName: mplatform-cluster-agent
      containers:
        - name: cluster-agent
          image: %s
          command:
            - /app/mplatform
          args:
            - cluster-agent
            - --server=$(MPLATFORM_SERVER)
            - --token=$(CLUSTER_AGENT_TOKEN)
            - --public-key=$(CLUSTER_AGENT_PUBLIC_KEY)
          env:
            - name: MPLATFORM_SERVER
              value: %s
            - name: CLUSTER_AGENT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: mplatform-cluster-agent-token
                  key: token
            - name: CLUSTER_AGENT_PUBLIC_KEY
              value: %s
`, tokenJSON, tokenHashJSON, imageJSON, serverJSON, publicKeyJSON)
}
