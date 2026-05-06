# Kubernetes deployment

Memoh can run on Kubernetes with `container.backend = "kubernetes"`. In this mode, the server creates one Pod and one PVC per bot workspace.

This repository includes a kustomize-based starter deployment in `deploy/kubernetes`. Treat it as a production baseline to adapt, not as a managed Helm chart.

## What it deploys

The manifests include:

- `memoh-server`: API server and in-process agent
- `memoh-web`: web UI
- `memoh-postgres`: PostgreSQL StatefulSet
- `memoh-qdrant`: Qdrant StatefulSet
- `memoh-server` initContainer: runs `migrate up` before the server starts
- `memoh-runtime-installer`: DaemonSet that copies the workspace bridge/runtime files to `/opt/memoh/runtime` on every node
- RBAC for the server to create workspace Pods, PVCs, Services, and VolumeSnapshots in the `memoh` namespace

## Prerequisites

- Kubernetes 1.25+
- A default StorageClass, or edit the PVCs and `kubernetes.pvc_storage_class`
- Nodes that allow the runtime installer DaemonSet to write `/opt/memoh/runtime`
- Optional: CSI snapshot controller if you want workspace snapshots
- Optional: an image pull secret if your cluster cannot pull public images directly

The current Kubernetes workspace runtime mounts `/opt/memoh/runtime` into every bot Pod as a hostPath. The included DaemonSet prepares that directory on each node. If your cluster forbids hostPath, you need to adapt the runtime distribution mechanism before using the Kubernetes backend.

## Configure secrets

Edit `deploy/kubernetes/config-secret.yaml` before applying:

- Change `admin.password`
- Change `auth.jwt_secret`
- Change the PostgreSQL password in both `config.toml` and the `memoh-postgres` Secret
- Set `kubernetes.pvc_storage_class` if your cluster has no default StorageClass
- Set `kubernetes.image_pull_secret` if needed

The important backend section is:

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

## Deploy

```bash
kubectl apply -k deploy/kubernetes
kubectl -n memoh rollout status deployment/memoh-server
kubectl -n memoh rollout status deployment/memoh-web
kubectl -n memoh rollout status daemonset/memoh-runtime-installer
```

For local access:

```bash
kubectl -n memoh port-forward svc/memoh-web 8082:8082
kubectl -n memoh port-forward svc/memoh-server 8080:8080
```

Then open `http://localhost:8082`.

For local clusters that can see locally built images, use the included overlay:

```bash
kubectl apply -k deploy/kubernetes-local
```

It rewrites `memohai/server` and `memohai/web` to the `k8s-dev` tag.

## Updating images

Patch images with kustomize:

```bash
kubectl kustomize deploy/kubernetes \
  | sed 's/memohai\/server:latest/memohai\/server:0.8.0/g' \
  | kubectl apply -f -
```

For a real environment, prefer a small overlay directory that pins images and storage settings explicitly.

## Notes and limitations

- The server Deployment overrides the server image entrypoint and runs `/app/memoh-server serve` directly. The Docker Compose entrypoint starts embedded containerd, which is not needed for the Kubernetes backend.
- The server Pod runs `/app/memoh-server migrate up` in an initContainer before starting the API server.
- The server probes use `GET /ping`; `/health` is intentionally registered as a HEAD endpoint.
- Workspace Pods use PVCs for `/data`; the PVC size comes from `kubernetes.pvc_size`.
- Snapshots require VolumeSnapshot CRDs and a working CSI snapshot controller.
- Provider-backed overlays such as Tailscale and NetBird may need extra RBAC depending on the provider configuration.
