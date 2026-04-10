# Bot Container Management

Every Bot in Memoh operates within its own isolated container environment. This isolation ensures security, provides a dedicated filesystem, and allows the bot to execute code or commands without affecting other bots or the host system.

## Concept: The Isolated Workspace

The container acts as the bot's private "computer." Within it, the bot can:
- Store and modify files
- Install software via package managers
- Execute scripts
- Maintain state across multiple sessions

---

## Operations

Manage the lifecycle of your bot's environment from the **Container** tab in the Bot Detail page.

### Lifecycle Actions

- **Create**: Initialize the container if it doesn't exist (using the configured image). Progress is shown via real-time SSE feedback during image pull and creation.
- **Start**: Launch the container. The bot must have a running container to perform many operations like file editing or executing tools.
- **Stop**: Gracefully shut down the container to save resources.
- **Delete**: Remove the container instance.

---

## Container Information

The **Container** tab displays real-time data about the bot's runtime:
- **Container ID**: Unique identifier for the instance.
- **Status**: Whether it's currently running, stopped, or creating.
- **Image**: The Docker/Containerd image used as the base.
- **Paths**: Host and container paths for data persistence.
- **Tasks**: Number of active background tasks running in the container.
- **CDI Devices**: The effective GPU CDI devices currently attached to the container, if any.

---

## Advanced: Provide CDI Devices

Memoh can provide host devices to a bot container through CDI (Container Device Interface). This is an advanced capability for users who want to expose host-managed devices, most commonly GPUs, to the container runtime.

In the Web UI, this capability is placed under **Advanced options** in the **Container** tab. It is optional and only needs to be configured when the bot must access CDI-backed devices from the host.

### Configure CDI Devices

1. Open the Bot's **Container** tab.
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

---

## Snapshots

Snapshots allow you to capture the current state of the bot's container and restore it later. This is useful for:
- Saving a known good configuration
- Versioning the bot's environment
- Testing complex changes safely

### Creating a Snapshot
1. Ensure the container is stopped or in a stable state.
2. Click **Create Snapshot**.
3. Provide a name for the snapshot.

### Restoring a Snapshot
- Find the desired snapshot in the list and click **Restore**. This will reset the container to the captured state.

### Managing Snapshots
- View a list of existing snapshots with their creation timestamps and parent relationships.
- Use the **Delete** button next to a snapshot to remove it.

---

## Data Export and Import

The Container tab supports exporting and importing container data for backup, migration, or sharing purposes.

### Export

1. Click **Export Data**.
2. The container's filesystem data is packaged into a downloadable archive.
3. Save the archive to your local machine.

### Import

1. Click **Import Data**.
2. Select an archive file from your local machine.
3. The archive contents are extracted into the container's filesystem.

### Restore

The **Restore** operation resets the container's data directory to a clean state. This is useful when the filesystem has become corrupted or you want to start fresh without recreating the container.

---

## Container Versioning

Memoh tracks container versions to manage the lifecycle of the bot's runtime environment. Version information includes:

- **Current Version**: The active container version.
- **Version History**: A log of container version changes over time.

This helps with auditing and understanding when container configurations were updated.
