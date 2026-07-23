package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/runtimefence"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/userinput"
	"github.com/memohai/memoh/internal/workspace"
)

// userInputService is the slice of *userinput.Service the resolver depends
// on, kept as an interface so the respond/resume routing can be tested with
// fakes.
type userInputService interface {
	CreatePending(ctx context.Context, input userinput.CreatePendingInput) (userinput.Request, error)
	ResolveTarget(ctx context.Context, input userinput.ResolveInput) (userinput.Request, error)
	AdvanceText(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error)
	Submit(ctx context.Context, input userinput.SubmitInput) (userinput.Request, error)
	Cancel(ctx context.Context, input userinput.CancelInput) (userinput.Request, error)
	CanRespond(req userinput.Request) bool
}

func (r *Resolver) AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	if r.userInput == nil {
		return userinput.AdvanceTextResult{}, errors.New("user input service not configured")
	}
	return r.userInput.AdvanceText(ctx, input)
}

type UserInputResponseInput struct {
	BotID                      string
	SessionID                  string
	ActorChannelIdentityID     string
	ActorUserID                string
	UserInputID                string
	ExplicitID                 string
	ReplyExternalMessageID     string
	Answers                    []userinput.QuestionAnswer
	TextAnswer                 string
	Canceled                   bool
	Reason                     string
	ChatToken                  string
	SuppressActivePromptAttach bool
	ResolveOnly                bool
}

// CommittedUserInputResponse contains the durable result of an ask_user
// response and the owner-local state needed to continue its parent turn.
// The decision is already terminal when this value is returned.
type CommittedUserInputResponse struct {
	request      userinput.Request
	input        UserInputResponseInput
	activePrompt *acpActivePromptSubscription
}

// CommitUserInputResponse durably resolves an ask_user decision without
// starting its agent continuation. Runtime admission uses this split so the
// terminal decision is published before any continuation event.
func (r *Resolver) CommitUserInputResponse(ctx context.Context, input UserInputResponseInput) (CommittedUserInputResponse, error) {
	if r.userInput == nil {
		return CommittedUserInputResponse{}, errors.New("user input service not configured")
	}
	target, err := r.userInput.ResolveTarget(ctx, userinput.ResolveInput{
		BotID:                  input.BotID,
		SessionID:              input.SessionID,
		ExplicitID:             firstNonEmpty(input.ExplicitID, input.UserInputID),
		ReplyExternalMessageID: input.ReplyExternalMessageID,
	})
	if err != nil {
		return CommittedUserInputResponse{}, err
	}
	if err := runtimefence.ValidateScope(ctx, target.BotID, target.SessionID); err != nil {
		return CommittedUserInputResponse{}, err
	}

	isACPMCP := userinput.IsACPMCPRequest(target)
	if isACPMCP {
		if err := r.authorizeACPUserInputResponse(ctx, target, input); err != nil {
			return CommittedUserInputResponse{}, err
		}
	} else {
		ctx = workspace.WithWorkspaceTarget(ctx, target.WorkspaceTargetID)
	}
	if isACPMCP && !r.userInput.CanRespond(target) {
		if target.RuntimeFenced {
			return CommittedUserInputResponse{}, ErrRuntimeDecisionOwnerUnavailable
		}
		if _, err := r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 "user input expired: the requesting tool call is no longer waiting",
		}); err != nil && !errors.Is(err, userinput.ErrAlreadyDecided) {
			return CommittedUserInputResponse{}, err
		}
		return CommittedUserInputResponse{request: target, input: input}, nil
	}

	var activePrompt *acpActivePromptSubscription
	if isACPMCP && !input.SuppressActivePromptAttach {
		activePrompt, _ = r.subscribeACPActivePrompt(
			firstNonEmpty(target.BotID, input.BotID),
			firstNonEmpty(target.SessionID, input.SessionID),
		)
	}

	var resolved userinput.Request
	if input.Canceled {
		resolved, err = r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 input.Reason,
		})
	} else {
		answers := input.Answers
		if len(answers) == 0 && strings.TrimSpace(input.TextAnswer) != "" {
			answers, err = userInputAnswersFromText(target.UIPayload, input.TextAnswer)
		}
		if err == nil {
			resolved, err = r.userInput.Submit(ctx, userinput.SubmitInput{
				RequestID:              target.ID,
				ActorChannelIdentityID: input.ActorChannelIdentityID,
				Answers:                answers,
			})
		}
	}
	if err != nil {
		if activePrompt != nil {
			activePrompt.release()
		}
		return CommittedUserInputResponse{}, err
	}
	return CommittedUserInputResponse{request: resolved, input: input, activePrompt: activePrompt}, nil
}

// ContinueCommittedUserInputResponse resumes the parent turn without
// resolving the ask_user decision a second time.
func (r *Resolver) ContinueCommittedUserInputResponse(ctx context.Context, committed CommittedUserInputResponse, eventCh chan<- WSStreamEvent) error {
	resolved := committed.request
	if strings.TrimSpace(resolved.ID) == "" {
		return errors.New("committed user input response is missing its request")
	}
	if userinput.IsACPMCPRequest(resolved) {
		if committed.activePrompt != nil {
			return forwardACPActivePrompt(ctx, committed.activePrompt, eventCh, acpActivePromptForwardOptions{
				SkipToolCallID:  resolved.ToolCallID,
				SkipUserInputID: resolved.ID,
			})
		}
		return emitApprovalAck(ctx, eventCh)
	}
	toolResult := sdk.ToolResultPart{
		ToolCallID: resolved.ToolCallID,
		ToolName:   resolved.ToolName,
		Result:     r.limitToolResultValue(resolved.Result, resolved.ToolName),
		IsError:    false,
	}
	continueFn := r.continueUserInputFn
	if continueFn == nil {
		continueFn = r.storeUserInputResultAndContinue
	}
	return continueFn(ctx, resolved, committed.input, toolResult, eventCh)
}

func (r *Resolver) PrepareUserInputResponse(ctx context.Context, input UserInputResponseInput) (runtimefence.PreservedDecision, error) {
	return r.prepareUserInputResponseTarget(ctx, input, false)
}

// PrepareUserInputResponseTarget validates a response and returns its
// canonical scope even when the decision was already committed. Runtime
// routers use that scope to replay or reconcile idempotent commands.
func (r *Resolver) PrepareUserInputResponseTarget(ctx context.Context, input UserInputResponseInput) (runtimefence.PreservedDecision, error) {
	return r.prepareUserInputResponseTarget(ctx, input, true)
}

func (r *Resolver) prepareUserInputResponseTarget(ctx context.Context, input UserInputResponseInput, includeDecided bool) (runtimefence.PreservedDecision, error) {
	if r.userInput == nil {
		return runtimefence.PreservedDecision{}, errors.New("user input service not configured")
	}
	target, err := r.userInput.ResolveTarget(ctx, userinput.ResolveInput{
		BotID:                  input.BotID,
		SessionID:              input.SessionID,
		ExplicitID:             firstNonEmpty(input.ExplicitID, input.UserInputID),
		ReplyExternalMessageID: input.ReplyExternalMessageID,
	})
	if includeDecided && errors.Is(err, userinput.ErrNotFound) {
		explicitID := firstNonEmpty(input.ExplicitID, input.UserInputID)
		if getter, ok := r.userInput.(userInputRequestGetter); ok && explicitID != "" {
			if existing, getErr := getter.Get(ctx, explicitID); getErr == nil {
				if (strings.TrimSpace(input.BotID) == "" || strings.TrimSpace(input.BotID) == existing.BotID) &&
					(strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.SessionID) == existing.SessionID) {
					target = existing
					err = nil
				}
			}
		}
	}
	if err != nil {
		return runtimefence.PreservedDecision{}, err
	}
	if err := runtimefence.ValidateScope(ctx, target.BotID, target.SessionID); err != nil {
		return runtimefence.PreservedDecision{}, err
	}
	if userinput.IsACPMCPRequest(target) {
		if err := r.authorizeACPUserInputResponse(ctx, target, input); err != nil {
			return runtimefence.PreservedDecision{}, err
		}
	}
	if !input.Canceled {
		answers := input.Answers
		if len(answers) == 0 && strings.TrimSpace(input.TextAnswer) != "" {
			answers, err = userInputAnswersFromText(target.UIPayload, input.TextAnswer)
			if err != nil {
				return runtimefence.PreservedDecision{}, err
			}
		}
		if err := userinput.ValidateAnswers(target.UIPayload, answers); err != nil {
			return runtimefence.PreservedDecision{}, err
		}
	}
	return runtimefence.PreservedDecision{
		Kind:      runtimefence.DecisionUserInput,
		ID:        target.ID,
		BotID:     target.BotID,
		SessionID: target.SessionID,
	}, nil
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
		if input.ResolveOnly && errors.Is(err, userinput.ErrNotFound) {
			if handled, replayErr := r.reconcileUserInputReplay(ctx, input); handled {
				return replayErr
			}
		}
		return err
	}
	if err := runtimefence.ValidateScope(ctx, target.BotID, target.SessionID); err != nil {
		return err
	}

	isACPMCP := userinput.IsACPMCPRequest(target)
	if isACPMCP {
		if err := r.authorizeACPUserInputResponse(ctx, target, input); err != nil {
			return err
		}
	}
	if !isACPMCP {
		ctx = workspace.WithWorkspaceTarget(ctx, target.WorkspaceTargetID)
	}
	if isACPMCP && !input.ResolveOnly && !r.userInput.CanRespond(target) {
		if target.RuntimeFenced {
			return ErrRuntimeDecisionOwnerUnavailable
		}
		if _, err := r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 "user input expired: the requesting tool call is no longer waiting",
		}); err != nil && !errors.Is(err, userinput.ErrAlreadyDecided) {
			return err
		}
		return emitApprovalAck(ctx, eventCh)
	}
	var activePrompt *acpActivePromptSubscription
	if isACPMCP && eventCh != nil && !input.SuppressActivePromptAttach {
		activePrompt, _ = r.subscribeACPActivePrompt(
			firstNonEmpty(target.BotID, input.BotID),
			firstNonEmpty(target.SessionID, input.SessionID),
		)
	}

	var resolved userinput.Request
	if input.Canceled {
		resolved, err = r.userInput.Cancel(ctx, userinput.CancelInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Reason:                 input.Reason,
		})
	} else {
		answers := input.Answers
		if len(answers) == 0 && strings.TrimSpace(input.TextAnswer) != "" {
			answers, err = userInputAnswersFromText(target.UIPayload, input.TextAnswer)
			if err != nil {
				if activePrompt != nil {
					activePrompt.release()
				}
				return err
			}
		}
		resolved, err = r.userInput.Submit(ctx, userinput.SubmitInput{
			RequestID:              target.ID,
			ActorChannelIdentityID: input.ActorChannelIdentityID,
			Answers:                answers,
		})
	}
	if err != nil {
		if activePrompt != nil {
			activePrompt.release()
		}
		if isACPMCP && errors.Is(err, userinput.ErrAlreadyDecided) {
			return emitApprovalAck(ctx, eventCh)
		}
		return err
	}
	if input.ResolveOnly {
		return emitApprovalAck(ctx, eventCh)
	}
	if userinput.IsACPMCPRequest(resolved) {
		// An ACP/MCP waiter is blocked on this request and resumes the run
		// itself. When this response stream has reattached to the active ACP
		// prompt, forward that live continuation so refreshes observe the same
		// loading/progress shape as native deferred requests.
		if activePrompt != nil {
			return forwardACPActivePrompt(ctx, activePrompt, eventCh, acpActivePromptForwardOptions{
				SkipToolCallID:  target.ToolCallID,
				SkipUserInputID: target.ID,
			})
		}
		return emitApprovalAck(ctx, eventCh)
	}

	toolResult := sdk.ToolResultPart{
		ToolCallID: resolved.ToolCallID,
		ToolName:   resolved.ToolName,
		Result:     r.limitToolResultValue(resolved.Result, resolved.ToolName),
		IsError:    false,
	}
	continueFn := r.continueUserInputFn
	if continueFn == nil {
		continueFn = r.storeUserInputResultAndContinue
	}
	return continueFn(ctx, resolved, input, toolResult, eventCh)
}

type userInputRequestGetter interface {
	Get(context.Context, string) (userinput.Request, error)
}

func (r *Resolver) reconcileUserInputReplay(ctx context.Context, input UserInputResponseInput) (bool, error) {
	getter, ok := r.userInput.(userInputRequestGetter)
	if !ok {
		return false, nil
	}
	targetID := firstNonEmpty(input.ExplicitID, input.UserInputID)
	target, err := getter.Get(ctx, targetID)
	if err != nil {
		if errors.Is(err, userinput.ErrNotFound) {
			return false, nil
		}
		return true, err
	}
	if strings.TrimSpace(target.BotID) != strings.TrimSpace(input.BotID) || strings.TrimSpace(target.SessionID) != strings.TrimSpace(input.SessionID) {
		return false, nil
	}
	if target.Status == userinput.StatusPending {
		return false, nil
	}
	if err := runtimefence.ValidateScope(ctx, target.BotID, target.SessionID); err != nil {
		return true, err
	}
	if userinput.IsACPMCPRequest(target) {
		if err := r.authorizeACPUserInputResponse(ctx, target, input); err != nil {
			return true, err
		}
	}
	answers := input.Answers
	if len(answers) == 0 && strings.TrimSpace(input.TextAnswer) != "" {
		answers, err = userInputAnswersFromText(target.UIPayload, input.TextAnswer)
		if err != nil {
			return true, err
		}
	}
	matches, err := userinput.ResponseMatches(target, input.Canceled, input.Reason, answers)
	if err != nil {
		return true, err
	}
	if !matches {
		return true, userinput.ErrAlreadyDecided
	}
	return true, emitApprovalAck(ctx, nil)
}

// ReconcileUserInputResponse checks whether a ResolveOnly response was
// already committed. It is read-only and does not require local run control.
func (r *Resolver) ReconcileUserInputResponse(ctx context.Context, input UserInputResponseInput) (bool, error) {
	if r == nil || r.userInput == nil {
		return false, errors.New("user input service not configured")
	}
	return r.reconcileUserInputReplay(ctx, input)
}

func (r *Resolver) authorizeACPUserInputResponse(ctx context.Context, target userinput.Request, input UserInputResponseInput) error {
	if r == nil || r.sessionService == nil {
		return errors.New("session service not configured")
	}
	if r.botPermissions == nil {
		return errors.New("bot permission checker not configured")
	}
	sessionID := firstNonEmpty(target.SessionID, input.SessionID)
	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !sessionpkg.IsACPRuntime(sess) {
		return nil
	}
	botID := firstNonEmpty(target.BotID, input.BotID)
	if strings.TrimSpace(sess.BotID) != "" && strings.TrimSpace(botID) != "" && sess.BotID != botID {
		return userinput.ErrForbidden
	}
	if strings.TrimSpace(botID) == "" {
		botID = sess.BotID
	}
	actorID := firstNonEmpty(input.ActorUserID, input.ActorChannelIdentityID)
	if actorID == "" {
		return userinput.ErrForbidden
	}
	acpMeta := mergeACPRuntimeMetadata(sess.Metadata, sess.RuntimeMetadata)
	runtimeOwnerID := metadataString(acpMeta, "runtime_owner_account_id")
	if runtimeOwnerID == "" {
		return userinput.ErrForbidden
	}
	if ok, err := r.botPermissions.HasBotPermission(ctx, botID, runtimeOwnerID, bots.PermissionWorkspaceExec); err != nil {
		return err
	} else if !ok {
		return userinput.ErrForbidden
	}
	if actorID != runtimeOwnerID {
		return userinput.ErrForbidden
	}
	return nil
}

func userInputAnswersFromText(payload userinput.UIPayload, text string) ([]userinput.QuestionAnswer, error) {
	answerText := strings.TrimSpace(text)
	if answerText == "" {
		return nil, errors.New("user input answer is required")
	}
	if len(payload.Questions) != 1 {
		return nil, errors.New("text response command can answer exactly one user input question")
	}
	question := payload.Questions[0]
	answer := userinput.QuestionAnswer{QuestionID: question.ID}
	switch question.Kind {
	case userinput.QuestionKindText:
		answer.Text = answerText
	case userinput.QuestionKindSingleSelect:
		optionID, ok := matchUserInputOption(question, answerText)
		switch {
		case ok:
			answer.OptionIDs = []string{optionID}
		case question.AllowCustom:
			answer.CustomText = answerText
		default:
			return nil, fmt.Errorf("answer %q does not match an option for question %q", answerText, question.ID)
		}
	case userinput.QuestionKindMultiSelect:
		parts := splitUserInputAnswerText(answerText)
		optionIDs := make([]string, 0, len(parts))
		custom := ""
		for _, part := range parts {
			if optionID, ok := matchUserInputOption(question, part); ok {
				optionIDs = append(optionIDs, optionID)
				continue
			}
			if question.AllowCustom && custom == "" {
				custom = part
				continue
			}
			return nil, fmt.Errorf("answer %q does not match an option for question %q", part, question.ID)
		}
		answer.OptionIDs = optionIDs
		answer.CustomText = custom
	default:
		return nil, fmt.Errorf("question %q has unsupported kind %q", question.ID, question.Kind)
	}
	return []userinput.QuestionAnswer{answer}, nil
}

func matchUserInputOption(question userinput.UIQuestion, text string) (string, bool) {
	target := strings.TrimSpace(text)
	for _, option := range question.Options {
		if strings.EqualFold(strings.TrimSpace(option.ID), target) || strings.EqualFold(strings.TrimSpace(option.Label), target) {
			return option.ID, true
		}
	}
	return "", false
}

func splitUserInputAnswerText(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if part := strings.TrimSpace(field); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 && strings.TrimSpace(text) != "" {
		parts = append(parts, strings.TrimSpace(text))
	}
	return parts
}

func (r *Resolver) storeUserInputResultAndContinue(ctx context.Context, req userinput.Request, input UserInputResponseInput, result sdk.ToolResultPart, eventCh chan<- WSStreamEvent) error {
	req = withLocalWebUserInputReplyTarget(req)
	ctx = workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	target, err := r.resolveWorkspaceTargetSnapshot(ctx, input.BotID, req.WorkspaceTargetID)
	if err != nil {
		return err
	}
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
		WorkspaceTargetID:       req.WorkspaceTargetID,
		WorkspaceTarget:         target,
	}
	if err := r.storeRoundWithOptions(ctx, storeReq, modelMessages, "", storeRoundOptions{AllowPendingToolCalls: true}); err != nil {
		return err
	}
	return r.continueUserInputSession(ctx, req, input, eventCh)
}

func (r *Resolver) continueUserInputSession(ctx context.Context, req userinput.Request, input UserInputResponseInput, eventCh chan<- WSStreamEvent) error {
	req = withLocalWebUserInputReplyTarget(req)
	ctx = workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
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

	cfg, err := r.prepareContinuationRunConfig(
		ctx,
		resolved.RunConfig,
		historyScopeFallbackFromUserInputRequest(req),
		compactionSummaryScope(firstNonEmpty(req.BotID, input.BotID), "", req.SessionID, req.ConversationType, "", req.ReplyTarget),
		eventCh,
	)
	if err != nil {
		return err
	}

	chatReq := conversation.ChatRequest{
		BotID:                   input.BotID,
		ChatID:                  input.BotID,
		SessionID:               req.SessionID,
		SourceChannelIdentityID: firstNonEmpty(req.ChannelIdentityID, input.ActorChannelIdentityID),
		CurrentChannel:          req.SourcePlatform,
		ReplyTarget:             req.ReplyTarget,
		ConversationType:        req.ConversationType,
		UserMessagePersisted:    true,
		WorkspaceTargetID:       req.WorkspaceTargetID,
		WorkspaceTarget:         workspaceTargetFromRunConfig(resolved.RunConfig),
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

func withLocalWebUserInputReplyTarget(req userinput.Request) userinput.Request {
	if strings.EqualFold(strings.TrimSpace(req.SourcePlatform), "web") && strings.TrimSpace(req.ReplyTarget) == "" {
		req.ReplyTarget = strings.TrimSpace(req.BotID)
	}
	return req
}
