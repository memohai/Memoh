# Runtime Diagnostics Codes

Runtime Health uses stable machine-readable `code` values so UI copy can be localized without parsing backend English text.

Key scopes:

- `workspace`: workspace backend, workdir, bridge, and MCP reachability.
- `container`: isolated runtime existence, task state, metrics, and setup failures.
- `display`: desktop/browser/display availability and prepare failures.
- `acp`: provider CLI, auth, profile/model, session resume, and ACP runtime failures.

The source-of-truth catalog lives in `internal/runtimediagnostics/codes.go` as `DiagnosticCodeCatalog()`. Add new codes there when introducing a new diagnostic state or persisted runtime event.
