# Bot Workspace Management

Every bot in Memoh works inside a workspace. In server deployments this is normally an isolated container, Pod, or VM-like runtime. In trusted desktop/local scenarios it may also be a local host directory. The workspace gives the bot a filesystem, command execution environment, MCP runtime, and optional graphical desktop.

## Concept: The Bot Workspace

The workspace acts as the bot's private computer. Within it, the bot can:

- store and modify files
- install software through package managers when the image allows it
- execute scripts and background tasks
- keep state across sessions
- optionally run a desktop display and headed browser

The underlying runtime is selected globally with `[container].backend` in `config.toml`, with trusted local workspace support controlled separately. The official Docker Compose server deploy uses `containerd`; Docker Engine, Apple, and local workspace modes are documented in [Workspace Backends](/installation/workspace-backends).

## Workspace tabs

Bot detail pages expose workspace-related settings across several tabs:

| Tab | Purpose |
|-----|---------|
| **Container** | Container lifecycle, snapshots, data export/import, and CDI device settings. |
| **Desktop** | Workspace display runtime, headed browser availability, live display sessions, and session cleanup. |
| **Network** | Workspace network and overlay provider status/actions. |
| **Tool Approval** | Approval settings for tools that need explicit human permission. |
| **Files** | Browse and edit the bot workspace filesystem. |
| **Terminal** | Open interactive shells inside the active workspace runtime. |

Some tabs are hidden or limited for trusted local workspaces when the feature only applies to container-backed runtimes.

## Container lifecycle

Manage the container-backed workspace from the **Container** tab.

- **Create**: Initialize the workspace container if it does not exist. Progress is shown through SSE during image pull and creation.
- **Start**: Launch the workspace runtime.
- **Stop**: Gracefully shut down the runtime to save resources.
- **Delete**: Remove the runtime instance.

Many workspace features, such as terminal access and container display, require the runtime to be running.

## Workspace display

The **Desktop** tab prepares and inspects the graphical workspace runtime. It checks for the desktop toolkit, Xvnc/VNC availability, browser availability, and active display sessions.

When enabled, the workspace can run a headed Chrome/Chromium browser inside the container. The Web UI display pane connects to that desktop session so you and the agent can operate the same visible browser. For the tool model, see [Browser / Computer Use](/getting-started/browser-computer-use).

## Container information

The **Container** tab displays runtime data such as:

- container ID and status
- image
- host and workspace paths
- active background tasks
- effective CDI devices, if configured

## Advanced: Provide CDI Devices

Memoh can provide host devices to a bot container through CDI (Container Device Interface). This is an advanced capability for users who want to expose host-managed devices, most commonly GPUs, to the container runtime.

In the Web UI, this capability is placed under **Advanced options** in the **Container** tab. It is optional and only needs to be configured when the bot must access CDI-backed devices from the host.

### Configure CDI Devices

1. Open the bot's **Container** tab.
2. Click **Create** if the container does not exist, or recreate the container if you need to change GPU settings.
3. Expand **Advanced options**.
4. Enable **GPU**.
5. Enter one or more CDI device names in **CDI devices**.

You can enter CDI device names one per line or separated with commas. Common GPU-related examples:

- `nvidia.com/gpu=0`
- `nvidia.com/gpu=all`
- `amd.com/gpu=0`
- `amd.com/gpu=all`

### Host Requirements

Before configuring CDI devices in Memoh, the host machine must already provide working device drivers, vendor toolkit support where required, and valid CDI specs. In practice, this usually means:

- the host GPU works normally outside the container
- CDI spec files exist under `/etc/cdi` or `/var/run/cdi`
- the device name you enter in Memoh matches a real CDI device on the host

To discover the exact CDI device names exposed by the host, use the vendor tool on the host machine:

- NVIDIA: `nvidia-ctk cdi list`
- AMD: `amd-ctk cdi list`

If Memoh reports an error such as `unresolvable CDI devices`, the configured device name does not match any CDI device visible to the container runtime.

### Important Behavior

- CDI device settings are applied when the container is created. Updating the setting later requires recreating the container.
- Stopping and starting an existing container does not change its attached CDI devices.
- The container image still needs the appropriate user-space libraries and tools if you want to run CUDA or ROCm software inside the container.
- After creation, the **Container** tab shows the effective attached CDI devices for verification.

## Snapshots

Snapshots allow you to capture the current state of the bot's container workspace and restore it later. This is useful for saving a known good configuration, versioning the runtime, or testing complex changes safely.

## Data Export and Import

The **Container** tab supports exporting and importing workspace data for backup, migration, or sharing purposes.

- **Export Data** packages the workspace filesystem data into a downloadable archive.
- **Import Data** extracts an uploaded archive into the workspace filesystem.
- **Restore** resets the data directory to a clean state when the filesystem has become corrupted or you want to start fresh without recreating the runtime.

## Versioning

Memoh tracks workspace/container versions to manage the lifecycle of the bot runtime environment. Version information helps with auditing and understanding when runtime configuration changed.
