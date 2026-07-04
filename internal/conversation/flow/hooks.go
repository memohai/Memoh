package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func (r *Resolver) applyUserMessageHook(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	res, err := r.runChatHook(ctx, req, hooks.EventUserMessageReceived, func(hreq *hooks.Request) {
		hreq.Channel = map[string]any{
			"platform":          strings.TrimSpace(req.CurrentChannel),
			"conversation_type": strings.TrimSpace(req.ConversationType),
			"reply_target":      strings.TrimSpace(req.ReplyTarget),
		}
		hreq.Extra = map[string]any{
			"query":               strings.TrimSpace(req.Query),
			"raw_query":           strings.TrimSpace(req.RawQuery),
			"attachment_count":    len(req.Attachments),
			"external_message_id": strings.TrimSpace(req.ExternalMessageID),
		}
	})
	if err != nil {
		if errors.Is(err, hooks.ErrDenied) || res.Decision == hooks.DecisionDeny {
			return req, fmt.Errorf("%w: %s", hooks.ErrDenied, firstHookText(res.Reason, err.Error()))
		}
		return req, err
	}
	if strings.TrimSpace(res.AppendContext) != "" {
		req.Messages = append(req.Messages, conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(formatResolverHookContext(hooks.EventUserMessageReceived, res.AppendContext)),
		})
	}
	// The reserved-skill-metadata guard runs at the resolver entrypoints
	// BEFORE this hook, so hook-modified messages must be re-checked here.
	// Today the only mutation is the metadata-less text append above, but the
	// guard is a security boundary (reserved keys are trusted downstream as
	// server-authored state) and must hold for any future hook capability
	// that can shape message content or metadata.
	if err := rejectReservedSkillMetadataIfPresent(req); err != nil {
		return req, err
	}
	return req, nil
}

func (r *Resolver) runPromptHook(ctx context.Context, cfg agentRunConfigView, eventName string) string {
	res, err := r.runBaseHook(ctx, cfg.BotID, cfg.SessionID, cfg.ChatID, eventName, func(req *hooks.Request) {
		req.Turn = map[string]any{
			"session_type":  cfg.SessionType,
			"message_count": cfg.MessageCount,
			"system_bytes":  cfg.SystemBytes,
		}
	})
	if err != nil {
		r.logHookWarn(eventName, cfg.BotID, cfg.SessionID, err)
		return ""
	}
	return strings.TrimSpace(res.AppendContext)
}

type agentRunConfigView struct {
	BotID        string
	SessionID    string
	ChatID       string
	SessionType  string
	MessageCount int
	SystemBytes  int
}

func (r *Resolver) runChatHook(ctx context.Context, chatReq conversation.ChatRequest, eventName string, enrich func(*hooks.Request)) (hooks.Result, error) {
	return r.runBaseHook(ctx, chatReq.BotID, chatReq.SessionID, chatReq.ChatID, eventName, enrich)
}

func (r *Resolver) runBaseHook(ctx context.Context, botID, sessionID, chatID, eventName string, enrich func(*hooks.Request)) (hooks.Result, error) {
	if r == nil || r.hookService == nil {
		return hooks.Result{Decision: hooks.DecisionAllow, RuntimeSupported: hooks.RuntimeSupported(eventName)}, nil
	}
	req := hooks.Request{
		Version:   1,
		Event:     eventName,
		BotID:     strings.TrimSpace(botID),
		SessionID: strings.TrimSpace(sessionID),
		ChatID:    strings.TrimSpace(chatID),
		Workspace: r.hookWorkspace(ctx, botID),
	}
	if enrich != nil {
		enrich(&req)
	}
	return r.hookService.Run(ctx, req, nil)
}

func (r *Resolver) hookWorkspace(ctx context.Context, botID string) hooks.WorkspaceInfo {
	info := hooks.WorkspaceInfo{
		CWD:     hooks.DefaultWorkDir,
		Runtime: bridge.WorkspaceBackendContainer,
	}
	if r == nil || r.agent == nil {
		return info
	}
	provider, ok := r.agent.BridgeProvider().(bridge.WorkspaceInfoProvider)
	if !ok {
		return info
	}
	workspaceInfo, err := provider.WorkspaceInfo(ctx, botID)
	if err != nil {
		return info
	}
	if strings.TrimSpace(workspaceInfo.DefaultWorkDir) != "" {
		info.CWD = workspaceInfo.DefaultWorkDir
	}
	if strings.TrimSpace(workspaceInfo.Backend) != "" {
		info.Runtime = workspaceInfo.Backend
	}
	return info
}

func (r *Resolver) logHookWarn(eventName, botID, sessionID string, err error) {
	if r == nil || r.logger == nil || err == nil {
		return
	}
	r.logger.Warn("hook failed",
		slog.String("event", eventName),
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
		slog.Any("error", err),
	)
}

func formatResolverHookContext(eventName, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "[Hook Context: " + strings.TrimSpace(eventName) + "]\n" + text
}

func firstHookText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
