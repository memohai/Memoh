package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpfeedback"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/apperror"
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

type acpPrompter interface {
	Prompt(ctx context.Context, input acpagent.PromptInput) (acpclient.PromptResult, error)
}

type acpPreparedAttachments struct {
	Images                   []acpclient.PromptImage
	Context                  []conversation.ChatAttachment
	References               []string
	CanFallbackImagesToFiles bool
}

type ACPSessionExecutionInfo struct {
	IsACP                 bool
	BotID                 string
	Type                  string
	RuntimeType           string
	CreatedByUserID       string
	AgentID               string
	ProjectPath           string
	RuntimeOwnerAccountID string
}

func (r *Resolver) SetACPSessionPool(pool acpPrompter) {
	r.acpPool = pool
}

func (r *Resolver) ACPSessionExecutionInfo(ctx context.Context, sessionID string) (ACPSessionExecutionInfo, error) {
	if r == nil || r.sessionService == nil || strings.TrimSpace(sessionID) == "" {
		return ACPSessionExecutionInfo{}, nil
	}
	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		return ACPSessionExecutionInfo{}, err
	}
	if !session.IsACPRuntime(sess) {
		return ACPSessionExecutionInfo{}, nil
	}
	acpMeta := mergeACPRuntimeMetadata(sess.Metadata, sess.RuntimeMetadata)
	return ACPSessionExecutionInfo{
		IsACP:                 true,
		BotID:                 sess.BotID,
		Type:                  sess.Type,
		RuntimeType:           sess.RuntimeType,
		CreatedByUserID:       sess.CreatedByUserID,
		AgentID:               metadataString(acpMeta, "acp_agent_id"),
		ProjectPath:           metadataString(acpMeta, "project_path"),
		RuntimeOwnerAccountID: metadataString(acpMeta, "runtime_owner_account_id"),
	}, nil
}

func (r *Resolver) isACPAgentSession(ctx context.Context, req conversation.ChatRequest) (bool, error) {
	if r == nil || r.sessionService == nil || strings.TrimSpace(req.SessionID) == "" {
		return false, nil
	}
	sess, err := r.sessionService.Get(ctx, req.SessionID)
	if err != nil {
		return false, err
	}
	if err := validateSessionBot(req.BotID, req.SessionID, sess.BotID); err != nil {
		return false, err
	}
	return session.IsACPRuntime(sess), nil
}

func (r *Resolver) streamACPAgentWS(ctx context.Context, req conversation.ChatRequest, eventCh chan<- WSStreamEvent, abortCh <-chan struct{}) error {
	if r.acpPool == nil {
		return errors.New("ACP session pool is not configured")
	}
	sess, err := r.sessionService.Get(ctx, req.SessionID)
	if err != nil {
		return err
	}
	if err := validateSessionBot(req.BotID, req.SessionID, sess.BotID); err != nil {
		return err
	}
	acpMeta := mergeACPRuntimeMetadata(sess.Metadata, sess.RuntimeMetadata)
	agentID := metadataString(acpMeta, "acp_agent_id")
	projectPath := metadataString(acpMeta, "project_path")
	runtimeOwnerAccountID := metadataString(acpMeta, "runtime_owner_account_id")
	if runtimeOwnerAccountID == "" {
		return acpfeedback.New(
			acpfeedback.CodeRuntimeOwnerMissing,
			"missing_runtime_owner",
			409,
			"chat.acp.runtimeOwnerMissing",
			"ACP runtime owner is missing; recreate or reauthorize the ACP session",
			nil,
		)
	}
	if err := r.requireACPRuntimeOwnerWorkspaceExec(ctx, req.BotID, runtimeOwnerAccountID); err != nil {
		return err
	}
	doneTurn, entered := r.tryEnterIdleSessionTurn(ctx, req.BotID, req.SessionID)
	if !entered {
		return acpfeedback.New(
			acpfeedback.CodeRuntimeBusy,
			"runtime_busy",
			409,
			"chat.acp.runtimeBusy",
			"External agent runtime is already processing a turn for this session.",
			nil,
		)
	}
	defer doneTurn()

	preparedAttachments, err := r.prepareACPAttachments(ctx, req)
	if err != nil {
		return err
	}
	contextReq := req
	contextReq.Attachments = preparedAttachments.Context
	contextReq.ReplyAttachments = nil
	contextMarkdown := r.buildACPContextMarkdown(ctx, contextReq, agentID, projectPath)

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	req.Query = strings.TrimSpace(req.Query)
	var leadingUser *messagepkg.Message
	req, leadingUser, err = r.persistACPLeadingUserMessage(context.WithoutCancel(ctx), req)
	if err != nil {
		return err
	}
	cleanupLeadingUser := func() {
		if leadingUser != nil {
			if cleanupErr := r.cleanupPersistedRound(context.WithoutCancel(ctx), []messagepkg.Message{*leadingUser}); cleanupErr != nil {
				r.logger.Warn("cleanup ACP leading user message failed", slog.Any("error", cleanupErr))
			}
		}
	}
	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.RawQuery)

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
		projectedMu          sync.Mutex
		projectedStatuses    = map[string]string{}
		projectionPersistErr error
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
	projectionError := func() error {
		projectedMu.Lock()
		defer projectedMu.Unlock()
		return projectionPersistErr
	}

	emit := func(ev agentpkg.StreamEvent) {
		if isACPDecisionProjectionEvent(ev) && recordProjectionStatus(ev) {
			persisted, persistErr := r.persistACPDecisionProjection(context.WithoutCancel(ctx), req, ev)
			if persistErr != nil {
				projectedMu.Lock()
				if projectionPersistErr == nil {
					projectionPersistErr = persistErr
				}
				projectedMu.Unlock()
				releaseProjection(ev.ToolCallID)
				cancel()
				return
			}
			if !persisted {
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
		BotID:                    req.BotID,
		ChatID:                   req.ChatID,
		SessionID:                req.SessionID,
		StreamID:                 req.StreamID,
		RouteID:                  req.RouteID,
		AgentID:                  agentID,
		ProjectPath:              projectPath,
		ModelID:                  strings.TrimSpace(req.Model),
		ReasoningEffort:          strings.TrimSpace(req.ReasoningEffort),
		Prompt:                   req.Query,
		Images:                   preparedAttachments.Images,
		AttachmentReferences:     preparedAttachments.References,
		CanFallbackImagesToFiles: preparedAttachments.CanFallbackImagesToFiles,
		ChannelIdentityID:        req.SourceChannelIdentityID,
		SessionToken:             req.Token,
		CurrentPlatform:          req.CurrentChannel,
		ReplyTarget:              req.ReplyTarget,
		ConversationType:         req.ConversationType,
		CanRequestUserInput:      r.canDeliverUserInputWS(eventCh),
		// This flag controls image bytes returned later by the read-media MCP
		// tool. Initial user images use ACP ImageBlock transport above.
		SupportsImageInput:    false,
		ToolOutputLimit:       r.toolOutputLimit(),
		ToolHTTPURL:           req.ToolHTTPURL,
		ContextURI:            acpContextURI,
		ContextMarkdown:       contextMarkdown,
		RuntimeOwnerAccountID: runtimeOwnerAccountID,
		ForceFreshRuntime:     req.ForceFreshRuntime,
		RuntimeGuard:          persistenceGuardFromContext(streamCtx),
		Sink:                  acpclient.EventSinkFunc(emit),
	})
	if persistErr := projectionError(); persistErr != nil {
		return fmt.Errorf("persist ACP decision projection: %w", persistErr)
	}
	if err != nil {
		r.logger.Error("ACP prompt failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		if cancelErr := r.cancelPendingACPApprovals(context.WithoutCancel(ctx), req, "tool approval cancelled: the turn ended before a decision arrived"); cancelErr != nil {
			return cancelErr
		}
		var feedbackErr *acpfeedback.Error
		if errors.As(err, &feedbackErr) {
			cleanupLeadingUser()
			return err
		}
		if appErr := acpPromptConfigAppError(err); appErr != nil {
			cleanupLeadingUser()
			return appErr
		}
		if feedbackErr := acpPromptInputFeedback(err); feedbackErr != nil {
			cleanupLeadingUser()
			return feedbackErr
		}
		result = ensureACPPromptOutput(result)
		failedResult, failureDelta := acpFailureResult(result, err)
		projected := projectedSnapshot()
		failedResult.Output = filterACPProjectedOutput(failedResult.Output, projected)
		if failureDelta != "" {
			emit(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: failureDelta})
		}
		if persistErr := r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, failedResult, err); persistErr != nil {
			return fmt.Errorf("persist failed ACP round: %w", persistErr)
		}
		emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
		emit(acpTerminalStreamEvent(agentpkg.EventAbort, failedResult))
		return nil
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
	projected := projectedSnapshot()
	result = ensureACPPromptOutput(result)
	result.Output = filterACPProjectedOutput(result.Output, projected)
	if err := r.persistACPRound(context.WithoutCancel(ctx), req, agentID, projectPath, result, nil); err != nil {
		return fmt.Errorf("persist ACP round: %w", err)
	}
	emit(acpTerminalStreamEvent(agentpkg.EventEnd, result))
	return nil
}

func (r *Resolver) prepareACPAttachments(ctx context.Context, req conversation.ChatRequest) (acpPreparedAttachments, error) {
	prepared := r.prepareGatewayAttachments(ctx, req)
	result := acpPreparedAttachments{
		Images:                   make([]acpclient.PromptImage, 0, len(prepared)),
		Context:                  make([]conversation.ChatAttachment, 0, len(prepared)),
		References:               make([]string, 0, len(prepared)),
		CanFallbackImagesToFiles: true,
	}
	for i, item := range prepared {
		attachmentType := strings.ToLower(strings.TrimSpace(item.Type))
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("attachment %d", i+1)
		}

		contextAttachment := conversation.ChatAttachment{
			Type:        attachmentType,
			ContentHash: strings.TrimSpace(item.ContentHash),
			Name:        strings.TrimSpace(item.Name),
			Mime:        attachmentpkg.NormalizeMime(item.Mime),
			Size:        item.Size,
			Metadata:    item.Metadata,
		}
		reference := strings.TrimSpace(item.FallbackPath)
		if reference == "" && item.Transport == gatewayTransportPublicURL {
			reference = strings.TrimSpace(item.Payload)
		}
		if reference != "" {
			if isLikelyPublicURL(reference) {
				contextAttachment.URL = reference
			} else {
				contextAttachment.Path = reference
			}
			result.References = append(result.References, reference)
		}

		if attachmentType == "image" && item.Transport == gatewayTransportInlineDataURL && strings.TrimSpace(item.Payload) != "" {
			image, imageErr := acpPromptImageFromDataURL(item.Payload, item.Mime)
			if imageErr != nil {
				return acpPreparedAttachments{}, acpfeedback.New(
					acpfeedback.CodeAttachmentInvalid,
					"invalid_image_data",
					http.StatusBadRequest,
					"chat.acp.attachmentInvalid",
					"The attachment is invalid. Please attach it again.",
					map[string]string{"name": name},
				)
			}
			result.Images = append(result.Images, image)
			if reference == "" {
				result.CanFallbackImagesToFiles = false
			}
		} else if reference == "" {
			return acpPreparedAttachments{}, acpfeedback.New(
				acpfeedback.CodeAttachmentUnavailable,
				"attachment_not_reachable",
				http.StatusBadRequest,
				"chat.acp.attachmentUnavailable",
				"The attachment could not be made available to the external agent. Please attach it again.",
				map[string]string{"name": name},
			)
		}

		result.Context = append(result.Context, contextAttachment)
	}
	return result, nil
}

func acpPromptImageFromDataURL(payload, fallbackMime string) (acpclient.PromptImage, error) {
	payload = strings.TrimSpace(payload)
	comma := strings.Index(payload, ",")
	if comma < 0 || !strings.HasPrefix(strings.ToLower(payload), "data:") ||
		!strings.Contains(strings.ToLower(payload[:comma]), ";base64") {
		return acpclient.PromptImage{}, acpclient.ErrInvalidPromptImage
	}
	mimeType := attachmentpkg.MimeFromDataURL(payload)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = attachmentpkg.NormalizeMime(fallbackMime)
	}
	normalized, err := acpclient.NormalizePromptImages([]acpclient.PromptImage{{
		Data:     strings.TrimSpace(payload[comma+1:]),
		MimeType: mimeType,
	}})
	if err != nil {
		return acpclient.PromptImage{}, err
	}
	return normalized[0], nil
}

func acpPromptInputFeedback(err error) *acpfeedback.Error {
	switch {
	case errors.Is(err, acpclient.ErrImagePromptUnsupported):
		return acpfeedback.New(
			acpfeedback.CodeImageInputUnsupported,
			"image_input_unsupported",
			http.StatusBadRequest,
			"chat.acp.imageInputUnsupported",
			"This external agent cannot read the attached image.",
			nil,
		)
	case errors.Is(err, acpclient.ErrInvalidPromptImage):
		return acpfeedback.New(
			acpfeedback.CodeAttachmentInvalid,
			"invalid_image_data",
			http.StatusBadRequest,
			"chat.acp.attachmentInvalid",
			"The attachment is invalid. Please attach it again.",
			nil,
		)
	default:
		return nil
	}
}

func acpPromptConfigAppError(err error) error {
	switch {
	case errors.Is(err, acpclient.ErrModelSelectionUnsupported):
		return apperror.New(apperror.CodeACPModelSelectionUnsupported, nil)
	case errors.Is(err, acpclient.ErrModelIDRequired):
		return apperror.New(apperror.CodeACPModelIDRequired, nil)
	case errors.Is(err, acpclient.ErrModelUnavailable):
		return apperror.New(apperror.CodeACPModelUnavailable, nil)
	case errors.Is(err, acpclient.ErrReasoningSelectionUnsupported):
		return apperror.New(apperror.CodeACPReasoningUnsupported, nil)
	case errors.Is(err, acpclient.ErrReasoningEffortRequired):
		return apperror.New(apperror.CodeACPReasoningEffortRequired, nil)
	case errors.Is(err, acpclient.ErrReasoningEffortUnavailable):
		return apperror.New(apperror.CodeACPReasoningUnavailable, nil)
	case errors.Is(err, acpagent.ErrRuntimeConfigUpdateFailed):
		return apperror.Wrap(apperror.CodeACPConfigUpdateFailed, err, nil)
	default:
		return nil
	}
}

func ensureACPPromptOutput(result acpclient.PromptResult) acpclient.PromptResult {
	if len(result.Output) == 0 {
		result.Output = acpclient.TranscriptFromEvents(result.Events, result.Text)
	}
	return result
}

func acpTerminalStreamEvent(eventType agentpkg.StreamEventType, result acpclient.PromptResult) agentpkg.StreamEvent {
	result = ensureACPPromptOutput(result)
	ev := agentpkg.StreamEvent{Type: eventType, HistoryCommitted: true}
	if data, err := json.Marshal(result.Output); err == nil {
		ev.Messages = data
	}
	if result.Usage != nil {
		if data, err := json.Marshal(result.Usage); err == nil {
			ev.Usage = data
		}
	}
	return ev
}

func validateSessionBot(botID, sessionID, sessionBotID string) error {
	bid := strings.TrimSpace(botID)
	sid := strings.TrimSpace(sessionID)
	sb := strings.TrimSpace(sessionBotID)
	if bid == "" || sb == "" || bid == sb {
		return nil
	}
	return fmt.Errorf("session %s belongs to bot %s, not %s", sid, sb, bid)
}

func (r *Resolver) requireACPRuntimeOwnerWorkspaceExec(ctx context.Context, botID, runtimeOwnerAccountID string) error {
	if r == nil || r.botPermissions == nil {
		return errors.New("bot permission checker not configured")
	}
	runtimeOwnerAccountID = strings.TrimSpace(runtimeOwnerAccountID)
	if runtimeOwnerAccountID == "" {
		return acpfeedback.New(
			acpfeedback.CodeRuntimeOwnerMissing,
			"missing_runtime_owner",
			409,
			"chat.acp.runtimeOwnerMissing",
			"ACP runtime owner is missing; recreate or reauthorize the ACP session",
			nil,
		)
	}
	ok, err := r.botPermissions.HasBotPermission(ctx, strings.TrimSpace(botID), runtimeOwnerAccountID, bots.PermissionWorkspaceExec)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return acpfeedback.New(
		acpfeedback.CodeNoWorkspaceExec,
		"missing_workspace_exec",
		403,
		"chat.acp.missingWorkspaceExec",
		"ACP runtime owner no longer has workspace execution permission for this bot.",
		nil,
	)
}

func mergeACPRuntimeMetadata(metadata, runtimeMetadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata)+len(runtimeMetadata))
	for key, value := range metadata {
		out[key] = value
	}
	for _, key := range []string{"acp_agent_id", "project_path", "acp_project_mode", "runtime_owner_account_id"} {
		if value, ok := runtimeMetadata[key]; ok {
			out[key] = value
		}
	}
	return out
}

func (r *Resolver) streamACPAgentChunks(ctx context.Context, req conversation.ChatRequest, chunkCh chan<- conversation.StreamChunk, errCh chan<- error) {
	eventCh := make(chan WSStreamEvent)
	done := make(chan error, 1)
	go func() {
		defer close(eventCh)
		done <- r.streamACPAgentWS(ctx, req, eventCh, nil)
		close(done)
	}()
	for eventCh != nil || done != nil {
		select {
		case event, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			select {
			case chunkCh <- event:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		case err, ok := <-done:
			if !ok {
				done = nil
				continue
			}
			if err != nil {
				errCh <- err
			}
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		}
	}
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

func (r *Resolver) persistACPLeadingUserMessage(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, *messagepkg.Message, error) {
	if req.UserMessagePersisted || r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" {
		return req, nil, nil
	}
	displayText := strings.TrimSpace(req.RawQuery)
	if displayText == "" {
		displayText = strings.TrimSpace(req.Query)
	}
	if displayText == "" && len(req.Attachments) == 0 {
		return req, nil, nil
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
		return req, nil, fmt.Errorf("marshal ACP leading user message: %w", err)
	}
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	sessionMode, runtimeType := r.persistSessionRuntimeSnapshot(ctx, req)
	persisted, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
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
		SessionMode:             sessionMode,
		RuntimeType:             runtimeType,
	})
	if err != nil {
		return req, nil, fmt.Errorf("persist ACP leading user message: %w", err)
	}
	req.UserMessagePersisted = true
	req.PersistedUserMessageID = persisted.ID
	return req, &persisted, nil
}

func (r *Resolver) persistACPDecisionProjection(ctx context.Context, req conversation.ChatRequest, ev agentpkg.StreamEvent) (bool, error) {
	if r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.SessionID) == "" {
		return false, nil
	}
	output := sdkMessagesToModelMessages(acpclient.TranscriptFromEvents([]event.StreamEvent{ev}, ""))
	sessionMode, runtimeType := r.persistSessionRuntimeSnapshot(ctx, req)
	for _, msg := range output {
		if msg.Role != "assistant" {
			continue
		}
		content, err := json.Marshal(msg)
		if err != nil {
			return false, fmt.Errorf("marshal ACP decision projection for %s: %w", ev.ToolCallID, err)
		}
		if _, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:                   req.BotID,
			SessionID:               req.SessionID,
			SenderChannelIdentityID: "",
			Role:                    "assistant",
			Content:                 content,
			Metadata:                buildRouteMetadata(req),
			SessionMode:             sessionMode,
			RuntimeType:             runtimeType,
		}); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
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
func (r *Resolver) cancelPendingACPApprovals(ctx context.Context, req conversation.ChatRequest, reason string) error {
	if r == nil || r.toolApproval == nil {
		return nil
	}
	cancelled, err := r.toolApproval.CancelPendingForSession(ctx, req.BotID, req.SessionID, reason)
	if err != nil {
		return fmt.Errorf("cancel pending ACP approvals: %w", err)
	}
	if len(cancelled) > 0 {
		r.logger.Info("cancelled pending ACP approvals with their turn",
			slog.String("session_id", req.SessionID),
			slog.Int("count", len(cancelled)))
	}
	return nil
}

func (r *Resolver) persistACPRound(ctx context.Context, req conversation.ChatRequest, agentID, projectPath string, result acpclient.PromptResult, promptErr error) error {
	meta := map[string]any{
		"acp_agent_id": agentID,
		"project_path": projectPath,
		"stop_reason":  result.StopReason,
	}
	if promptErr != nil {
		meta["error"] = acpUserFacingFailureMessage(promptErr)
		var feedbackErr *acpfeedback.Error
		if errors.As(promptErr, &feedbackErr) {
			meta["error_code"] = feedbackErr.Code
			meta["error_reason"] = feedbackErr.Reason
			meta["i18n_key"] = feedbackErr.I18nKey
		} else {
			meta["error_code"] = "acp_runtime_prompt_failed"
		}
	}
	// result.Output is already assembled by the ACP client; the resolver only
	// converts and stores it.
	output := sdkMessagesToModelMessages(result.Output)
	if len(output) == 0 {
		output = []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("")}}
	}
	if result.Usage != nil {
		for idx := len(output) - 1; idx >= 0; idx-- {
			if output[idx].Role == "assistant" {
				usage, _ := json.Marshal(result.Usage)
				output[idx].Usage = usage
				break
			}
		}
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
	skipMemory := promptErr != nil || req.UserMessagePersisted || req.SkipMemoryExtraction
	err := r.storeRoundWithOptions(ctx, req, round, "", storeRoundOptions{
		SkipMemory:              skipMemory,
		AllowEmptyAssistantText: true,
		MessageMetadataByIndex:  metadataByIndex,
	})
	if err == nil && promptErr == nil && req.UserMessagePersisted && !req.SkipMemoryExtraction {
		go r.storeMemory(context.WithoutCancel(ctx), req, round)
	}
	return err
}

// acpFailureResult appends a short, sanitized failure marker to the partial
// result. Detailed upstream errors can include local paths or auth file names,
// so they stay in logs instead of user-visible chat history.
func acpFailureResult(result acpclient.PromptResult, err error) (acpclient.PromptResult, string) {
	message := acpUserFacingFailureMessage(err)
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

func acpUserFacingFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	var feedback *acpfeedback.Error
	if errors.As(err, &feedback) {
		return strings.TrimSpace(feedback.Message)
	}
	return "ACP agent failed to complete the turn. Please retry."
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
