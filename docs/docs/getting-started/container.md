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

- **Create**: Initialize the container if it doesn't exist (using the configured image).
- **Start**: Launch the container. The bot must have a running container to perform many operations like file editing or executing tools.
- **Stop**: Gracefully shut down the container to save resources.
- **Delete**: Remove the container instance. This will delete the temporary state but preserve the data in persistent volumes.

---

## Container Information

The **Container** tab displays real-time data about the bot's runtime:
- **Container ID**: Unique identifier for the instance.
- **Status**: Whether it's currently running, stopped, or creating.
- **Image**: The Docker/Containerd image used as the base.
- **Paths**: Host and container paths for data persistence.
- **Tasks**: Number of active background tasks running in the container.

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
