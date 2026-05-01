# 容器

每个机器人在**自己的**容器里跑，文件系统、网络边界都隔开，互不影响，也跟宿主机隔离。

## 是什么

可以把它想成机器人私用的一台小电脑：能存文件、装包、跑脚本、跨会话留状态。

---

## 操作

在 **Container** tab。

底层 runtime 由 `config.toml` 的 `[container].backend` 决定。官方 Docker Compose 栈使用 `containerd`；宿主机 Docker、Kubernetes、Apple 后端也可用于手动部署。改这个值前建议先看 [Workspace backend](/zh/installation/workspace-backends.md)。

### 生命周期

- **Create**：没有就按镜像建；拉镜像、建实例时会有 SSE 进度。
- **Start**：要跑文件工具、终端等，多半要先起来。
- **Stop**：省资源，优雅停。
- **Delete**：删实例（数据行为视版本与设置而定，以界面为准）。

---

## 信息

会显示如：容器 id、状态、用的镜像、宿主机/容器路径、后台任务数、若配了 **CDI 设备**（常见 GPU）也会列出来。

---

## 进阶：CDI 设备

要把宿主机通过 **CDI**（常见是 GPU）透进容器，在 **Container** → **Advanced** 里配。一般只有确实要在里面跑 CUDA/ROCm 等才要动。

### 配法

1. 打开 **Container**。
2. 没有容器先 **Create**；要改 GPU 类设置往往要**重建**容器。
3. 展开 **Advanced**。
4. 开 **GPU**，在 **CDI devices** 里写设备名。

可每行一个或逗号分隔，例如：

- `nvidia.com/gpu=0`
- `nvidia.com/gpu=all`
- `amd.com/gpu=0`
- `amd.com/gpu=all`

### 宿主要求

宿主机上驱动、厂商工具、CDI spec 要已就绪。通常意味着：

- 宿主机上 GPU 本来就能用
- `/etc/cdi` 或 `/var/run/cdi` 里有 spec
- 你填的名字和运行时看见的一致

查本机名：

- NVIDIA：`nvidia-ctk cdi list`
- AMD：`amd-ctk cdi list`

若报 `unresolvable CDI devices`，多半是名字对不上。

### 注意

- CDI 在**创建**时生效，改配置后常要**重建**容器；只停再起**不会**换已挂设备。
- 镜像里仍要装对的用户态库，才能真跑算子。
- 建好后 **Container** tab 会显示当前挂上的设备，便于核对。

---

## 快照

**Create Snapshot** 把当前环境状态勾下来，方便回滚、试大改。  
**Restore** 按某个快照回退。可删不要的快照。

---

## 导入导出

- **Export Data**：把容器内数据打成包下载。
- **Import Data**：从本地上传打进文件系统。

### Restore（数据侧）

**Restore** 在数据目录侧做「清到干净再灌」，适合盘坏了或想从零来而又不删容器实例时，以界面说明为准。

---

## 版本

会跟 **Current Version**、**Version History** 等，方便审计环境何时、因何变过。
