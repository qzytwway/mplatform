<div align="center">

<img src="./ui/src/assets/icon.svg" alt="MPlatform logo" width="128" height="128">

# MPlatform

**All your clusters. One workspace.**

MPlatform is a lightweight, open-source Kubernetes workspace for multi-cluster operations, observability, access control, and AI-assisted troubleshooting.

Use **MPlatform Desktop (Beta)** on macOS and Windows, or deploy MPlatform as a shared web console.

[![Release](https://img.shields.io/github/v/release/qzytwway/mplatform?style=flat-square&logo=github&label=Release)](https://github.com/qzytwway/mplatform/releases)
[![Stars](https://img.shields.io/github/stars/qzytwway/mplatform?style=flat-square&logo=github&label=Stars)](https://github.com/qzytwway/mplatform/stargazers)
[![Downloads](https://img.shields.io/github/downloads/qzytwway/mplatform/total?style=flat-square&logo=github&label=Downloads)](https://github.com/qzytwway/mplatform/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg?style=flat-square)](LICENSE)

[**Releases**](https://github.com/qzytwway/mplatform/releases)

**English** · [中文](./README_zh.md)

</div>

<img width="1586" height="1167" alt="image" src="https://github.com/user-attachments/assets/5710204d-5d34-44af-85dc-3b436e205c12" />

## Why MPlatform

### Observe

- Monitor CPU, memory, and network usage with per-cluster Prometheus integrations.
- Stream and filter pod logs, then inspect events, conditions, and related resources in context.

### Operate

- Manage core Kubernetes resources, CRDs, and Helm releases across multiple clusters.
- Edit live YAML, scale or restart workloads, and use browser-based pod, node, and kubectl terminals.
- Find resources with global search and access pods or services through the built-in Kube Proxy.

### Govern

- Support OAuth, LDAP, local accounts, MFA, and passkey login.
- Control access with RBAC and per-cluster permissions, backed by audit logs for resource changes.

### Work with AI

- Inspect resources, logs, and Prometheus metrics with the built-in AI assistant.
- Review and confirm write operations before execution; the assistant respects the current user's RBAC permissions.

## Quick Start

### MPlatform Desktop (Beta)

Manage your clusters with local kubeconfig files on macOS and Windows, without deploying a MPlatform server.

Download: [macOS DMG](https://github.com/qzytwway/mplatform/releases/latest/download/mplatform-darwin-arm64.dmg) · [Windows x64 installer](https://github.com/qzytwway/mplatform/releases/latest/download/mplatform-windows-amd64.exe).

On macOS (Apple Silicon, macOS 13+), you can also install with Homebrew:

```bash
brew install --cask qzytwway/tap/mplatform
```

The macOS beta is not yet signed or notarized. If the first launch is blocked, run:

```bash
xattr -dr com.apple.quarantine "/Applications/MPlatform.app"
```

### Self-hosted with Helm

For a quick evaluation, install MPlatform from the OCI registry into a dedicated namespace:

```bash
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform \
  --namespace mplatform-system \
  --create-namespace
```

Forward the service to your local machine:

```bash
kubectl port-forward --namespace mplatform-system svc/mplatform 8080:8080
```

Open [http://localhost:8080](http://localhost:8080), create the first administrator, and follow the setup flow to connect a cluster. When MPlatform runs inside the cluster it manages, choose the `in-cluster` connection type for the simplest setup.

> [!IMPORTANT]
> The default chart values are intended for evaluation. Before using MPlatform in production, enable persistent storage or configure an external database, replace the default encryption key, and review the chart's cluster-wide RBAC permissions.

## Other Installation Options

### Docker

```bash
mkdir -p data
docker run -d --name mplatform \
  -p 8080:8080 \
  -v "$(pwd)/data:/data" \
  -e DB_DSN=/data/db.sqlite \
  ghcr.io/qzytwway/mplatform:latest
```

### Kubernetes Manifest

The standalone manifest is intended for evaluation and stores application data inside the container unless you add persistent storage.

```bash
kubectl apply -f https://raw.githubusercontent.com/qzytwway/mplatform/main/deploy/install.yaml
kubectl port-forward --namespace kube-system svc/mplatform 8080:8080
```

The manifest grants its service account `cluster-admin`. Review and restrict these permissions before using it outside a test environment.

### Build from Source

Building MPlatform requires Go 1.26, Node.js `^20.19.0` or `>=22.12.0`, pnpm, and Make.

```bash
git clone https://github.com/qzytwway/mplatform.git
cd mplatform
make deps
make build
./mplatform
```

## Community

- Read the [contributing guidelines](./CONTRIBUTING.md) before opening a pull request.
- Report vulnerabilities according to the [security policy](./SECURITY.md).

## License

MPlatform is licensed under the [Apache License 2.0](LICENSE).
