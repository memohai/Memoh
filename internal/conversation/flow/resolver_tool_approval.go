package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextlimit"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
)

type ToolApprovalResponseInput struct {
	BotID                      string
	SessionID                  string
	BaseHeadTurnID             string
	ActorChannelIdentityID     string
	ApprovalID                 string
	ExplicitID                 string
	ReplyExternalMessageID     string
	Decision                   string
	Reason                     string
	ChatToken                  string
	SuppressActivePromptAttach bool
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
	if err := r.validateBaseContinuationTurnHead(ctx, target.SessionID, target.PersistTurnID, input.BaseHeadTurnID, "tool approval"); err != nil {
		return err
	}
	if isACP, err := r.isACPToolApprovalSession(ctx, target.SessionID); err != nil {
		return err
	} else if isACP {
		return r.respondACPToolApproval(ctx, target, input, eventCh)
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
			Result:     r.limitToolResultText(rejectedToolResultText(input.Reason), rejected.ToolName),
			IsError:    true,
		}
	default:
		return fmt.Errorf("unknown tool approval decision %q", input.Decision)
	}

	return r.storeToolResultAndContinue(ctx, target, input, toolResult, eventCh)
}

func (r *Resolver) toolOutputLimit() contextlimit.ToolOutputLimit {
	limit := agentpkg.DefaultLimits().ToolOutputLimit()
	if r != nil && r.agent != nil {
		limit = r.agent.Limits().ToolOutputLimit()
	}
	return limit
}

func (r *Resolver) limitToolResultText(text, toolName string) string {
	limit := r.toolOutputLimit()
	return contextlimit.LimitString(text, "tool result ("+toolName+")", limit)
}

func (r *Resolver) limitToolResultValue(value any, toolName string) any {
	return contextlimit.LimitToolOutput(value, "tool result ("+toolName+")", r.toolOutputLimit())
}

func (r *Resolver) limitToolApprovalResult(result sdk.ToolApprovalResult, toolName string) sdk.ToolApprovalResult {
	if result.Decision == sdk.ToolApprovalDecisionRejected {
		result.Reason = r.limitToolResultText(result.Reason, toolName)
	}
	return result
}

func (r *Resolver) isACPToolApprovalSession(ctx context.Context, sessionID string) (bool, error) {
	if r == nil || r.sessionService == nil {
		return false, nil
	}
	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return sess.Type == sessionpkg.TypeACPAgent, nil
}

func (r *Resolver) respondACPToolApproval(ctx context.Context, target toolapproval.Request, input ToolApprovalResponseInput, eventCh chan<- WSStreamEvent) error {
	if !r.toolApproval.CanRespond(target) {
		_, err := r.toolApproval.Reject(ctx, target.ID, "", "tool approval expired: the requesting tool call is no longer waiting")
		if err != nil && !errors.Is(err, toolapproval.ErrAlreadyDecided) {
			return err
		}
		return emitApprovalAck(ctx, eventCh)
	}
	var activePrompt *acpActivePromptSubscription
	if eventCh != nil && !input.SuppressActivePromptAttach {
		activePrompt, _ = r.subscribeACPActivePrompt(
			firstNonEmpty(target.BotID, input.BotID),
			firstNonEmpty(target.SessionID, input.SessionID),
		)
	}
	switch strings.ToLower(strings.TrimSpace(input.Decision)) {
	case "approve", "approved":
		if _, err := r.toolApproval.Approve(ctx, target.ID, input.ActorChannelIdentityID, input.Reason); err != nil {
			if activePrompt != nil {
				activePrompt.release()
			}
			return err
		}
	case "reject", "rejected":
		if _, err := r.toolApproval.Reject(ctx, target.ID, input.ActorChannelIdentityID, input.Reason); err != nil {
			if activePrompt != nil {
				activePrompt.release()
			}
			return err
		}
	default:
		if activePrompt != nil {
			activePrompt.release()
		}
		return fmt.Errorf("unknown tool approval decision %q", input.Decision)
	}
	if activePrompt != nil {
		return forwardACPActivePrompt(ctx, activePrompt, eventCh, acpActivePromptForwardOptions{
			SkipToolCallID: target.ToolCallID,
			SkipApprovalID: target.ID,
		})
	}
	return emitApprovalAck(ctx, eventCh)
}

func emitApprovalAck(ctx context.Context, eventCh chan<- WSStreamEvent) error {
	if eventCh == nil {
		return nil
	}
	for _, event := range []agentpkg.StreamEvent{
		{Type: agentpkg.EventAgentStart},
		{Type: agentpkg.EventAgentEnd},
	} {
		if err := sendAgentStreamEvent(ctx, eventCh, event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) executeApprovedTool(ctx context.Context, req toolapproval.Request, input ToolApprovalResponseInput) (sdk.ToolResultPart, error) {
	req = withLocalWebReplyTarget(req)
	resolved, err := r.resolveRunConfig(ctx, baseRunConfigParams{
		BotID:             input.BotID,
		SessionID:         req.SessionID,
		ChannelIdentityID: firstNonEmpty(req.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentPlatform:   req.SourcePlatform,
		ReplyTarget:       req.ReplyTarget,
		ConversationType:  req.ConversationType,
		SessionToken:      input.ChatToken,
		PersistTurnID:     req.PersistTurnID,
		BaseHeadTurnID:    req.PersistTurnID,
	})
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
	approval = withLocalWebReplyTarget(approval)
	doneTurn := r.enterSessionTurn(ctx, input.BotID, approval.SessionID)
	defer doneTurn()
	if err := r.validateBaseContinuationTurnHead(ctx, approval.SessionID, approval.PersistTurnID, input.BaseHeadTurnID, "tool approval"); err != nil {
		return err
	}
	modelMessages := sdkMessagesToModelMessages([]sdk.Message{sdk.ToolMessage(result)})
	run := continuationTurnRun(approval.SessionID, approval.PersistTurnID)
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
	if err := r.storeRoundWithOptions(ctx, storeReq, &run, modelMessages, "", storeRoundOptions{AllowPendingToolCalls: true}); err != nil {
		return err
	}
	return r.continueToolApprovalSession(ctx, approval, input, eventCh)
}

func (r *Resolver) continueToolApprovalSession(ctx context.Context, approval toolapproval.Request, input ToolApprovalResponseInput, eventCh chan<- WSStreamEvent) error {
	approval = withLocalWebReplyTarget(approval)
	resolved, err := r.resolveRunConfig(ctx, baseRunConfigParams{
		BotID:             input.BotID,
		SessionID:         approval.SessionID,
		ChannelIdentityID: firstNonEmpty(approval.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentPlatform:   approval.SourcePlatform,
		ReplyTarget:       approval.ReplyTarget,
		ConversationType:  approval.ConversationType,
		SessionToken:      input.ChatToken,
		PersistTurnID:     approval.PersistTurnID,
		BaseHeadTurnID:    approval.PersistTurnID,
	})
	if err != nil {
		return err
	}

	run := continuationTurnRun(approval.SessionID, approval.PersistTurnID)
	contextReq := conversation.ChatRequest{
		BotID:     input.BotID,
		ChatID:    input.BotID,
		SessionID: approval.SessionID,
	}
	loaded, err := r.loadMessagesForTurnRun(ctx, contextReq, run, defaultMaxContextMinutes)
	if err != nil {
		return err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded = r.replaceCompactedMessages(ctx, loaded)
	messages, _ := trimMessagesByTokens(r.logger, loaded, 0)

	cfg := resolved.RunConfig
	cfg.Messages = modelMessagesToSDKMessages(nonNilModelMessages(sanitizeMessages(messages)))
	cfg.Query = ""
	cfg.LiveToolStream = eventCh != nil
	cfg.CanRequestUserInput = r.canDeliverUserInputWS(eventCh)
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
			if snap, ok := extractTerminalSnapshot(data); ok {
				if storeErr := r.persistTerminalSnapshot(
					context.WithoutCancel(ctx),
					req,
					&run,
					resolvedContext{model: models.GetResponse{ID: resolved.ModelID}},
					snap,
				); storeErr != nil {
					return storeErr
				}
				stored = true
			}
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

func withLocalWebReplyTarget(req toolapproval.Request) toolapproval.Request {
	if strings.EqualFold(strings.TrimSpace(req.SourcePlatform), "web") && strings.TrimSpace(req.ReplyTarget) == "" {
		req.ReplyTarget = strings.TrimSpace(req.BotID)
	}
	return req
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
