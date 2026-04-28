package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/toolapproval"
)

type ToolApprovalResponseInput struct {
	BotID                  string
	SessionID              string
	ActorChannelIdentityID string
	ApprovalID             string
	ExplicitID             string
	ReplyExternalMessageID string
	Decision               string
	Reason                 string
	ChatToken              string
}

func (r *Resolver) RespondToolApproval(ctx context.Context, input ToolApprovalResponseInput, eventCh chan<- WSStreamEvent) error {
	if r.toolApproval == nil {
		return errors.New("tool approval service not configured")
	}
	target, err := r.toolApproval.ResolveTarget(ctx, toolapproval.ResolveInput{
		BotID:                  input.BotID,
		SessionID:              input.SessionID,
		ExplicitID:             firstNonEmpty(input.ExplicitID, input.ApprovalID),
		ReplyExternalMessageID: input.ReplyExternalMessageID,
	})
	if err != nil {
		return err
	}

	var toolResult sdk.ToolResultPart
	switch strings.ToLower(strings.TrimSpace(input.Decision)) {
	case "approve", "approved":
		approved, err := r.toolApproval.Approve(ctx, target.ID, input.ActorChannelIdentityID, input.Reason)
		if err != nil {
			return err
		}
		toolResult, err = r.executeApprovedTool(ctx, approved, input)
		if err != nil {
			return err
		}
	case "reject", "rejected":
		rejected, err := r.toolApproval.Reject(ctx, target.ID, input.ActorChannelIdentityID, input.Reason)
		if err != nil {
			return err
		}
		toolResult = sdk.ToolResultPart{
			ToolCallID: rejected.ToolCallID,
			ToolName:   rejected.ToolName,
			Result:     rejectedToolResultText(input.Reason),
			IsError:    true,
		}
	default:
		return fmt.Errorf("unknown tool approval decision %q", input.Decision)
	}

	return r.storeToolResultAndContinue(ctx, target, input, toolResult, eventCh)
}

func (r *Resolver) executeApprovedTool(ctx context.Context, req toolapproval.Request, input ToolApprovalResponseInput) (sdk.ToolResultPart, error) {
	resolved, err := r.ResolveRunConfig(ctx,
		input.BotID,
		req.SessionID,
		firstNonEmpty(req.ChannelIdentityID, input.ActorChannelIdentityID),
		req.SourcePlatform,
		req.ReplyTarget,
		req.ConversationType,
		input.ChatToken,
	)
	if err != nil {
		return sdk.ToolResultPart{}, err
	}
	return r.agent.ExecuteTool(ctx, resolved.RunConfig, sdk.ToolCall{
		ToolCallID: req.ToolCallID,
		ToolName:   req.ToolName,
		Input:      req.ToolInput,
	})
}

func (r *Resolver) storeToolResultAndContinue(ctx context.Context, approval toolapproval.Request, input ToolApprovalResponseInput, result sdk.ToolResultPart, eventCh chan<- WSStreamEvent) error {
	modelMessages := sdkMessagesToModelMessages([]sdk.Message{sdk.ToolMessage(result)})
	storeReq := conversation.ChatRequest{
		BotID:                   input.BotID,
		ChatID:                  input.BotID,
		SessionID:               approval.SessionID,
		SourceChannelIdentityID: firstNonEmpty(approval.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentChannel:          approval.SourcePlatform,
		ReplyTarget:             approval.ReplyTarget,
		ConversationType:        approval.ConversationType,
		UserMessagePersisted:    true,
	}
	if err := r.storeRoundWithOptions(ctx, storeReq, modelMessages, "", storeRoundOptions{AllowPendingToolCalls: true}); err != nil {
		return err
	}
	return r.continueToolApprovalSession(ctx, approval, input, eventCh)
}

func (r *Resolver) continueToolApprovalSession(ctx context.Context, approval toolapproval.Request, input ToolApprovalResponseInput, eventCh chan<- WSStreamEvent) error {
	resolved, err := r.ResolveRunConfig(ctx,
		input.BotID,
		approval.SessionID,
		firstNonEmpty(approval.ChannelIdentityID, input.ActorChannelIdentityID),
		approval.SourcePlatform,
		approval.ReplyTarget,
		approval.ConversationType,
		input.ChatToken,
	)
	if err != nil {
		return err
	}

	loaded, err := r.loadMessages(ctx, input.BotID, approval.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded = r.replaceCompactedMessages(ctx, loaded)
	messages, _ := trimMessagesByTokens(r.logger, loaded, 0)

	cfg := resolved.RunConfig
	cfg.Messages = modelMessagesToSDKMessages(nonNilModelMessages(sanitizeMessages(messages)))
	cfg.Query = ""
	cfg = r.prepareRunConfig(ctx, cfg)

	req := conversation.ChatRequest{
		BotID:                   input.BotID,
		ChatID:                  input.BotID,
		SessionID:               approval.SessionID,
		SourceChannelIdentityID: firstNonEmpty(approval.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentChannel:          approval.SourcePlatform,
		ReplyTarget:             approval.ReplyTarget,
		ConversationType:        approval.ConversationType,
		UserMessagePersisted:    true,
	}

	stream := r.agent.Stream(ctx, cfg)
	stored := false
	for event := range stream {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if !stored && event.IsTerminal() && len(event.Messages) > 0 {
			if _, storeErr := r.tryStoreStream(ctx, req, data, resolved.ModelID, resolvedContext{model: models.GetResponse{ID: resolved.ModelID}}); storeErr != nil {
				return storeErr
			}
			stored = true
		}
		if eventCh != nil {
			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func rejectedToolResultText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "tool execution rejected by user"
	}
	return "tool execution rejected by user: " + reason
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
