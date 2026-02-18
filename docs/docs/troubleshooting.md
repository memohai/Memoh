# Troubleshooting

## MCP Container: `no running task found: task mcp-xxx not found`

### Symptom

When a bot tries to use container tools (e.g., execute commands), the server logs show:

```
level=WARN msg="exec failed" provider=container_tool bot_id=xxx command=date error="no running task found: task mcp-xxx not found"
```

The containerd container logs may also show:

```
level=error msg="failed to delete task" error="rpc error: code = NotFound desc = container not created: not found"
```

### Cause

The `[mcp] data_root` in `config.toml` is set to a **host machine path** (e.g., `/Users/you/Code/Memoh/data`), but the server and containerd containers use a Docker named volume mounted at `/opt/memoh/data`.

When the server creates an MCP container inside containerd, it uses `data_root` as the mount source. Since this host path does not exist inside the containerd container, `runc` fails with:

```
failed to fulfil mount request: open /Users/you/Code/Memoh/data/bots/xxx: no such file or directory
```

### Solution

1. Set `data_root` to the in-container path in your config:

```toml
[mcp]
data_root = "/opt/memoh/data"
```

2. Clean up the stale containerd container (if it exists):

```bash
docker exec memoh-containerd ctr -n default containers rm mcp-<bot-id>
```

3. Restart the server:

```bash
docker compose restart server
```

> **Note**: If you also run the server locally (outside Docker), keep the Docker config (`conf/app.docker.toml`) separate from your local `config.toml`, and update `docker-compose.yml` to mount the Docker-specific config instead.

## MCP Container: Image update not taking effect after rebuild

### Symptom

After updating `Dockerfile.containerd` (e.g., adding Node.js/Python to the MCP image), rebuilding and restarting the containerd container, MCP tools still fail with errors like:

```
exec: "npx": executable file not found in $PATH
```

### Cause

The containerd entrypoint script (`containerd-entrypoint.sh`) skips image import if the image already exists in containerd's image store:

```sh
if ! ctr -n default images check "name==${MCP_IMAGE}" ...; then
  # import
fi
```

Since `containerd_data` is a persistent Docker volume, the old MCP image survives across container restarts. The new image embedded in the rebuilt Docker image is never imported.

### Solution

1. Remove the old MCP image from containerd:

```bash
docker exec memoh-containerd ctr -n default images rm docker.io/library/memoh-mcp:latest
```

2. Restart the containerd container to trigger re-import:

```bash
docker compose restart containerd
```

3. Verify the new image was imported (size should be significantly larger if Node.js/Python were added):

```bash
docker exec memoh-containerd ctr -n default images ls
```

4. Delete the bot's MCP container and recreate it from the bot detail page so it uses the new image.
