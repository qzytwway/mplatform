<div align="center">

<img src="./ui/src/assets/icon-animated.svg" alt="MPlatform logo" width="128" height="128">

# MPlatform

**一个控制台，管好多套 Kubernetes 集群，也管好跑在上面的大模型。**

[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg?style=flat-square)](LICENSE)

</div>

MPlatform 是一个轻量的 Kubernetes 管理平台，后端用 Go，前端用 React，打包成单个二进制或镜像即可运行。它把多集群运维、可观测性、权限管理和大模型推理部署放进同一个 Web 控制台：工程师可以在界面上完成从接入集群、准备 GPU，到部署 vLLM / SGLang 推理服务、对外发布接口的全过程，不需要手写 YAML。

<table>
  <tr>
    <td width="33%"><img src="docs/images/workbench-issues-first.png" alt="模型服务工作台的问题优先布局"></td>
    <td width="33%"><img src="docs/images/prometheus-metrics.png" alt="Prometheus 监控面板"></td>
    <td width="33%"><img src="docs/images/logs.png" alt="日志检索"></td>
  </tr>
  <tr>
    <td align="center">问题优先工作台（原型图）</td>
    <td align="center">Prometheus 监控面板</td>
    <td align="center">日志检索</td>
  </tr>
</table>

## 功能

### 集群管理

- 在主机上通过 SSH 直接创建新集群（kubeadm），或纳管已有集群：上传 kubeconfig、In-Cluster、Cluster Agent 三种接入方式。
- 浏览和管理各类 Kubernetes 资源与 CRD，在线编辑 YAML，伸缩、重启工作负载。
- 浏览器内的 Pod 终端、节点终端和 kubectl 终端；全局搜索；通过内置代理访问集群内的 Pod 和 Service。
- 应用商店：安装和管理 Helm chart，内置一个本地 chart 仓库，可在界面上传 chart 包。

### 模型服务

- **模型仓库**：登记模型，来源可以是节点上的本地目录，也可以是 S3 兼容的对象存储（启动时自动下载）。
- **推理服务**：支持 vLLM 和 SGLang，三种部署模式：
  - **单机**：一个 Pod 跑一个副本，可指定节点。
  - **多机**：基于 LeaderWorkerSet，一个副本跨多台机器，自动设置张量并行 / 流水线并行参数。
  - **PD 分离**：基于 DisaggregatedSet，Prefill 和 Decode 分别设置副本数、节点数、GPU、镜像、资源、参数、环境变量和调度；单个 P 或 D 也可以跨多台机器。KV 传输支持 Mooncake（RDMA / TCP）和 NIXL，路由使用 SGLang Router 或 vLLM Router，可多副本、可选路由策略。
- **网络**：多机和 PD 服务可通过 RDMA 设备插件（可选 Multus 网络）或主机网络使用 RDMA。
- **调度**：节点标签、容忍、优先级类，PD 的 P / D 可以分别设置。
- **运维**：创建后可在界面修改配置并滚动更新；按副本或按 P / D / 路由分别伸缩；推理引擎卡死时由存活探针自动重启。
- **GPU**：整卡由 GPU Operator 提供，vGPU（按显存 / 算力切分）由 HAMi 提供，可查看各节点的 GPU 分配情况。
- **网关**：基于 Higress 把推理服务发布为 OpenAI 兼容接口，按调用方发放 API Key、设置配额，并统计用量。
- **推理指标**：每个服务自动创建 PodMonitor，内置推理服务监控面板：首 token 延迟、每 token 延迟、吞吐、排队数、KV Cache 使用率。

### 可观测性

- 按集群接入 Prometheus（kube-prometheus-stack），查看 CPU、内存、网络和 GPU 指标，内置监控面板。
- 日志采集与检索（Vector + VictoriaLogs），支持 Pod 日志实时查看与过滤。
- 告警规则与通知。

### 权限与安全

- 本地账号、OAuth、LDAP 登录，支持 MFA 和 Passkey。
- 基于 RBAC 的权限控制，可按集群授权；资源变更有审计记录；支持 API 密钥。

### AI 助手

- 内置 AI 助手，可以查看资源、日志和 Prometheus 指标来协助排障；写操作需要人工确认，并遵守当前用户的 RBAC 权限。

## 部署

MPlatform 默认监听 `8080` 端口，默认使用 SQLite 存数据，也支持 MySQL 和 PostgreSQL。下面三种方式任选其一。

> [!IMPORTANT]
> 生产环境务必设置自己的 `MPLATFORM_ENCRYPT_KEY`（加密集群凭证等敏感数据）和 `JWT_SECRET`（签发登录令牌），并且部署后不要再更换加密密钥，否则已保存的凭证将无法解密。可以用 `openssl rand -hex 32` 生成。

### 方式一：Docker

适合单机试用，或把平台部署在集群外的一台服务器上。

1. 准备数据目录和配置文件。密钥只生成这一次，之后重建容器时继续使用同一个文件：

   ```bash
   mkdir -p mplatform/data/charts && cd mplatform
   cat > mplatform.env <<EOF
   DB_DSN=/data/mplatform.db
   CHART_DIR=/data/charts
   MPLATFORM_ENCRYPT_KEY=$(openssl rand -hex 32)
   JWT_SECRET=$(openssl rand -hex 32)
   EOF
   ```

2. 启动容器：

   ```bash
   docker run -d --name mplatform --restart unless-stopped \
     -p 8080:8080 \
     -v "$(pwd)/data:/data" \
     --env-file mplatform.env \
     ghcr.io/qzytwway/mplatform:latest
   ```

   也可以从源码自己构建镜像，把上面命令里的镜像换成 `mplatform:local`：

   ```bash
   git clone https://github.com/qzytwway/mplatform.git
   cd mplatform
   docker build -t mplatform:local .
   ```

3. 打开 `http://<服务器地址>:8080`，创建第一个管理员账号，然后按引导接入集群。平台在集群外运行时，用“上传 kubeconfig”或“Cluster Agent”方式接入；kubeconfig 里的 API Server 地址必须能从容器内访问（`127.0.0.1` 这类本机地址不行）。

数据库、上传的 chart 包都保存在 `data/` 目录里，升级时拉取新镜像、删除旧容器后用同样的命令重新启动即可。

### 方式二：Docker Compose

适用场景与方式一相同，用一个文件管理容器配置，启动和升级更省事。

1. 按方式一第 1 步准备 `data/` 目录和 `mplatform.env`，然后在同一目录（`mplatform/`）下创建 `docker-compose.yaml`：

   ```yaml
   services:
     mplatform:
       image: ghcr.io/qzytwway/mplatform:latest
       container_name: mplatform
       restart: unless-stopped
       ports:
         - "8080:8080"
       env_file: mplatform.env
       volumes:
         - ./data:/data
   ```

2. 启动容器：

   ```bash
   docker compose up -d
   ```

3. 打开 `http://<服务器地址>:8080`，后续步骤与方式一第 3 步相同。

升级时拉取新镜像并重建容器，数据仍保存在 `data/` 目录：

```bash
docker compose pull && docker compose up -d
```

### 方式三：Kubernetes（Helm）

适合正式使用。平台部署在它要管理的集群内时，可以直接用 In-Cluster 方式接入这个集群。

1. 准备一个 values 文件，开启持久化并设置密钥：

   ```bash
   cat > mplatform-values.yaml <<EOF
   encryptKey: "$(openssl rand -hex 32)"
   jwtSecret: "$(openssl rand -hex 32)"
   # SQLite 使用 ReadWriteOnce 的 PVC 时必须用 Recreate，避免新旧 Pod 抢同一个卷
   deploymentStrategy:
     type: Recreate
   db:
     sqlite:
       persistence:
         pvc:
           enabled: true
   chartStore:
     persistence:
       pvc:
         enabled: true
   EOF
   ```

   请妥善保存这个文件，以后执行 `helm upgrade` 时也要带上它，否则密钥会被改回默认值。使用外部数据库时，把 `db.type` 设为 `mysql` 或 `postgres`，并在 `db.dsn` 中填写连接串，例如 `mysql://user:password@tcp(host:3306)/mplatform`。

2. 安装。使用仓库中的 chart：

   ```bash
   git clone https://github.com/qzytwway/mplatform.git
   helm install mplatform ./mplatform/charts/mplatform \
     --namespace mplatform-system --create-namespace \
     -f mplatform-values.yaml
   ```

   或者使用已发布的 OCI chart：

   ```bash
   helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform \
     --namespace mplatform-system --create-namespace \
     -f mplatform-values.yaml
   ```

3. 访问。临时访问可以用端口转发，然后打开 `http://localhost:8080`：

   ```bash
   kubectl port-forward --namespace mplatform-system svc/mplatform 8080:8080
   ```

   长期使用可在 values 里开启 `ingress`（或 Gateway API 的 `gateway`）配置域名。首次打开时创建管理员账号，接入集群时选择 In-Cluster。

升级：

```bash
helm upgrade mplatform ./mplatform/charts/mplatform \
  --namespace mplatform-system -f mplatform-values.yaml
```

chart 默认使用 `latest` 镜像，并在每次启动 Pod 时重新拉取。`helm upgrade` 只在配置有变化时才会重建 Pod，只想更新到最新镜像时，重启 Deployment 即可：

```bash
kubectl rollout restart deployment/mplatform --namespace mplatform-system
```

更多可配置项（镜像、资源、Ingress、配置文件托管、RBAC 等）见 [charts/mplatform/values.yaml](./charts/mplatform/values.yaml)。

> [!NOTE]
> chart 默认给平台的 ServiceAccount 授予集群内全部权限，以便管理集群和安装组件。如有需要，可通过 values 中的 `rbac.rules` 收紧。

### 常用环境变量

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `PORT` | 监听端口 | `8080` |
| `DB_TYPE` | 数据库类型：`sqlite`、`mysql`、`postgres` | `sqlite` |
| `DB_DSN` | SQLite 文件路径，或 MySQL / PostgreSQL 连接串 | `dev.db` |
| `MPLATFORM_ENCRYPT_KEY` | 敏感数据的加密密钥 | 内置默认值（不安全） |
| `JWT_SECRET` | 登录令牌的签名密钥 | 内置默认值（不安全） |
| `CHART_DIR` | 应用商店本地仓库的 chart 目录 | `chart-packages` |
| `HOST` | 平台对外地址，用于生成 OAuth 回调地址 | 从请求头获取 |
| `MPLATFORM_BASE` | 部署在子路径下时的路径前缀，例如 `/mplatform` | 空 |
| `MPLATFORM_CONFIG_FILE` | 配置文件路径，用文件托管集群、OAuth、LDAP、RBAC 等配置（界面上对应部分变为只读） | 空 |

使用 Helm 时这些变量由 chart 根据 values 设置，额外的变量可以通过 `extraEnvs` 传入。

## 部署后：准备模型服务所需组件

模型服务依赖的组件都在平台界面上一键安装，它们的 chart 从应用商店的本地仓库（`local`，对应 `CHART_DIR` 目录）读取。下表这些已验证版本的 chart 包都放在代码仓库的 [chart-packages](chart-packages/THIRD_PARTY_NOTICES.md) 目录里，各自的来源、许可证和校验和也列在那里：

- 从源码运行（`make dev`、`./mplatform`）时，`CHART_DIR` 默认就是这个目录，不用再准备。
- 用 Docker 或 Helm 部署时，平台镜像里不含 chart 包，先在“应用商店”上传这些包（`.tgz`），或者把它们复制到 `CHART_DIR` 目录。

| 组件 | 用途 | chart | 已验证版本 |
| --- | --- | --- | --- |
| LeaderWorkerSet | 多机、PD 分离推理服务 | `lws` | v1.0.0 |
| GPU Operator | 整卡 GPU 的驱动和设备插件 | `gpu-operator`（定制版） | v26.7.2-mplatform.1 |
| HAMi | vGPU 切分 | `hami` | 2.11.0 |
| Higress | 推理服务网关 | `higress-core` | 2.2.6 |
| kube-prometheus-stack | 指标、监控面板与告警 | `kube-prometheus-stack` | 93.1.1 |
| VictoriaLogs + Vector | 日志采集、检索与日志告警 | `victoria-logs-single`、`vector` | 0.13.12、0.61.0 |

平台按名称读取 chart，本地仓库里同名的 chart 有多个版本时用最新的一个。其他版本也能安装，但没有和平台一起验证过，安装界面会标出“未验证版本”。GPU Operator 用官方 chart 也能装上，但 Prometheus 采集不到 GPU 指标，GPU 面板会是空的，所以要用定制版：仓库里已经放了一份，换版本时用 `make gpu-operator-chart` 重新构建，详见 [charts/addons/gpu-operator](charts/addons/gpu-operator/README.md)。

然后进入“模型服务”，按顺序安装 GPU 组件和 LeaderWorkerSet、登记模型、创建推理服务；需要对外提供接口时再安装网关并发布服务。

### 离线环境

1. 把代码仓库 `chart-packages` 目录里的 chart 包上传到应用商店，或复制到 `CHART_DIR`。
2. 上传后，各组件的安装卡片会列出它需要的全部镜像：“设置”里的“监控”“日志”，以及“模型服务”里的 GPU 资源、推理服务和网关页面。这份清单是平台用安装时的参数渲染 chart 得出的，换了 chart 版本也会跟着变。
3. 把这些镜像同步到内网镜像仓库，名称只保留最后一段，例如 `quay.io/prometheus/node-exporter:v1.12.1-distroless` 对应 `<仓库地址>/node-exporter:v1.12.1-distroless`。
4. 在“集群管理”里给集群设置镜像仓库地址。平台安装组件时会把所有镜像改写到这个地址下，包括 Helm hook 和 Operator 运行时才拉取的镜像。
5. 平台自己的镜像也要同步：平台镜像（`ghcr.io/qzytwway/mplatform`，集群代理和飞书、Bark 告警转发器也用它）和日志告警用的 vmalert，后两者分别列在“监控”和“日志”的设置卡片里。

镜像清单宁多不漏，以下镜像在平台的默认用法下不会被拉取，可以不同步：

- kube-prometheus-stack 的 `thanos`，只在启用 Thanos 时使用。
- GPU Operator 的 `k8s-cc-manager`、`vgpu-device-manager`、`kubevirt-gpu-device-plugin`、`nvidia-sandbox-device-plugin`，只在机密计算或沙箱节点上运行。

GPU 驱动镜像实际拉取时，tag 会带上节点操作系统的后缀，例如 `nvcr.io/nvidia/driver:595.91.07-ubuntu22.04`，请按节点的系统同步对应镜像；节点已经预装驱动时不需要这个镜像。

## 从源码构建

需要 Go 1.26、Node.js（`^20.19.0` 或 `>=22.12.0`）、pnpm 和 Make：

```bash
make deps
make build
./mplatform
```

开发时运行 `make dev`，前端开发服务器在 `5173` 端口，后端在 `8080` 端口。

## 参与贡献

- 提交 PR 前请先阅读[贡献指南](./CONTRIBUTING.md)。
- 安全问题请按[安全策略](./SECURITY.md)报告。

## 许可证

MPlatform 基于 [Apache License 2.0](LICENSE) 开源。
