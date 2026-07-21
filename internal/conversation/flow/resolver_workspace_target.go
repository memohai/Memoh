package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/workspace"
)

var ErrWorkspaceTargetACPUnsupported = errors.New("workspace_target_id is not supported for ACP sessions")

// ValidateWorkspaceTarget validates a user-selected Computer without changing
// the Bot's Primary target. It is used by handlers before creating a session.
func (r *Resolver) ValidateWorkspaceTarget(ctx context.Context, botID, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	if r == nil || r.workspaceTargets == nil {
		return errors.New("workspace target resolver not configured")
	}
	_, err := r.workspaceTargets.ResolveWorkspaceTarget(ctx, strings.TrimSpace(botID), targetID)
	return err
}

func (r *Resolver) prepareWorkspaceRequest(ctx context.Context, req conversation.ChatRequest) (context.Context, conversation.ChatRequest, error) {
	requestedTargetID := strings.TrimSpace(req.WorkspaceTargetID)
	if requestedTargetID != "" {
		if strings.TrimSpace(req.UserID) == "" {
			return ctx, req, errors.New("user id is required to select a computer")
		}
		if r == nil || r.botPermissions == nil {
			return ctx, req, errors.New("workspace target permission checker not configured")
		}
		allowed, err := r.botPermissions.HasBotPermission(ctx, req.BotID, req.UserID, bots.PermissionWorkspaceRead)
		if err != nil {
			return ctx, req, fmt.Errorf("check workspace target permission: %w", err)
		}
		if !allowed {
			return ctx, req, errors.New("workspace_read permission is required to select a computer")
		}
	}
	if r == nil || r.workspaceTargets == nil {
		if requestedTargetID != "" {
			return ctx, req, errors.New("workspace target resolver not configured")
		}
		return ctx, req, nil
	}
	resolved, err := r.workspaceTargets.ResolveWorkspaceTarget(ctx, req.BotID, requestedTargetID)
	if err != nil {
		return ctx, req, err
	}
	req.WorkspaceTargetID = strings.TrimSpace(resolved.TargetID)
	req.WorkspaceTarget = &conversation.WorkspaceTarget{
		TargetID: strings.TrimSpace(resolved.TargetID),
		Kind:     strings.TrimSpace(resolved.Kind),
		Name:     strings.TrimSpace(resolved.Name),
	}
	ctx = workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	return ctx, req, nil
}

func (r *Resolver) resolveWorkspaceTargetSnapshot(ctx context.Context, botID, targetID string) (*conversation.WorkspaceTarget, error) {
	if r == nil || r.workspaceTargets == nil {
		if strings.TrimSpace(targetID) == "" {
			return nil, nil
		}
		return nil, errors.New("workspace target resolver not configured")
	}
	resolved, err := r.workspaceTargets.ResolveWorkspaceTarget(ctx, botID, targetID)
	if err != nil {
		return nil, err
	}
	return &conversation.WorkspaceTarget{
		TargetID: strings.TrimSpace(resolved.TargetID),
		Kind:     strings.TrimSpace(resolved.Kind),
		Name:     strings.TrimSpace(resolved.Name),
	}, nil
}

func workspaceTargetFromRunConfig(cfg agentpkg.RunConfig) *conversation.WorkspaceTarget {
	if strings.TrimSpace(cfg.Identity.WorkspaceTargetID) == "" {
		return nil
	}
	return &conversation.WorkspaceTarget{
		TargetID: strings.TrimSpace(cfg.Identity.WorkspaceTargetID),
		Kind:     strings.TrimSpace(cfg.Identity.WorkspaceTargetKind),
		Name:     strings.TrimSpace(cfg.Identity.WorkspaceTargetName),
	}
}

func rejectACPWorkspaceTarget(req conversation.ChatRequest) error {
	if strings.TrimSpace(req.WorkspaceTargetID) == "" {
		return nil
	}
	return ErrWorkspaceTargetACPUnsupported
}
