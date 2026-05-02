# Kubernetes 部署

Memoh 可以用 `container.backend = "kubernetes"` 跑在 Kubernetes 上。这个模式下，server 会为每个机器人 workspace 创建一个 Pod 和一个 PVC。

仓库里提供了一套 kustomize 示例：`deploy/kubernetes`。它是可改造的起点，不是托管式 Helm chart。

## 部署内容

这套 manifests 包含：

- `memoh-server`：API server 和同进程 agent
- `memoh-web`：网页端
- `memoh-browser`：浏览器网关
- `memoh-postgres`：PostgreSQL StatefulSet
- 记忆向量索引存储在 PostgreSQL 的 pgvector 中
- `memoh-server` initContainer：server 启动前执行 `migrate up`
- `memoh-runtime-installer`：DaemonSet，把 workspace bridge/runtime 文件复制到每个节点的 `/opt/memoh/runtime`
- RBAC：允许 server 在 `memoh` namespace 创建 workspace Pod、PVC、Service 和 VolumeSnapshot

## 前置条件

- Kubernetes 1.25+
- 有默认 StorageClass，或者修改 PVC 和 `kubernetes.pvc_storage_class`
- 节点允许 runtime installer DaemonSet 写入 `/opt/memoh/runtime`
- 可选：如果要用 workspace snapshot，需要 CSI snapshot controller
- 可选：如果集群不能直接拉公开镜像，需要 image pull secret

当前 Kubernetes workspace runtime 会把每个节点上的 `/opt/memoh/runtime` 作为 hostPath 挂进机器人 Pod。示例里的 DaemonSet 会准备这个目录。如果你的集群禁用 hostPath，需要先改造 runtime 分发方式再使用 Kubernetes backend。

## 修改 Secret

部署前先编辑 `deploy/kubernetes/config-secret.yaml`：

- 修改 `admin.password`
- 修改 `auth.jwt_secret`
- 同时修改 `config.toml` 里的 PostgreSQL 密码和 `memoh-postgres` Secret
- 如果没有默认 StorageClass，设置 `kubernetes.pvc_storage_class`
- 如需私有镜像拉取，设置 `kubernetes.image_pull_secret`

关键配置段：

```toml
[container]
backend = "kubernetes"
default_image = "debian:bookworm-slim"
image_pull_policy = "if_not_present"
data_root = "/opt/memoh/data"
runtime_dir = "/opt/memoh/runtime"

[kubernetes]
namespace = "memoh"
in_cluster = true
service_account_name = "memoh-workspace"
pvc_size = "10Gi"
bridge_port = 9090
```

## 部署

```bash
kubectl apply -k deploy/kubernetes
kubectl -n memoh rollout status deployment/memoh-server
kubectl -n memoh rollout status deployment/memoh-web
kubectl -n memoh rollout status daemonset/memoh-runtime-installer
```

本地访问：

```bash
kubectl -n memoh port-forward svc/memoh-web 8082:8082
kubectl -n memoh port-forward svc/memoh-server 8080:8080
```

然后打开 `http://localhost:8082`。

如果本地集群能直接使用本地构建的镜像，可以用这个 overlay：

```bash
kubectl apply -k deploy/kubernetes-local
```

它会把 `memohai/server`、`memohai/web` 和 `memohai/browser` 改写成 `k8s-dev` tag。

## 更新镜像

可以用 kustomize overlay 固定镜像版本。临时测试时也可以：

```bash
kubectl kustomize deploy/kubernetes \
  | sed 's/memohai\/server:latest/memohai\/server:0.8.0/g' \
  | kubectl apply -f -
```

正式环境建议建一个 overlay 目录，把镜像版本和存储参数显式写死。

## 注意与限制

- server Deployment 会覆盖镜像 entrypoint，直接运行 `/app/memoh-server serve`。Docker Compose 的 entrypoint 会启动内置 containerd，Kubernetes backend 不需要它。
- server Pod 会先在 initContainer 里运行 `/app/memoh-server migrate up`，再启动 API server。
- server 探针使用 `GET /ping`；`/health` 当前是 HEAD endpoint。
- 机器人 workspace Pod 使用 PVC 作为 `/data`，容量来自 `kubernetes.pvc_size`。
- snapshot 依赖 VolumeSnapshot CRD 和可用的 CSI snapshot controller。
- Tailscale、NetBird 等 provider-backed overlay 可能需要额外 RBAC，具体取决于 provider 配置。
