# Workspace Templates

`workspace/` is the canonical set of files seeded into every agent workspace
on first boot:

- `AGENTS.md`, `MEMORY.md`, `PROFILES.md`, `HEARTBEAT.md` — bot persona and
  memory scaffolding (not developer guides).
- `.memoh/` — default hooks configuration and built-in skills
  (`skill-creator`, `hooks-setup`).

Consumers:

- **Bridge runtime**: `docker/Dockerfile.server` copies `workspace/` into the
  toolkit assembly; the bridge seeds it into `/data` on first boot
  (`cmd/bridge/main.go`). The devenv compose files bind-mount it at
  `/opt/memoh/runtime/templates`.
- **Go importers**: `templates.WorkspaceFS()` exposes the same tree as an
  embedded `fs.FS` for downstream distributions that provision workspaces
  themselves.

When seeding, skip `.gitkeep` placeholders and never overwrite existing files.

> `internal/workspace/templates/` holds a legacy copy of the four markdown
> files for the deprecated local (non-container) workspace path; a test keeps
> it in sync with this directory.
