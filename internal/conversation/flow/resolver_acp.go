package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/userinput"
)

type acpPrompter interface {
	Prompt(ctx context.Context, input acpagent.PromptInput) (acpclient.PromptResult, error)
}

func (r *Resolver) SetACPSessionPool(pool acpPrompter) {
	r.acpPool = pool
}

func (r *Resolver) isACPAgentSession(ctx context.Context, req conversation.ChatRequest) (bool, error) {
	if r == nil || r.sessionService == nil || strings.TrimSpace(req.SessionID) == "" {
		return false, nil
	}
	sess, err := r.sessionService.Get(ctx, req.SessionID)
	if err != nil {
		return false, err
	}
	return sess.Type == session.TypeACPAgent, nil
}

func (r *Resolver) streamACPAgentWS(ctx context.Context, req conversation.ChatRequest, eventCh chan<- WSStreamEvent, abortCh <-chan struct{}) error {
	if r.acpPool == nil {
		return errors.New("ACP session pool is not configured")
	}
	sess, err := r.sessionService.Get(ctx, req.SessionID)
	if err != nil {
		return err
	}
	agentID := metadataString(sess.Metadata, "acp_agent_id")
	projectPath := metadataString(sess.Metadata, "project_path")
	contextMarkdown := r.buildACPContextMarkdown(ctx, req, agentID, projectPath)

	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	req.Query = strings.TrimSpace(req.Query)
	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-abortCh:
			cancel()
		case <-streamCtx.Done():
		}
	}()

	emit := func(event agentpkg.StreamEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		select {
		case eventCh <- json.RawMessage(data):
		case <-streamCtx.Done():
		}
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventAgentStart})
	// No eager text_start here: the UI message converter allocates block IDs
	// in arrival order and the frontend sorts by ID, so pre-creating the text
	// block would pin the answer text above any reasoning that streams first.
	// The first text_delta lazily creates the text block instead.

	result, err := r.acpPool.Prompt(streamCtx, acpagent.PromptInput{
		BotID:             req.BotID,
		ChatID:            req.ChatID,
		SessionID:         req.SessionID,
		StreamID:          req.StreamID,
		RouteID:           req.RouteID,
		AgentID:           agentID,
		ProjectPath:       projectPath,
		Prompt:            req.Query,
		ChannelIdentityID: req.SourceChannelIdentityID,
		SessionToken:      req.Token,
		CurrentPlatform:   req.CurrentChannel,
		ReplyTarget:       req.ReplyTarget,
		ConversationType:  req.ConversationType,
		ToolHTTPURL:       req.ToolHTTPURL,
		ContextURI:        acpContextURI,
		ContextMarkdown:   contextMarkdown,
		Sink: acpclient.EventSinkFunc(func(event acpclient.StreamEvent) {
			for _, mapped := range mapACPStreamEvent(event) {
				emit(mapped)
			}
		}),
	})
	if err != nil {
		r.logger.Error("ACP prompt failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		failedResult, failureDelta := acpFailureResult(result, err)
		if failureDelta != "" {
			emit(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: failureDelta})
		}
		_ = r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, failedResult, err)
		emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
		emit(agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
		return nil
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
	if err := r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, result, nil); err != nil {
		r.logger.Error("ACP persist failed", slog.Any("error", err), slog.String("session_id", req.SessionID))
	}
	emit(agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd})
	return nil
}

func mapACPStreamEvent(event acpclient.StreamEvent) []agentpkg.StreamEvent {
	switch event.Type {
	case acpclient.StreamEventTextDelta:
		if event.Delta == "" {
			return nil
		}
		return []agentpkg.StreamEvent{{Type: agentpkg.EventTextDelta, Delta: event.Delta}}
	case acpclient.StreamEventReasoningDelta:
		if event.Delta == "" {
			return nil
		}
		return []agentpkg.StreamEvent{{Type: agentpkg.EventReasoningDelta, Delta: event.Delta}}
	case acpclient.StreamEventToolCallStart:
		return []agentpkg.StreamEvent{{
			Type:       agentpkg.EventToolCallStart,
			ToolName:   event.ToolName,
			ToolCallID: event.ToolCallID,
			Input:      event.Input,
		}}
	case acpclient.StreamEventToolCallEnd:
		return []agentpkg.StreamEvent{{
			Type:       agentpkg.EventToolCallEnd,
			ToolName:   event.ToolName,
			ToolCallID: event.ToolCallID,
			Input:      event.Input,
			Result:     event.Result,
			Error:      event.Error,
		}}
	case acpclient.StreamEventUserInputRequest:
		// Same shape the in-process agent loop emits, so the stream converter
		// attaches the pending question to the existing tool call block.
		return []agentpkg.StreamEvent{{
			Type:        agentpkg.EventUserInputRequest,
			ToolName:    event.ToolName,
			ToolCallID:  event.ToolCallID,
			Input:       event.Input,
			UserInputID: event.UserInputID,
			ShortID:     event.ShortID,
			Status:      event.Status,
			Metadata:    event.Metadata,
		}}
	default:
		return nil
	}
}

func (r *Resolver) persistACPRound(ctx context.Context, req conversation.ChatRequest, agentID, projectPath string, result acpclient.PromptResult, promptErr error) error {
	meta := map[string]any{
		"acp_agent_id": agentID,
		"project_path": projectPath,
		"stop_reason":  result.StopReason,
	}
	if promptErr != nil {
		meta["error"] = promptErr.Error()
	}
	output := acpResultOutputMessages(result)
	round := make([]conversation.ModelMessage, 0, 1+len(output))
	round = append(round, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(req.Query)})
	round = append(round, output...)

	metadataByIndex := make(map[int]map[string]any, len(output))
	for idx, msg := range output {
		if msg.Role == "assistant" {
			metadataByIndex[idx+1] = meta
		}
	}
	return r.storeRoundWithOptions(ctx, req, round, "", storeRoundOptions{
		SkipMemory:              promptErr != nil,
		AllowEmptyAssistantText: true,
		MessageMetadataByIndex:  metadataByIndex,
	})
}

func acpResultOutputMessages(result acpclient.PromptResult) []conversation.ModelMessage {
	output := make([]conversation.ModelMessage, 0)
	assistantParts := make([]sdk.MessagePart, 0)
	var reasoning strings.Builder
	var text strings.Builder
	sawTextDelta := false

	flushReasoning := func() {
		if reasoning.Len() == 0 {
			return
		}
		assistantParts = append(assistantParts, sdk.ReasoningPart{Text: reasoning.String()})
		reasoning.Reset()
	}
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		assistantParts = append(assistantParts, sdk.TextPart{Text: text.String()})
		text.Reset()
	}
	assistantHasToolCall := func() bool {
		for _, part := range assistantParts {
			if _, ok := part.(sdk.ToolCallPart); ok {
				return true
			}
		}
		return false
	}
	flushAssistant := func() {
		flushReasoning()
		flushText()
		if len(assistantParts) == 0 {
			return
		}
		converted := sdkMessagesToModelMessages([]sdk.Message{{
			Role:    sdk.MessageRoleAssistant,
			Content: assistantParts,
		}})
		output = append(output, converted...)
		assistantParts = assistantParts[:0]
	}
	appendText := func(delta string) {
		if delta == "" {
			return
		}
		if assistantHasToolCall() {
			flushAssistant()
		}
		sawTextDelta = true
		text.WriteString(delta)
	}
	appendReasoning := func(delta string) {
		if delta == "" {
			return
		}
		if text.Len() > 0 || assistantHasToolCall() {
			flushAssistant()
		}
		reasoning.WriteString(delta)
	}
	appendToolResult := func(event acpclient.StreamEvent) {
		result := event.Result
		isError := strings.TrimSpace(event.Error) != ""
		if result == nil && isError {
			result = strings.TrimSpace(event.Error)
		}
		converted := sdkMessagesToModelMessages([]sdk.Message{
			sdk.ToolMessage(sdk.ToolResultPart{
				ToolCallID: strings.TrimSpace(event.ToolCallID),
				ToolName:   strings.TrimSpace(event.ToolName),
				Result:     result,
				IsError:    isError,
			}),
		})
		output = append(output, converted...)
	}
	attachToolMetadata := func(event acpclient.StreamEvent, key string, value map[string]any) {
		toolCallID := strings.TrimSpace(event.ToolCallID)
		toolName := strings.TrimSpace(event.ToolName)
		for idx, part := range assistantParts {
			toolCall, ok := part.(sdk.ToolCallPart)
			if !ok {
				continue
			}
			if toolCallID != "" && strings.TrimSpace(toolCall.ToolCallID) != toolCallID {
				continue
			}
			if toolCallID == "" && toolName != "" && strings.TrimSpace(toolCall.ToolName) != toolName {
				continue
			}
			if toolCall.ProviderMetadata == nil {
				toolCall.ProviderMetadata = map[string]any{}
			}
			toolCall.ProviderMetadata[key] = value
			if event.Input != nil {
				toolCall.Input = event.Input
			}
			assistantParts[idx] = toolCall
			return
		}
		if text.Len() > 0 || reasoning.Len() > 0 {
			flushAssistant()
		}
		assistantParts = append(assistantParts, sdk.ToolCallPart{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Input:      event.Input,
			ProviderMetadata: map[string]any{
				key: value,
			},
		})
	}
	userInputMetadata := func(event acpclient.StreamEvent) map[string]any {
		status := strings.TrimSpace(event.Status)
		if status == "" {
			status = userinput.StatusPending
		}
		userInputID := strings.TrimSpace(event.UserInputID)
		if userInputID == "" {
			if value, _ := event.Metadata["user_input_id"].(string); value != "" {
				userInputID = strings.TrimSpace(value)
			}
		}
		return map[string]any{
			"user_input_id": userInputID,
			"short_id":      event.ShortID,
			"status":        status,
			"ui_payload":    event.Metadata["ui_payload"],
		}
	}
	for _, event := range result.Events {
		switch event.Type {
		case acpclient.StreamEventReasoningDelta:
			appendReasoning(event.Delta)
		case acpclient.StreamEventTextDelta:
			appendText(event.Delta)
		case acpclient.StreamEventToolCallStart:
			flushReasoning()
			flushText()
			assistantParts = append(assistantParts, sdk.ToolCallPart{
				ToolCallID: strings.TrimSpace(event.ToolCallID),
				ToolName:   strings.TrimSpace(event.ToolName),
				Input:      event.Input,
			})
		case acpclient.StreamEventToolCallEnd:
			flushAssistant()
			appendToolResult(event)
		case acpclient.StreamEventUserInputRequest:
			attachToolMetadata(event, "user_input", userInputMetadata(event))
		}
	}
	if !sawTextDelta {
		appendText(strings.TrimSpace(result.Text))
	}
	flushAssistant()

	if len(output) == 0 {
		return []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("")}}
	}
	return output
}

// acpFailureResult appends the raw upstream error (truncated, single-line) to
// the partial result so users see what went wrong inline. The frontend is
// responsible for any i18n "ACP agent failed" prefix; the backend only
// surfaces the technical detail.
func acpFailureResult(result acpclient.PromptResult, err error) (acpclient.PromptResult, string) {
	message := truncateOneLineError(err)
	if message == "" {
		return result, ""
	}
	if strings.TrimSpace(result.Text) != "" {
		delta := "\n\n" + message
		result.Text = strings.TrimSpace(result.Text + delta)
		result.Events = append(result.Events, acpclient.StreamEvent{Type: acpclient.StreamEventTextDelta, Delta: delta})
		return result, delta
	}
	result.Text = message
	result.Events = append(result.Events, acpclient.StreamEvent{Type: acpclient.StreamEventTextDelta, Delta: message})
	return result, message
}

func truncateOneLineError(err error) string {
	if err == nil {
		return ""
	}
	message := oneLine(err.Error())
	if message == "" {
		return ""
	}
	const maxRunes = 500
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "..."
	}
	return message
}

func oneLine(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
