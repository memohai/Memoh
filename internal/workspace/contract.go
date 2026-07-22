package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	WorkspaceContractPath           = "/opt/memoh/workspace-contract.json"
	CurrentWorkspaceContractVersion = 1
	WorkspaceToolkitDir             = "/opt/memoh/toolkit"
	WorkspaceScriptsDir             = "/opt/memoh/scripts"
)

var ErrWorkspaceImageIncompatible = errors.New("workspace image is incompatible")

type WorkspaceContract struct {
	ContractVersion int                       `json:"contract_version"`
	Platform        WorkspaceContractPlatform `json:"platform"`
	Paths           WorkspaceContractPaths    `json:"paths"`
}

type WorkspaceContractPlatform struct {
	OS   string `json:"os"`
	Libc string `json:"libc"`
}

type WorkspaceContractPaths struct {
	Toolkit string `json:"toolkit"`
	Scripts string `json:"scripts"`
}

var requiredWorkspaceExecutables = []string{
	WorkspaceToolkitDir + "/bin/node",
	WorkspaceToolkitDir + "/bin/python3",
	WorkspaceToolkitDir + "/bin/uv",
	WorkspaceToolkitDir + "/bin/codex",
	WorkspaceToolkitDir + "/bin/codex-acp",
	WorkspaceToolkitDir + "/bin/claude-agent-acp",
	WorkspaceToolkitDir + "/bin/hermes-acp",
	WorkspaceToolkitDir + "/display/bin/a11y-cli",
	WorkspaceScriptsDir + "/display-prepare.sh",
	WorkspaceScriptsDir + "/display-apply-style.sh",
	WorkspaceScriptsDir + "/desktop-style.sh",
}

func validateWorkspaceContract(ctx context.Context, client *bridge.Client) error {
	if client == nil {
		return fmt.Errorf("%w: bridge client is unavailable", ErrWorkspaceImageIncompatible)
	}
	reader, err := client.ReadRaw(ctx, WorkspaceContractPath)
	if err != nil {
		return fmt.Errorf("%w: read contract: %w", ErrWorkspaceImageIncompatible, err)
	}
	defer func() { _ = reader.Close() }()

	payload, err := io.ReadAll(io.LimitReader(reader, 64*1024))
	if err != nil {
		return fmt.Errorf("%w: read contract payload: %w", ErrWorkspaceImageIncompatible, err)
	}
	if err := validateWorkspaceContractPayload(payload); err != nil {
		return err
	}

	result, err := client.Exec(ctx, workspaceExecutableCheckCommand(), "/", 30)
	if err != nil {
		return fmt.Errorf("%w: validate runtime executables: %w", ErrWorkspaceImageIncompatible, err)
	}
	if result == nil || result.ExitCode != 0 {
		return fmt.Errorf("%w: one or more runtime executables are missing", ErrWorkspaceImageIncompatible)
	}
	return nil
}

func validateWorkspaceContractPayload(payload []byte) error {
	var contract WorkspaceContract
	if err := json.Unmarshal(payload, &contract); err != nil {
		return fmt.Errorf("%w: decode contract: %w", ErrWorkspaceImageIncompatible, err)
	}
	if contract.ContractVersion != CurrentWorkspaceContractVersion {
		return fmt.Errorf(
			"%w: contract version %d, want %d",
			ErrWorkspaceImageIncompatible,
			contract.ContractVersion,
			CurrentWorkspaceContractVersion,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(contract.Platform.OS), "linux") {
		return fmt.Errorf("%w: unsupported operating system %q", ErrWorkspaceImageIncompatible, contract.Platform.OS)
	}
	if !strings.EqualFold(strings.TrimSpace(contract.Platform.Libc), "glibc") {
		return fmt.Errorf("%w: unsupported libc %q", ErrWorkspaceImageIncompatible, contract.Platform.Libc)
	}
	if strings.TrimSpace(contract.Paths.Toolkit) != WorkspaceToolkitDir {
		return fmt.Errorf("%w: toolkit path %q", ErrWorkspaceImageIncompatible, contract.Paths.Toolkit)
	}
	if strings.TrimSpace(contract.Paths.Scripts) != WorkspaceScriptsDir {
		return fmt.Errorf("%w: scripts path %q", ErrWorkspaceImageIncompatible, contract.Paths.Scripts)
	}
	return nil
}

func workspaceExecutableCheckCommand() string {
	return "test -x " + strings.Join(requiredWorkspaceExecutables, " -a -x ")
}
