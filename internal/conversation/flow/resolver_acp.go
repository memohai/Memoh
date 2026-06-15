package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
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
	req = r.persistACPLeadingUserMessage(context.WithoutCancel(ctx), req)
	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	activePrompt := r.registerACPActivePrompt(req.BotID, req.SessionID)
	defer r.unregisterACPActivePrompt(req.BotID, req.SessionID, activePrompt)
	go func() {
		select {
		case <-abortCh:
			cancel()
		case <-streamCtx.Done():
		}
	}()

	var (
		projectedMu       sync.Mutex
		projectedStatuses = map[string]string{}
	)
	recordProjectionStatus := func(ev agentpkg.StreamEvent) bool {
		toolCallID := strings.TrimSpace(ev.ToolCallID)
		if toolCallID == "" {
			return false
		}
		status := acpDecisionProjectionStatus(ev)
		projectedMu.Lock()
		defer projectedMu.Unlock()
		if projectedStatuses[toolCallID] == status {
			return false
		}
		projectedStatuses[toolCallID] = status
		return true
	}
	releaseProjection := func(toolCallID string) {
		toolCallID = strings.TrimSpace(toolCallID)
		if toolCallID == "" {
			return
		}
		projectedMu.Lock()
		delete(projectedStatuses, toolCallID)
		projectedMu.Unlock()
	}
	projectedSnapshot := func() map[string]struct{} {
		projectedMu.Lock()
		defer projectedMu.Unlock()
		if len(projectedStatuses) == 0 {
			return nil
		}
		out := make(map[string]struct{}, len(projectedStatuses))
		for id := range projectedStatuses {
			out[id] = struct{}{}
		}
		return out
	}

	emit := func(ev agentpkg.StreamEvent) {
		if isACPDecisionProjectionEvent(ev) && recordProjectionStatus(ev) {
			if !r.persistACPDecisionProjection(context.WithoutCancel(ctx), req, ev) {
				releaseProjection(ev.ToolCallID)
			}
		}
		if activePrompt != nil {
			activePrompt.emit(ev)
		}
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		select {
		case eventCh <- json.RawMessage(data):
		case <-streamCtx.Done():
		}
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventStart})
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
		// ACP/native MCP does not yet have the in-process read-media decoration
		// path that turns read image bytes into model-native image input. Keep
		// this false until ACP model capability and image transport are wired.
		SupportsImageInput: false,
		ToolHTTPURL:        req.ToolHTTPURL,
		ContextURI:         acpContextURI,
		ContextMarkdown:    contextMarkdown,
		Sink:               acpclient.EventSinkFunc(emit),
	})
	if err != nil {
		r.logger.Error("ACP prompt failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		r.cancelPendingACPApprovals(context.WithoutCancel(ctx), req, "tool approval cancelled: the turn ended before a decision arrived")
		failedResult, failureDelta := acpFailureResult(result, err)
		projected := projectedSnapshot()
		failedResult.Output = filterACPProjectedOutput(failedResult.Output, projected)
		if failureDelta != "" {
			emit(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: failureDelta})
		}
		_ = r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, failedResult, err)
		emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
		emit(agentpkg.StreamEvent{Type: agentpkg.EventAbort})
		return nil
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
	projected := projectedSnapshot()
	result.Output = filterACPProjectedOutput(result.Output, projected)
	if err := r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, result, nil); err != nil {
		r.logger.Error("ACP persist failed", slog.Any("error", err), slog.String("session_id", req.SessionID))
	}
	emit(agentpkg.StreamEvent{Type: agentpkg.EventEnd})
	return nil
}

func isACPDecisionProjectionEvent(ev agentpkg.StreamEvent) bool {
	switch ev.Type {
	case agentpkg.EventUserInputRequest, agentpkg.EventToolApprovalRequest:
		return strings.TrimSpace(ev.ToolCallID) != ""
	default:
		return false
	}
}

func acpDecisionProjectionStatus(ev agentpkg.StreamEvent) string {
	status := strings.ToLower(strings.TrimSpace(ev.Status))
	if status == "" {
		return "pending"
	}
	return status
}

func (r *Resolver) persistACPLeadingUserMessage(ctx context.Context, req conversation.ChatRequest) conversation.ChatRequest {
	if req.UserMessagePersisted || r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" {
		return req
	}
	displayText := strings.TrimSpace(req.RawQuery)
	if displayText == "" {
		displayText = strings.TrimSpace(req.Query)
	}
	if displayText == "" && len(req.Attachments) == 0 {
		return req
	}
	contentText := strings.TrimSpace(req.Query)
	if contentText == "" {
		contentText = displayText
	}
	content, err := json.Marshal(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(contentText),
	})
	if err != nil {
		r.logger.Warn("persist ACP leading user message: marshal failed", slog.Any("error", err))
		return req
	}
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	if _, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
		BotID:                   req.BotID,
		SessionID:               req.SessionID,
		SenderChannelIdentityID: senderChannelIdentityID,
		SenderUserID:            senderUserID,
		ExternalMessageID:       req.ExternalMessageID,
		SourceReplyToMessageID:  req.SourceReplyToMessageID,
		Role:                    "user",
		Content:                 content,
		Metadata:                mergeMetadata(buildRouteMetadata(req), buildInteractionMetadata(req)),
		Assets:                  chatAttachmentsToAssetRefs(req.Attachments),
		EventID:                 req.EventID,
		DisplayText:             displayText,
	}); err != nil {
		r.logger.Warn("persist ACP leading user message failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err))
		return req
	}
	req.UserMessagePersisted = true
	return req
}

func (r *Resolver) persistACPDecisionProjection(ctx context.Context, req conversation.ChatRequest, ev agentpkg.StreamEvent) bool {
	if r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.SessionID) == "" {
		return false
	}
	output := sdkMessagesToModelMessages(acpclient.TranscriptFromEvents([]event.StreamEvent{ev}, ""))
	for _, msg := range output {
		if msg.Role != "assistant" {
			continue
		}
		content, err := json.Marshal(msg)
		if err != nil {
			r.logger.Warn("persist ACP decision projection: marshal failed",
				slog.String("tool_call_id", ev.ToolCallID),
				slog.Any("error", err))
			return false
		}
		if _, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:                   req.BotID,
			SessionID:               req.SessionID,
			SenderChannelIdentityID: "",
			Role:                    "assistant",
			Content:                 content,
			Metadata:                buildRouteMetadata(req),
		}); err != nil {
			r.logger.Warn("persist ACP decision projection failed",
				slog.String("bot_id", req.BotID),
				slog.String("session_id", req.SessionID),
				slog.String("tool_call_id", ev.ToolCallID),
				slog.Any("error", err))
			return false
		}
		return true
	}
	return false
}

func filterACPProjectedOutput(messages []sdk.Message, projected map[string]struct{}) []sdk.Message {
	if len(messages) == 0 || len(projected) == 0 {
		return messages
	}
	out := make([]sdk.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != sdk.MessageRoleAssistant {
			out = append(out, msg)
			continue
		}
		content := make([]sdk.MessagePart, 0, len(msg.Content))
		changed := false
		for _, part := range msg.Content {
			call, ok := part.(sdk.ToolCallPart)
			if !ok {
				content = append(content, part)
				continue
			}
			if _, skip := projected[strings.TrimSpace(call.ToolCallID)]; skip {
				changed = true
				continue
			}
			content = append(content, part)
		}
		if changed {
			if len(content) == 0 {
				continue
			}
			msg.Content = content
		}
		out = append(out, msg)
	}
	return out
}

// cancelPendingACPApprovals closes the residual approval window when a turn
// dies abnormally: any pending row for the session belonged to that turn (the
// pool's turn slot guarantees one turn per session), and its waiter is gone -
// left pending, the persisted card would stay actionable forever and a late
// approve would flip a row nobody executes.
func (r *Resolver) cancelPendingACPApprovals(ctx context.Context, req conversation.ChatRequest, reason string) {
	if r == nil || r.toolApproval == nil {
		return
	}
	cancelled, err := r.toolApproval.CancelPendingForSession(ctx, req.BotID, req.SessionID, reason)
	if err != nil {
		r.logger.Warn("cancel pending ACP approvals failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err))
		return
	}
	if len(cancelled) > 0 {
		r.logger.Info("cancelled pending ACP approvals with their turn",
			slog.String("session_id", req.SessionID),
			slog.Int("count", len(cancelled)))
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
	// result.Output is already assembled by the ACP client; the resolver only
	// converts and stores it.
	output := sdkMessagesToModelMessages(result.Output)
	if len(output) == 0 {
		output = []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("")}}
	}
	round := make([]conversation.ModelMessage, 0, 1+len(output))
	round = append(round, conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(req.Query)})
	round = append(round, output...)

	metadataByIndex := make(map[int]map[string]any, len(output))
	metadataOffset := 1
	if req.UserMessagePersisted {
		metadataOffset = 0
	}
	for idx, msg := range output {
		if msg.Role == "assistant" {
			metadataByIndex[idx+metadataOffset] = meta
		}
	}
	skipMemory := promptErr != nil || req.UserMessagePersisted
	err := r.storeRoundWithOptions(ctx, req, round, "", storeRoundOptions{
		SkipMemory:              skipMemory,
		AllowEmptyAssistantText: true,
		MessageMetadataByIndex:  metadataByIndex,
	})
	if err == nil && promptErr == nil && req.UserMessagePersisted {
		go r.storeMemory(context.WithoutCancel(ctx), req, round)
	}
	return err
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
		result.Events = append(result.Events, event.StreamEvent{Type: event.TextDelta, Delta: delta})
		result.Output = acpclient.AppendTranscriptText(result.Output, message)
		return result, delta
	}
	result.Text = message
	result.Events = append(result.Events, event.StreamEvent{Type: event.TextDelta, Delta: message})
	result.Output = acpclient.AppendTranscriptText(result.Output, message)
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
