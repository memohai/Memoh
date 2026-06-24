package acpclient

import (
	"context"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

type CommandDiagnosticRequest struct {
	Command string           `json:"command"`
	WorkDir string           `json:"work_dir,omitempty"`
	Env     []string         `json:"env,omitempty"`
	Backend WorkspaceBackend `json:"backend,omitempty"`
}

type CommandDiagnostic struct {
	ConfiguredCommand string   `json:"configured_command"`
	ResolvedCommand   string   `json:"resolved_command,omitempty"`
	Source            string   `json:"source,omitempty"`
	Available         bool     `json:"available"`
	Error             string   `json:"error,omitempty"`
	Checks            []string `json:"checks,omitempty"`
}

func ResolveCommandDiagnostic(ctx context.Context, client *bridge.Client, req CommandDiagnosticRequest) CommandDiagnostic {
	command := strings.TrimSpace(req.Command)
	diag := CommandDiagnostic{
		ConfiguredCommand: command,
		Checks:            []string{},
	}
	if client == nil {
		diag.Error = "workspace bridge client is required"
		return diag
	}
	if command == "" {
		diag.Error = "ACP command is required"
		return diag
	}
	if !isPlainCommand(command) {
		diag.Available = true
		diag.ResolvedCommand = command
		diag.Source = "raw_command"
		return diag
	}
	if strings.Contains(command, "/") {
		diag.Source = "absolute_path"
		check := "test -x " + escapeShellArg(command)
		diag.Checks = append(diag.Checks, check)
		result, err := checkCommand(ctx, client, check, req.WorkDir, req.Env)
		if err != nil {
			diag.Error = err.Error()
			return diag
		}
		if result.ExitCode == 0 {
			diag.Available = true
			diag.ResolvedCommand = command
			return diag
		}
		diag.Error = commandNotAvailableError(command, result, req.Backend).Error()
		return diag
	}

	pathCheck := "command -v " + escapeShellArg(command) + " >/dev/null 2>&1"
	diag.Checks = append(diag.Checks, pathCheck)
	result, err := checkCommand(ctx, client, pathCheck, req.WorkDir, req.Env)
	if err != nil {
		diag.Error = err.Error()
		return diag
	}
	if result.ExitCode == 0 {
		diag.Available = true
		diag.ResolvedCommand = command
		diag.Source = "path"
		return diag
	}
	lastResult := result

	if req.Backend == WorkspaceBackendContainer {
		toolkitCommand := containerToolkitBin + "/" + command
		toolkitCheck := "test -x " + escapeShellArg(toolkitCommand)
		diag.Checks = append(diag.Checks, toolkitCheck)
		toolkitResult, err := checkCommand(ctx, client, toolkitCheck, req.WorkDir, req.Env)
		if err != nil {
			diag.Error = err.Error()
			return diag
		}
		lastResult = toolkitResult
		if toolkitResult.ExitCode == 0 {
			diag.Available = true
			diag.ResolvedCommand = toolkitCommand
			diag.Source = "container_toolkit"
			return diag
		}
	}

	diag.Source = "path"
	diag.Error = commandNotAvailableError(command, lastResult, req.Backend).Error()
	return diag
}
