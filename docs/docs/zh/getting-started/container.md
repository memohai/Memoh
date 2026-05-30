# Workspace 与容器

每个机器人都在一个 workspace 里工作。Server Deploy 里通常是隔离容器、Pod 或类似 VM 的 runtime；在受信任的 Desktop/local 场景里，也可以是宿主机上的本地目录。workspace 提供文件系统、命令执行环境、MCP runtime，以及可选的图形桌面。

## 是什么

可以把 workspace 想成机器人私用的一台小电脑。它能：

- 存文件、改文件
- 在镜像允许时装包
- 跑脚本和后台任务
- 跨会话保留状态
- 可选地运行桌面显示和有头浏览器

底层容器 runtime 由 `config.toml` 的 `[container].backend` 决定，trusted local workspace 另行控制。官方 Docker Compose Server Deploy 使用 `containerd`；Docker Engine、Apple 和 local workspace 的差异见 [Workspace backend](/zh/installation/workspace-backends)。

## Workspace 相关 tab

机器人详情页里有几组和 workspace 相关的 tab：

| Tab | 内容 |
|-----|------|
| **Container** | 容器生命周期、快照、导入导出、CDI 设备。 |
| **Desktop** | Workspace display runtime、有头浏览器可用性、实时 display session、关闭会话。 |
| **Network** | Workspace 网络与 overlay provider 状态/动作。 |
| **Tool Approval** | 需要人类确认的工具审批设置。 |
| **Files** | 浏览和编辑机器人 workspace 文件系统。 |
| **Terminal** | 在当前 workspace runtime 里打开交互 shell。 |

如果机器人使用 trusted local workspace，一些只适用于容器 runtime 的 tab 或按钮可能会隐藏或不可用。

## 容器生命周期

在 **Container** tab 管容器型 workspace：

- **Create**：没有就按镜像建；拉镜像、建实例时会有 SSE 进度。
- **Start**：启动 workspace runtime。
- **Stop**：省资源，优雅停。
- **Delete**：删除 runtime 实例。

终端、容器 display 等很多 workspace 能力都需要 runtime 正在运行。

## Workspace display

**Desktop** tab 用来准备和检查图形 workspace runtime。它会检查 desktop toolkit、Xvnc/VNC、浏览器和当前 display session。

启用后，workspace 可以在容器里跑有头 Chrome/Chromium。网页端 Display pane 会连接到同一个桌面会话，你和 agent 看到、操作的是同一个可见浏览器。工具层面的区别见 [Browser / Computer Use](/zh/getting-started/browser-computer-use)。

## 运行时信息

**Container** tab 会显示：

- container id 与状态
- 镜像
- 宿主机和 workspace 路径
- 后台任务数
- 若配置了 CDI 设备，也会列出实际挂载的设备

## 进阶：CDI 设备

要把宿主机通过 **CDI**（常见是 GPU）透进容器，在 **Container** -> **Advanced** 里配。一般只有确实要在里面跑 CUDA/ROCm 等才要动。

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

## 快照

**Create Snapshot** 会记录当前容器 workspace 状态，方便回滚、版本化 runtime 或试大改。**Restore** 按某个快照回退，可删不要的快照。

## 导入导出

**Container** tab 支持导入导出 workspace 数据：

- **Export Data**：把 workspace 文件系统数据打成包下载。
- **Import Data**：从本地上传归档并解进 workspace 文件系统。
- **Restore**：在数据目录侧做“清到干净再灌”，适合盘坏了或想从零来而又不删 runtime 实例时。

## 版本

Memoh 会跟踪 workspace/container 版本，帮助审计 runtime 环境何时、因何变过。
