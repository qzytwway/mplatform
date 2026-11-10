<div align="center">

<img src="./ui/src/assets/icon.svg" alt="MPlatform Logo" width="128" height="128">

# MPlatform

**所有集群，一个工作空间。**

MPlatform 是一个轻量、开源的 Kubernetes 工作空间，面向多集群运维、可观测性、访问控制与 AI 辅助排障。

你可以在 macOS 和 Windows 上使用 **MPlatform 桌面版（Beta）**，也可以将 MPlatform 部署为共享 Web 控制台。

[![Release](https://img.shields.io/github/v/release/qzytwway/mplatform?style=flat-square&logo=github&label=Release)](https://github.com/qzytwway/mplatform/releases)
[![Stars](https://img.shields.io/github/stars/qzytwway/mplatform?style=flat-square&logo=github&label=Stars)](https://github.com/qzytwway/mplatform/stargazers)
[![Downloads](https://img.shields.io/github/downloads/qzytwway/mplatform/total?style=flat-square&logo=github&label=Downloads)](https://github.com/qzytwway/mplatform/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg?style=flat-square)](LICENSE)

[**版本发布**](https://github.com/qzytwway/mplatform/releases)

[English](./README.md) · **中文**

</div>

<img width="1586" height="1167" alt="image" src="https://github.com/user-attachments/assets/a88a63b7-5b71-444d-8d98-66f147a68ef7" />

## 为什么选择 MPlatform

### 可观测

- 通过各集群独立配置的 Prometheus 查看实时 CPU、内存和网络指标。
- 实时查看并过滤 Pod 日志，在同一上下文中检查事件、状态和关联资源。

### 运维

- 跨多个集群管理 Kubernetes 核心资源、CRD 和 Helm Release。
- 在线编辑 YAML、扩缩容或重启工作负载，并使用面向 Pod、Node 和 kubectl 的 Web 终端。
- 通过全局搜索快速定位资源，或使用内置 Kube Proxy 直接访问 Pod 和 Service。

### 治理

- 支持 OAuth、LDAP、本地账户、MFA 和 Passkey 登录。
- 通过 RBAC 和集群访问权限控制操作范围，并使用审计日志追踪资源变更。

### AI 辅助

- 使用内置 AI 助手检查资源、日志和 Prometheus 指标。
- 写操作执行前需要用户确认，且 AI 助手遵循当前用户的 RBAC 权限。

## 快速开始

### MPlatform 桌面版（Beta）

在 macOS 和 Windows 上使用本地 kubeconfig 管理集群，无需部署 MPlatform 服务。

下载安装包：[macOS DMG](https://github.com/qzytwway/mplatform/releases/latest/download/mplatform-darwin-arm64.dmg) · [Windows x64](https://github.com/qzytwway/mplatform/releases/latest/download/mplatform-windows-amd64.exe)。

macOS（Apple Silicon，macOS 13+）也可以通过 Homebrew 安装：

```bash
brew install --cask qzytwway/tap/mplatform
```

macOS Beta 版尚未签名和公证，首次打开如被系统拦截，请执行：

```bash
xattr -dr com.apple.quarantine "/Applications/MPlatform.app"
```

### 通过 Helm 自行部署

如需快速体验，可通过 OCI Registry 将 MPlatform 安装到独立的命名空间：

```bash
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform \
  --namespace mplatform-system \
  --create-namespace
```

将服务转发到本地：

```bash
kubectl port-forward --namespace mplatform-system svc/mplatform 8080:8080
```

打开 [http://localhost:8080](http://localhost:8080)，创建首个管理员账户，并按照初始化流程连接集群。如果 MPlatform 运行在需要管理的集群中，选择 `in-cluster` 连接类型即可完成最简单的配置。

> [!IMPORTANT]
> Chart 默认值仅适合快速体验。用于生产环境前，请启用持久化存储或配置外部数据库、替换默认加密密钥，并检查 Chart 创建的集群级 RBAC 权限。

## 其他安装方式

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

独立 Manifest 适合快速体验。除非额外挂载持久化存储，否则应用数据会保存在容器内。

```bash
kubectl apply -f https://raw.githubusercontent.com/qzytwway/mplatform/main/deploy/install.yaml
kubectl port-forward --namespace kube-system svc/mplatform 8080:8080
```

该 Manifest 会为 ServiceAccount 授予 `cluster-admin` 权限。在测试环境之外使用前，请检查并收紧这些权限。

### 从源码构建

构建 MPlatform 需要 Go 1.26、Node.js `^20.19.0` 或 `>=22.12.0`、pnpm 和 Make。

```bash
git clone https://github.com/qzytwway/mplatform.git
cd mplatform
make deps
make build
./mplatform
```

## 社区

- 提交 Pull Request 前请先阅读[贡献指南](./CONTRIBUTING.md)。
- 请按照[安全策略](./SECURITY.md)报告安全漏洞。

## 许可证

MPlatform 基于 [Apache License 2.0](LICENSE) 发布。
