package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/mcp"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

const (
	nativeToolApprovalWaitTimeout = 10 * time.Minute
	nativeUserInputWaitTimeout    = 10 * time.Minute
)

type NativeToolSourceOptions struct {
	AllowAll          bool
	AllowTools        map[string]bool
	Approval          NativeToolApprovalService
	ApprovalPublisher messageevent.Publisher
	UserInput         NativeToolUserInputService
	ToolEvents        NativeToolEventSink
}

type NativeToolApprovalService interface {
	EvaluatePolicy(ctx context.Context, input toolapproval.CreatePendingInput) (toolapproval.Evaluation, error)
	CreatePending(ctx context.Context, input toolapproval.CreatePendingInput) (toolapproval.Request, error)
	Reject(ctx context.Context, approvalID, actorID, reason string) (toolapproval.Request, error)
	WaitForDecision(ctx context.Context, approvalID string) (toolapproval.Request, error)
}

type NativeToolUserInputService interface {
	CreatePending(ctx context.Context, input userinput.CreatePendingInput) (userinput.Request, error)
	Cancel(ctx context.Context, input userinput.CancelInput) (userinput.Request, error)
	WaitForResponse(ctx context.Context, requestID string) (userinput.Request, error)
	WaitForRegisteredResponse(ctx context.Context, requestID string) (userinput.Request, error)
	// RegisterWaiter must be called before the pending request is announced
	// to users, or an instant answer can be misjudged as orphaned.
	RegisterWaiter(requestID string) func()
}

// NativeToolEventSink delivers tool lifecycle events into the live prompt
// stream of the calling runtime — the same channel tool_call_start travels —
// so attachments like pending user input land on the existing tool call block.
type NativeToolEventSink interface {
	AppendToolEvent(session mcp.ToolSessionContext, event mcp.ToolStreamEvent) bool
}

// NativeToolSource exposes Memoh-native ToolProvider tools through the MCP
// ToolSource interface used by ACP and external tool gateways.
type NativeToolSource struct {
	logger     *slog.Logger
	mu         sync.RWMutex
	providers  []ToolProvider
	allowAll   bool
	allow      map[string]struct{}
	approval   NativeToolApprovalService
	publisher  messageevent.Publisher
	userInput  NativeToolUserInputService
	toolEvents NativeToolEventSink
}

func NewNativeToolSource(log *slog.Logger, providers []ToolProvider, opts NativeToolSourceOptions) *NativeToolSource {
	if log == nil {
		log = slog.Default()
	}
	allow := map[string]struct{}{}
	for name, enabled := range opts.AllowTools {
		if !enabled {
			continue
		}
		if normalized := strings.TrimSpace(name); normalized != "" {
			allow[normalized] = struct{}{}
		}
	}
	source := &NativeToolSource{
		logger:     log.With(slog.String("tool_source", "native")),
		allowAll:   opts.AllowAll,
		allow:      allow,
		approval:   opts.Approval,
		publisher:  opts.ApprovalPublisher,
		userInput:  opts.UserInput,
		toolEvents: opts.ToolEvents,
	}
	source.SetProviders(providers)
	return source
}

func (s *NativeToolSource) SetProviders(providers []ToolProvider) {
	if s == nil {
		return
	}
	filtered := make([]ToolProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	s.mu.Lock()
	s.providers = filtered
	s.mu.Unlock()
}

func (s *NativeToolSource) ListTools(ctx context.Context, session mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	tools := s.loadTools(ctx, session)
	if len(tools) == 0 {
		return []mcp.ToolDescriptor{}, nil
	}
	seen := map[string]struct{}{}
	descriptors := make([]mcp.ToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" || tool.Execute == nil || !s.allowed(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		descriptors = append(descriptors, mcp.ToolDescriptor{
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			InputSchema: toolInputSchema(tool.Parameters),
		})
	}
	return descriptors, nil
}

func (s *NativeToolSource) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || !s.allowed(toolName) {
		return nil, mcp.ErrToolNotFound
	}
	tools := s.loadTools(ctx, session)
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != toolName || tool.Execute == nil {
			continue
		}
		if arguments == nil {
			arguments = map[string]any{}
		}
		if toolName == userinput.ToolNameAskUser {
			return s.callAskUser(ctx, session, arguments)
		}
		approval, err := s.requireApproval(ctx, session, toolName, arguments)
		if err != nil {
			return nil, err
		}
		if !approval.approved {
			return mcp.BuildToolErrorResult(approval.message), nil
		}
		result, err := tool.Execute(&sdk.ToolExecContext{
			Context:  ctx,
			ToolName: toolName,
		}, arguments)
		if err != nil {
			return nil, err
		}
		return mcp.BuildToolSuccessResult(result), nil
	}
	return nil, mcp.ErrToolNotFound
}

func (s *NativeToolSource) callAskUser(ctx context.Context, session mcp.ToolSessionContext, arguments map[string]any) (map[string]any, error) {
	if err := userinput.ValidateAskUserInput(arguments); err != nil {
		return mcp.BuildToolSuccessResult(map[string]any{
			"status":      "invalid_arguments",
			"error":       err.Error(),
			"instruction": "Call ask_user again with a valid `questions` array. Every question needs `text` and a `kind` of single_select, multi_select, or text; select kinds need `options` with labels.",
		}), nil
	}
	if s == nil || s.userInput == nil {
		return mcp.BuildToolErrorResult("user input service is not configured"), nil
	}
	toolCallID := strings.TrimSpace(session.ToolCallID)
	if toolCallID == "" {
		toolCallID = "mcp-" + uuid.NewString()
	}
	// This request only has an in-process waiter; if the process dies before
	// the waiter can cancel it, the expiry guard keeps it from living forever
	// as an answerable zombie. The buffer keeps the waiter's own timeout as
	// the normal-path winner.
	expiresAt := time.Now().Add(nativeUserInputWaitTimeout + time.Minute)
	req, err := s.userInput.CreatePending(ctx, userinput.CreatePendingInput{
		BotID:                        session.BotID,
		SessionID:                    session.SessionID,
		RouteID:                      session.RouteID,
		ChannelIdentityID:            session.ChannelIdentityID,
		RequestedByChannelIdentityID: session.ChannelIdentityID,
		ToolCallID:                   toolCallID,
		ToolName:                     userinput.ToolNameAskUser,
		Input:                        arguments,
		ProviderMetadata: map[string]any{
			"source":     userinput.ProviderSourceACPMCP,
			"runtime_id": session.RuntimeID,
			"stream_id":  session.StreamID,
		},
		SourcePlatform:   session.CurrentPlatform,
		ReplyTarget:      session.ReplyTarget,
		ConversationType: session.ConversationType,
		ExpiresAt:        &expiresAt,
	})
	if err != nil {
		return nil, err
	}
	if req.Status != userinput.StatusPending {
		if len(req.Result) > 0 {
			return mcp.BuildToolSuccessResult(req.Result), nil
		}
		return mcp.BuildToolErrorResult("ask_user request is no longer pending"), nil
	}
	if strings.TrimSpace(session.StreamID) == "" {
		// Cleanup must survive caller cancellation, or the request stays
		// pending with nobody waiting.
		cancelCtx, cancelCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelCancel()
		canceled, cancelErr := s.userInput.Cancel(cancelCtx, userinput.CancelInput{
			RequestID:              req.ID,
			ActorChannelIdentityID: session.ChannelIdentityID,
			Reason:                 "user input requested without an interactive stream",
		})
		if cancelErr != nil {
			return nil, cancelErr
		}
		return mcp.BuildToolSuccessResult(canceled.Result), nil
	}

	// Register before announcing: the responder treats "no registered waiter"
	// as an orphaned request, so an instant answer must already see us.
	// Release before timeout/abort cleanup; cleanup must not look like a live
	// consumer.
	release := s.userInput.RegisterWaiter(req.ID)
	delivered := s.emitUserInputRequest(session, req)
	if !delivered {
		release()
		cancelCtx, cancelCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelCancel()
		canceled, cancelErr := s.userInput.Cancel(cancelCtx, userinput.CancelInput{
			RequestID:              req.ID,
			ActorChannelIdentityID: session.ChannelIdentityID,
			Reason:                 "user input request was not delivered to the interactive stream",
		})
		if cancelErr != nil {
			return nil, cancelErr
		}
		return mcp.BuildToolSuccessResult(canceled.Result), nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, nativeUserInputWaitTimeout)
	defer cancel()
	resolved, err := s.userInput.WaitForRegisteredResponse(waitCtx, req.ID)
	release()
	if err != nil {
		// The waiter is gone either way (timeout or aborted run); never leave
		// the request pending, or the UI keeps offering a question nobody is
		// waiting on.
		timedOut := errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
		reason := "user input aborted"
		if timedOut {
			reason = "user input timed out"
		}
		cancelCtx, cancelCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelCancel()
		canceled, cancelErr := s.userInput.Cancel(cancelCtx, userinput.CancelInput{
			RequestID:              req.ID,
			ActorChannelIdentityID: session.ChannelIdentityID,
			Reason:                 reason,
		})
		if cancelErr != nil {
			// Cancel can lose to a real answer even when the parent run was
			// aborted. If the user beat cleanup, deliver that answer instead
			// of reporting the cleanup race as a tool failure.
			if late, waitErr := s.userInput.WaitForRegisteredResponse(cancelCtx, req.ID); waitErr == nil &&
				late.Status != userinput.StatusPending && len(late.Result) > 0 {
				return mcp.BuildToolSuccessResult(late.Result), nil
			}
		}
		if !timedOut {
			if cancelErr != nil && s.logger != nil {
				s.logger.Warn("cancel pending user input after aborted wait failed",
					slog.String("request_id", req.ID), slog.Any("error", cancelErr))
			}
			return nil, err
		}
		if cancelErr != nil {
			return nil, cancelErr
		}
		return mcp.BuildToolSuccessResult(canceled.Result), nil
	}
	return mcp.BuildToolSuccessResult(resolved.Result), nil
}

type nativeApprovalResult struct {
	approved bool
	message  string
}

func (s *NativeToolSource) requireApproval(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (nativeApprovalResult, error) {
	if s == nil || s.approval == nil {
		return nativeApprovalResult{approved: true}, nil
	}
	input := toolapproval.CreatePendingInput{
		BotID:                        session.BotID,
		SessionID:                    session.SessionID,
		RouteID:                      session.RouteID,
		ChannelIdentityID:            session.ChannelIdentityID,
		RequestedByChannelIdentityID: session.ChannelIdentityID,
		ToolCallID:                   "mcp-" + uuid.NewString(),
		ToolName:                     toolName,
		ToolInput:                    arguments,
		SourcePlatform:               session.CurrentPlatform,
		ReplyTarget:                  session.ReplyTarget,
		ConversationType:             session.ConversationType,
	}
	eval, err := s.approval.EvaluatePolicy(ctx, input)
	if err != nil {
		return nativeApprovalResult{}, err
	}
	if eval.Decision == toolapproval.DecisionBypass {
		return nativeApprovalResult{approved: true}, nil
	}

	req, err := s.approval.CreatePending(ctx, input)
	if err != nil {
		return nativeApprovalResult{}, err
	}
	if strings.TrimSpace(session.StreamID) == "" {
		reason := "tool execution requires approval, but this ACP tool call is not attached to an interactive stream"
		rejected, rejectErr := s.approval.Reject(ctx, req.ID, session.ChannelIdentityID, reason)
		if rejectErr != nil {
			return nativeApprovalResult{}, rejectErr
		}
		return nativeApprovalResult{message: rejectedToolApprovalText(rejected.DecisionReason)}, nil
	}

	s.publishToolApprovalRequest(session, req)
	waitCtx, cancel := context.WithTimeout(ctx, nativeToolApprovalWaitTimeout)
	defer cancel()
	decided, err := s.approval.WaitForDecision(waitCtx, req.ID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			rejectCtx, rejectCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer rejectCancel()
			rejected, rejectErr := s.approval.Reject(rejectCtx, req.ID, session.ChannelIdentityID, "tool approval timed out")
			if rejectErr != nil {
				return nativeApprovalResult{}, rejectErr
			}
			return nativeApprovalResult{message: rejectedToolApprovalText(rejected.DecisionReason)}, nil
		}
		return nativeApprovalResult{}, err
	}
	switch strings.ToLower(strings.TrimSpace(decided.Status)) {
	case toolapproval.StatusApproved:
		return nativeApprovalResult{approved: true}, nil
	case toolapproval.StatusRejected:
		return nativeApprovalResult{message: rejectedToolApprovalText(decided.DecisionReason)}, nil
	default:
		msg := "tool execution was not approved"
		if status := strings.TrimSpace(decided.Status); status != "" {
			msg += ": " + status
		}
		return nativeApprovalResult{message: msg}, nil
	}
}

// emitUserInputRequest delivers the pending question over the same tool event
// channel as the gateway's tool_call_start, so the stream converter attaches
// it to the existing tool call block — exactly like the in-process agent loop.
func (s *NativeToolSource) emitUserInputRequest(session mcp.ToolSessionContext, req userinput.Request) bool {
	if s == nil || s.toolEvents == nil {
		return false
	}
	delivered := s.toolEvents.AppendToolEvent(session, mcp.ToolStreamEvent{
		Type:        "user_input_request",
		ToolCallID:  req.ToolCallID,
		ToolName:    req.ToolName,
		Input:       req.Input,
		UserInputID: req.ID,
		ShortID:     req.ShortID,
		Status:      userinput.StatusPending,
		Metadata:    userinput.DeferredMetadata(req),
	})
	if !delivered && s.logger != nil {
		s.logger.Warn("user input request not delivered to prompt stream",
			slog.String("request_id", req.ID),
			slog.String("stream_id", session.StreamID))
	}
	return delivered
}

func (s *NativeToolSource) publishToolApprovalRequest(session mcp.ToolSessionContext, req toolapproval.Request) {
	if s == nil || s.publisher == nil {
		return
	}
	streamID := strings.TrimSpace(session.StreamID)
	sessionID := strings.TrimSpace(session.SessionID)
	botID := strings.TrimSpace(session.BotID)
	if streamID == "" || sessionID == "" || botID == "" {
		return
	}

	running := false
	messageID := 1000000 + req.ShortID
	message := map[string]any{
		"id":           messageID,
		"type":         "tool",
		"name":         req.ToolName,
		"input":        req.ToolInput,
		"tool_call_id": req.ToolCallID,
		"running":      &running,
		"approval": map[string]any{
			"approval_id": req.ID,
			"short_id":    req.ShortID,
			"status":      toolapproval.StatusPending,
			"can_approve": true,
		},
	}
	s.publishAgentStream(botID, sessionID, map[string]any{
		"type":       "start",
		"stream_id":  streamID,
		"session_id": sessionID,
	})
	s.publishAgentStream(botID, sessionID, map[string]any{
		"type":       "message",
		"stream_id":  streamID,
		"session_id": sessionID,
		"data":       message,
	})
	s.publishAgentStream(botID, sessionID, map[string]any{
		"type":       "end",
		"stream_id":  streamID,
		"session_id": sessionID,
	})
}

func (s *NativeToolSource) publishAgentStream(botID, sessionID string, stream map[string]any) {
	payload := map[string]any{
		"session_id": sessionID,
		"stream":     stream,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.publisher.Publish(messageevent.Event{
		Type:  messageevent.EventTypeAgentStream,
		BotID: botID,
		Data:  data,
	})
}

func rejectedToolApprovalText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "tool execution rejected by user"
	}
	return "tool execution rejected by user: " + reason
}

func (s *NativeToolSource) loadTools(ctx context.Context, session mcp.ToolSessionContext) []sdk.Tool {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	providers := append([]ToolProvider(nil), s.providers...)
	s.mu.RUnlock()
	toolSession := SessionContext{
		BotID:             session.BotID,
		ChatID:            firstNonEmpty(session.ChatID, session.BotID),
		SessionID:         session.SessionID,
		SessionType:       session.SessionType,
		ChannelIdentityID: session.ChannelIdentityID,
		SessionToken:      session.SessionToken,
		CurrentPlatform:   session.CurrentPlatform,
		ReplyTarget:       session.ReplyTarget,
		ConversationType:  session.ConversationType,
		IsSubagent:        session.IsSubagent,
	}
	var out []sdk.Tool
	for _, provider := range providers {
		providerTools, err := provider.Tools(ctx, toolSession)
		if err != nil {
			s.logger.Warn("native tool provider failed", slog.Any("error", err))
			continue
		}
		out = append(out, providerTools...)
	}
	return out
}

func (s *NativeToolSource) allowed(name string) bool {
	if s == nil {
		return false
	}
	if s.allowAll {
		return strings.TrimSpace(name) != ""
	}
	if len(s.allow) == 0 {
		return false
	}
	_, ok := s.allow[strings.TrimSpace(name)]
	return ok
}

func toolInputSchema(parameters any) map[string]any {
	if parameters == nil {
		return emptyObjectSchema()
	}
	if schema, ok := parameters.(map[string]any); ok && schema != nil {
		return schema
	}
	raw, err := json.Marshal(parameters)
	if err != nil {
		return emptyObjectSchema()
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return emptyObjectSchema()
	}
	if strings.TrimSpace(StringArg(schema, "type")) == "" {
		schema["type"] = "object"
	}
	return schema
}
