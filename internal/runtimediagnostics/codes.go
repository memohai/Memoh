package runtimediagnostics

type DiagnosticCodeDefinition struct {
	Code        string
	Scope       string
	Severity    string
	Description string
	NextAction  string
}

func DiagnosticCodeCatalog() []DiagnosticCodeDefinition {
	return []DiagnosticCodeDefinition{
		{Code: "workspace_unknown", Scope: "workspace", Severity: "unknown", Description: "Workspace state has not been resolved yet."},
		{Code: "workspace_manager_missing", Scope: "workspace", Severity: "error", Description: "Server workspace manager is not configured."},
		{Code: "workspace_info_failed", Scope: "workspace", Severity: "error", Description: "Workspace backend information could not be read."},
		{Code: "workspace_local", Scope: "workspace", Severity: "ok", Description: "The bot uses a trusted local workspace; bridge probes stay read-only."},
		{Code: "workspace_reachable", Scope: "workspace", Severity: "ok", Description: "Workspace bridge and MCP client are reachable."},
		{Code: "bridge_unreachable", Scope: "workspace", Severity: "warn", Description: "Workspace bridge or MCP client is not reachable.", NextAction: "Start or repair the workspace runtime from settings."},
		{Code: "container_unknown", Scope: "container", Severity: "unknown", Description: "Container state has not been resolved yet."},
		{Code: "container_manager_missing", Scope: "container", Severity: "unknown", Description: "Container manager is not configured."},
		{Code: "runtime_cold_start_required", Scope: "container/acp", Severity: "warn", Description: "Runtime/session exists, but a warm process is not available.", NextAction: "Start the runtime from the relevant settings surface when needed."},
		{Code: "container_status_failed", Scope: "container", Severity: "error", Description: "Container status lookup failed."},
		{Code: "container_running", Scope: "container", Severity: "ok", Description: "Workspace container task is running."},
		{Code: "container_stopped", Scope: "container", Severity: "warn", Description: "Workspace container exists but is stopped."},
		{Code: "container_setup_failed", Scope: "container", Severity: "error", Description: "Generic container setup failure event."},
		{Code: "container_image_prepare_failed", Scope: "container", Severity: "error", Description: "Workspace image preparation failed."},
		{Code: "container_start_failed", Scope: "container", Severity: "error", Description: "Container start failed."},
		{Code: "container_stop_failed", Scope: "container", Severity: "error", Description: "Container stop failed."},
		{Code: "container_not_found", Scope: "container", Severity: "error", Description: "Container action targeted a missing container record."},
		{Code: "display_unknown", Scope: "display", Severity: "unknown", Description: "Display state has not been resolved yet."},
		{Code: "display_manager_missing", Scope: "display", Severity: "unknown", Description: "Display service is not configured."},
		{Code: "display_disabled", Scope: "display", Severity: "disabled", Description: "Workspace display is disabled."},
		{Code: "display_available", Scope: "display", Severity: "ok", Description: "Workspace display is enabled and reachable."},
		{Code: "display_unavailable", Scope: "display", Severity: "warn", Description: "Workspace display is enabled but unavailable."},
		{Code: "display_prepare_failed", Scope: "display", Severity: "error", Description: "Display prepare request failed."},
		{Code: "acp_agent_disabled", Scope: "acp", Severity: "disabled", Description: "ACP provider is disabled for this bot."},
		{Code: "unsupported_backend", Scope: "acp", Severity: "error", Description: "ACP profile does not support the current workspace backend."},
		{Code: "workspace_bridge_unreachable", Scope: "acp", Severity: "warn", Description: "ACP provider cannot inspect the workspace bridge."},
		{Code: "cli_missing", Scope: "acp", Severity: "error", Description: "Provider CLI command is not available."},
		{Code: "auth_missing", Scope: "acp", Severity: "error", Description: "Required provider authentication is missing."},
		{Code: "auth_file_permissive", Scope: "acp", Severity: "warn", Description: "Local provider auth file exists but has group/other permissions."},
		{Code: "warm_resumable", Scope: "acp", Severity: "ok", Description: "A matching ACP runtime is warm and can resume."},
		{Code: "no_acp_session", Scope: "acp", Severity: "unknown", Description: "No ACP session exists yet for this provider."},
		{Code: "bind_warm_runtime_failed", Scope: "acp", Severity: "error", Description: "Binding an existing warm runtime failed."},
		{Code: "model_set_failed", Scope: "acp", Severity: "error", Description: "Setting the ACP runtime model failed."},
		{Code: "upstream_adapter_failed", Scope: "acp", Severity: "error", Description: "Upstream ACP adapter returned a runtime error."},
		{Code: "prompt_config_failed", Scope: "acp", Severity: "error", Description: "ACP prompt startup configuration failed."},
		{Code: "runtime_start_failed", Scope: "acp", Severity: "error", Description: "ACP runtime process start failed."},
		{Code: "runtime_diagnostic_event", Scope: "acp", Severity: "error", Description: "Fallback code for normalized diagnostic events."},
	}
}
