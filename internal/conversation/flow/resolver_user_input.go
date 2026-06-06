package flow

import (
	"context"
	"encoding/json"
	"errors"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/userinput"
)

// userInputService is the slice of *userinput.Service the resolver depends
// on, kept as an interface so the respond/resume routing can be tested with
// fakes.
type userInputService interface {
	CreatePending(ctx context.Context, input userinput.CreatePendingInput) (userinput.Request, error)
	ResolveTarget(ctx context.Context, input userinput.ResolveInput) (userinput.Request, error)
	Submit(ctx context.Context, input userinput.SubmitInput) (userinput.Request, error)
	Cancel(ctx context.Context, input userinput.CancelInput) (userinput.Request, error)
	HasWaiter(requestID string) bool
}

type UserInputResponseInput struct {
	BotID                  string
	SessionID              string
	ActorChannelIdentityID string
	UserInputID            string
	ExplicitID             string
	ReplyExternalMessageID string
	Answers                []userinput.QuestionAnswer
	Canceled               bool
	Reason                 string
	ChatToken              string
}

func (r *Resolver) RespondUserInput(ctx context.Context, input UserInputResponseInput, eventCh chan<- WSStreamEvent) error {
	if r.userInput == nil {
		return errors.New("user input service not configured")
	}
	target, err := r.userInput.ResolveTarget(ctx, userinput.ResolveInput{
		BotID:                  input.BotID,
		SessionID:              input.SessionID,
		ExplicitID:             firstNonEmpty(input.ExplicitID, input.UserInputID),
		ReplyExternalMessageID: input.ReplyExternalMessageID,
	})
	if err != nil {
		return err
	}

	if userinput.IsACPMCPRequest(target) && !input.Canceled && !r.userInput.HasWaiter(target.ID) {
		// No waiter is blocked on this request in this process (its timeout
		// fired, or the process restarted). Submitting would record an
		// answer nobody consumes — close it out honestly instead.
		if _, err := r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 "user input expired: the requesting tool call is no longer waiting",
		}); err != nil && !errors.Is(err, userinput.ErrAlreadyDecided) {
			return err
		}
		return emitApprovalAck(ctx, eventCh)
	}

	var resolved userinput.Request
	if input.Canceled {
		resolved, err = r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 input.Reason,
		})
	} else {
		resolved, err = r.userInput.Submit(ctx, userinput.SubmitInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Answers:                input.Answers,
		})
	}
	if err != nil {
		if userinput.IsACPMCPRequest(target) && errors.Is(err, userinput.ErrAlreadyDecided) {
			return emitApprovalAck(ctx, eventCh)
		}
		return err
	}
	if userinput.IsACPMCPRequest(resolved) {
		// An ACP/MCP waiter is blocked on this request and resumes the run
		// itself; only acknowledge here instead of continuing the session.
		return emitApprovalAck(ctx, eventCh)
	}

	toolResult := sdk.ToolResultPart{
		ToolCallID: resolved.ToolCallID,
		ToolName:   resolved.ToolName,
		Result:     resolved.Result,
		IsError:    false,
	}
	continueFn := r.continueUserInputFn
	if continueFn == nil {
		continueFn = r.storeUserInputResultAndContinue
	}
	return continueFn(ctx, resolved, input, toolResult, eventCh)
}

func (r *Resolver) storeUserInputResultAndContinue(ctx context.Context, req userinput.Request, input UserInputResponseInput, result sdk.ToolResultPart, eventCh chan<- WSStreamEvent) error {
	modelMessages := sdkMessagesToModelMessages([]sdk.Message{sdk.ToolMessage(result)})
	storeReq := conversation.ChatRequest{
		BotID:                   input.BotID,
		ChatID:                  input.BotID,
		SessionID:               req.SessionID,
		SourceChannelIdentityID: firstNonEmpty(req.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentChannel:          req.SourcePlatform,
		ReplyTarget:             req.ReplyTarget,
		ConversationType:        req.ConversationType,
		UserMessagePersisted:    true,
	}
	if err := r.storeRoundWithOptions(ctx, storeReq, modelMessages, "", storeRoundOptions{AllowPendingToolCalls: true}); err != nil {
		return err
	}
	return r.continueUserInputSession(ctx, req, input, eventCh)
}

func (r *Resolver) continueUserInputSession(ctx context.Context, req userinput.Request, input UserInputResponseInput, eventCh chan<- WSStreamEvent) error {
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
		return err
	}

	loaded, err := r.loadMessages(ctx, input.BotID, req.SessionID, defaultMaxContextMinutes)
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

	chatReq := conversation.ChatRequest{
		BotID:                   input.BotID,
		ChatID:                  input.BotID,
		SessionID:               req.SessionID,
		SourceChannelIdentityID: firstNonEmpty(req.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentChannel:          req.SourcePlatform,
		ReplyTarget:             req.ReplyTarget,
		ConversationType:        req.ConversationType,
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
					chatReq,
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
