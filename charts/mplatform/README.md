# mplatform

![Version: 0.17.0](https://img.shields.io/badge/Version-0.17.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)

A Helm chart for Kubernetes Dashboard - MPlatform

## Installation

### Install from OCI registry

```bash
# Install in kube-system namespace (recommended)
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform -n kube-system

# Or install in custom namespace
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform -n my-namespace --create-namespace
```

### Add Helm Repository

```bash
helm repo add mplatform https://charts.mplatform.example.com
helm repo update
```

### Install Chart

```bash
# Install in kube-system namespace (recommended)
helm install mplatform mplatform/mplatform -n kube-system

# Or install in custom namespace
helm install mplatform mplatform/mplatform -n my-namespace --create-namespace
```

### Upgrade Chart

```bash
helm upgrade mplatform oci://ghcr.io/qzytwway/charts/mplatform -n kube-system

# Or use the Helm repository
helm upgrade mplatform mplatform/mplatform -n kube-system
```

### Uninstall Chart

```bash
helm uninstall mplatform -n kube-system
```

### Chart Values

See [values.yaml](./values.yaml).
