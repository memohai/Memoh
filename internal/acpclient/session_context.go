package acpclient

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const HermesContainerHome = dataMountPath + "/.memoh-hermes"

type SessionContextInput struct {
	AgentID       string
	SetupMode     SetupMode
	BotID         string
	Backend       string
	WorkspaceRoot string
	ProjectPath   string
	LocalDataRoot string
}

type ResolvedSessionContext struct {
	AgentID       string
	SetupMode     SetupMode
	Backend       WorkspaceBackend
	WorkspaceRoot string
	ProjectPath   string
	CWD           string
	HermesHome    string
}

func ResolveSessionContext(input SessionContextInput) (ResolvedSessionContext, error) {
	var backend WorkspaceBackend
	switch strings.ToLower(strings.TrimSpace(input.Backend)) {
	case "", bridge.WorkspaceBackendContainer:
		backend = WorkspaceBackendContainer
	case bridge.WorkspaceBackendLocal:
		backend = WorkspaceBackendLocal
	default:
		return ResolvedSessionContext{}, fmt.Errorf("unsupported workspace backend %q", input.Backend)
	}
	root := strings.TrimSpace(input.WorkspaceRoot)
	if root == "" {
		root = dataMountPath
	}

	var resolvedRoot string
	var projectPath string
	var err error
	if backend == WorkspaceBackendLocal {
		resolvedRoot, err = resolveRoot(root)
		if err != nil {
			return ResolvedSessionContext{}, err
		}
		projectPath, err = ResolvePathUnderRoot(resolvedRoot, input.ProjectPath)
	} else {
		resolvedRoot = dataMountPath
		projectPath, err = ResolvePathUnderVirtualRoot(resolvedRoot, input.ProjectPath)
	}
	if err != nil {
		return ResolvedSessionContext{}, err
	}

	ctx := ResolvedSessionContext{
		AgentID:       strings.TrimSpace(input.AgentID),
		SetupMode:     normalizeSetupMode(input.SetupMode),
		Backend:       backend,
		WorkspaceRoot: resolvedRoot,
		ProjectPath:   projectPath,
		CWD:           projectPath,
	}
	if isHermesAgent(input.AgentID) && ctx.SetupMode != SetupModeSelf {
		home, err := resolveHermesHome(ctx.Backend, input.BotID, input.LocalDataRoot)
		if err != nil {
			return ResolvedSessionContext{}, err
		}
		ctx.HermesHome = home
	}
	return ctx, nil
}

func resolveHermesHome(backend WorkspaceBackend, botID, localDataRoot string) (string, error) {
	switch backend {
	case WorkspaceBackendContainer:
		return HermesContainerHome, nil
	case WorkspaceBackendLocal:
		root := strings.TrimSpace(localDataRoot)
		if root == "" {
			return "", errors.New("local Hermes managed setup requires local data root for HERMES_HOME isolation")
		}
		botID = strings.TrimSpace(botID)
		if botID == "" {
			return "", errors.New("local Hermes managed setup requires bot id")
		}
		return filepath.Join(root, "acp", "hermes", safePathSegment(botID)), nil
	default:
		return "", fmt.Errorf("unsupported workspace backend %q for Hermes managed setup", backend)
	}
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\x00", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		return "unknown"
	}
	return value
}

func resolveWorkspacePaths(info bridge.WorkspaceInfo, rawProjectPath string) (string, string, WorkspaceBackend, error) {
	ctx, err := ResolveSessionContext(SessionContextInput{
		Backend:       info.Backend,
		WorkspaceRoot: info.DefaultWorkDir,
		ProjectPath:   rawProjectPath,
		LocalDataRoot: info.LocalDataRoot,
	})
	if err != nil {
		return "", "", WorkspaceBackendContainer, err
	}
	return ctx.WorkspaceRoot, ctx.ProjectPath, ctx.Backend, nil
}

func resolvedHermesHome(ctx *ResolvedSessionContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.HermesHome)
}
