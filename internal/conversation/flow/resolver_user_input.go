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

type UserInputResponseInput struct {
	BotID                  string
	SessionID              string
	ActorChannelIdentityID string
	UserInputID            string
	ExplicitID             string
	ReplyExternalMessageID string
	Answer                 any
	OptionID               string
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

	var resolved userinput.Request
	if input.Canceled {
		resolved, err = r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 input.Reason,
		})
	} else {
		submit := userinput.SubmitInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Answer:                 input.Answer,
			OptionID:               input.OptionID,
		}
		if submit.OptionID != "" {
			submit.OptionValue = responseOptionValue(target, submit.OptionID)
		}
		resolved, err = r.userInput.Submit(ctx, submit)
	}
	if err != nil {
		return err
	}

	toolResult := sdk.ToolResultPart{
		ToolCallID: resolved.ToolCallID,
		ToolName:   resolved.ToolName,
		Result:     resolved.Result,
		IsError:    false,
	}
	return r.storeUserInputResultAndContinue(ctx, resolved, input, toolResult, eventCh)
}

func optionValue(req userinput.Request, optionID string) any {
	for _, option := range req.UIPayload.Options {
		if option.ID == optionID {
			return option.Value
		}
	}
	return nil
}

func optionRequiresText(req userinput.Request, optionID string) bool {
	for _, option := range req.UIPayload.Options {
		if option.ID == optionID {
			return option.InputType == "text"
		}
	}
	return false
}

func responseOptionValue(req userinput.Request, optionID string) any {
	if optionRequiresText(req, optionID) {
		return nil
	}
	return optionValue(req, optionID)
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
