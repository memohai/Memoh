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
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpfeedback"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/agent/sessionmode"
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

type acpPrompter interface {
	Prompt(ctx context.Context, input acpagent.PromptInput) (acpclient.PromptResult, error)
}

const acpDecisionProjectionTimeout = 5 * time.Second

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
	req, err = r.persistACPLeadingUserMessage(context.WithoutCancel(ctx), req)
	if err != nil {
		return err
	}
	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.RawQuery)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	canDeliverDecisions := eventCh != nil &&
		sessionmode.IsInteractive(sess.Type) &&
		strings.TrimSpace(req.StreamID) != "" &&
		req.TurnReplacement == nil &&
		len(req.DiscussDeliveryClaims) == 0
	canRequestUserInput := canDeliverDecisions && r.userInput != nil
	promptStreamID := strings.TrimSpace(req.StreamID)
	if !canDeliverDecisions {
		promptStreamID = ""
	}
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
		projectedMu               sync.Mutex
		durableProjectionStatuses = acpTranscriptCoveredStatuses{}
		transcriptCoveredStatuses = acpTranscriptCoveredStatuses{}
		suppressedPending         = map[string]struct{}{}
		observedTerminal          = map[string]struct{}{}
		emitErrMu                 sync.Mutex
		emitErr                   error
	)
	setEmitError := func(err error) {
		if err == nil {
			return
		}
		emitErrMu.Lock()
		first := emitErr == nil
		if first {
			emitErr = err
		}
		emitErrMu.Unlock()
		if first {
			cancel()
		}
	}
	getEmitError := func() error {
		emitErrMu.Lock()
		defer emitErrMu.Unlock()
		return emitErr
	}
	recordProjectionStatus := func(ev agentpkg.StreamEvent) bool {
		toolCallID := strings.TrimSpace(ev.ToolCallID)
		if toolCallID == "" {
			return false
		}
		status := acpDecisionProjectionStatus(ev)
		projectedMu.Lock()
		defer projectedMu.Unlock()
		if status == "pending" {
			transcriptCoveredStatuses.add(toolCallID, status)
			if _, terminal := observedTerminal[toolCallID]; terminal {
				return false
			}
			if _, suppressed := suppressedPending[toolCallID]; suppressed {
				return false
			}
		} else {
			observedTerminal[toolCallID] = struct{}{}
		}
		if durableProjectionStatuses.has(toolCallID, status) {
			return true
		}
		projectionCtx, projectionCancel := context.WithTimeout(context.WithoutCancel(ctx), acpDecisionProjectionTimeout)
		persisted := r.persistACPDecisionProjection(projectionCtx, req, ev)
		projectionCancel()
		if persisted {
			transcriptCoveredStatuses.add(toolCallID, status)
		}
		if !persisted {
			if status == "pending" {
				suppressedPending[toolCallID] = struct{}{}
			}
			return false
		}
		durableProjectionStatuses.add(toolCallID, status)
		return true
	}
	projectedSnapshot := func() acpTranscriptCoveredStatuses {
		projectedMu.Lock()
		defer projectedMu.Unlock()
		return transcriptCoveredStatuses.clone()
	}

	emit := func(ev agentpkg.StreamEvent) bool {
		select {
		case <-streamCtx.Done():
			return false
		default:
		}
		if getEmitError() != nil {
			return false
		}
		projectionAcknowledged := true
		if isACPDecisionProjectionEvent(ev) {
			if strings.TrimSpace(ev.ToolCallID) == "" {
				setEmitError(fmt.Errorf("ACP prompt emitted decision event %s without a tool call id", ev.Type))
				return false
			}
			canDeliver := canDeliverDecisions
			if ev.Type == agentpkg.EventUserInputRequest {
				canDeliver = canRequestUserInput
			}
			if !canDeliver {
				setEmitError(fmt.Errorf("ACP prompt cannot accept decision event %s", ev.Type))
				return false
			}
			projectionAcknowledged = recordProjectionStatus(ev)
			if !projectionAcknowledged && acpDecisionProjectionStatus(ev) == "pending" {
				return false
			}
		}
		data, err := json.Marshal(ev)
		if err != nil {
			return false
		}
		if activePrompt != nil {
			activePrompt.emit(ev)
		}
		select {
		case eventCh <- json.RawMessage(data):
			return projectionAcknowledged
		case <-streamCtx.Done():
			return false
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
		StreamID:                 promptStreamID,
		SessionType:              sess.Type,
		RouteID:                  req.RouteID,
		AgentID:                  agentID,
		ProjectPath:              projectPath,
		Prompt:                   req.Query,
		Images:                   preparedAttachments.Images,
		AttachmentReferences:     preparedAttachments.References,
		CanFallbackImagesToFiles: preparedAttachments.CanFallbackImagesToFiles,
		ChannelIdentityID:        req.SourceChannelIdentityID,
		SessionToken:             req.Token,
		CurrentPlatform:          req.CurrentChannel,
		ReplyTarget:              req.ReplyTarget,
		ConversationType:         req.ConversationType,
		CanRequestUserInput:      canRequestUserInput,
		// This flag controls image bytes returned later by the read-media MCP
		// tool. Initial user images use ACP ImageBlock transport above.
		SupportsImageInput:    false,
		ToolOutputLimit:       r.toolOutputLimit(),
		ToolHTTPURL:           req.ToolHTTPURL,
		ContextURI:            acpContextURI,
		ContextMarkdown:       contextMarkdown,
		RuntimeOwnerAccountID: runtimeOwnerAccountID,
		ForceFreshRuntime:     req.ForceFreshRuntime,
		Sink:                  acpclient.EventSinkFunc(emit),
	})
	if emitErr := getEmitError(); emitErr != nil {
		r.cancelPendingACPApprovals(context.WithoutCancel(ctx), req, "tool approval cancelled: the prompt cannot accept decisions")
		return emitErr
	}
	if err != nil {
		r.logger.Error("ACP prompt failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		r.cancelPendingACPApprovals(context.WithoutCancel(ctx), req, "tool approval cancelled: the turn ended before a decision arrived")
		var feedbackErr *acpfeedback.Error
		if errors.As(err, &feedbackErr) {
			return err
		}
		if feedbackErr := acpPromptInputFeedback(err); feedbackErr != nil {
			return feedbackErr
		}
		result = ensureACPPromptOutput(result)
		failedResult, failureDelta := acpFailureResult(result, err)
		projected := projectedSnapshot()
		failedResult.Output = filterACPProjectedOutput(failedResult.Output, projected)
		if failureDelta != "" {
			emit(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: failureDelta})
		}
		cursorCommitted, persistErr := r.persistACPRoundResult(acpRoundPersistContext(ctx, req), req, agentID, projectPath, failedResult, err)
		if persistErr != nil {
			r.logger.Error("ACP failed round persist failed", slog.Any("error", persistErr), slog.String("session_id", req.SessionID))
		}
		emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
		emit(acpTerminalStreamEventWithCursor(agentpkg.EventAbort, failedResult, req, cursorCommitted))
		if persistErr != nil {
			return fmt.Errorf("persist failed ACP round: %w", persistErr)
		}
		return nil
	}

	emit(agentpkg.StreamEvent{Type: agentpkg.EventTextEnd})
	projected := projectedSnapshot()
	result = ensureACPPromptOutput(result)
	result.Output = filterACPProjectedOutput(result.Output, projected)
	cursorCommitted, persistErr := r.persistACPRoundResult(acpRoundPersistContext(ctx, req), req, agentID, projectPath, result, nil)
	if persistErr != nil {
		r.logger.Error("ACP persist failed", slog.Any("error", persistErr), slog.String("session_id", req.SessionID))
	}
	emit(acpTerminalStreamEventWithCursor(agentpkg.EventEnd, result, req, cursorCommitted))
	if persistErr != nil {
		return fmt.Errorf("persist ACP round: %w", persistErr)
	}
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

func ensureACPPromptOutput(result acpclient.PromptResult) acpclient.PromptResult {
	if len(result.Output) == 0 {
		result.Output = acpclient.TranscriptFromEvents(result.Events, result.Text)
	}
	return result
}

func acpRoundPersistContext(ctx context.Context, req conversation.ChatRequest) context.Context {
	if req.DiscussConsumedEventCursor > 0 {
		return ctx
	}
	return context.WithoutCancel(ctx)
}

func acpTerminalStreamEvent(eventType agentpkg.StreamEventType, result acpclient.PromptResult) agentpkg.StreamEvent {
	result = ensureACPPromptOutput(result)
	ev := agentpkg.StreamEvent{Type: eventType}
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

func acpTerminalStreamEventWithCursor(
	eventType agentpkg.StreamEventType,
	result acpclient.PromptResult,
	req conversation.ChatRequest,
	cursorCommitted bool,
) agentpkg.StreamEvent {
	ev := acpTerminalStreamEvent(eventType, result)
	if req.DiscussConsumedEventCursor > 0 {
		ev.Metadata = map[string]any{agentpkg.MetadataKeyDiscussCursorCommitted: cursorCommitted}
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
		return true
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

func (r *Resolver) persistACPLeadingUserMessage(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	if req.TurnReplacement != nil {
		existingRequestMessageID := strings.TrimSpace(req.TurnReplacement.ExistingRequestMessageID)
		if existingRequestMessageID != "" {
			persistedUserMessageID := strings.TrimSpace(req.PersistedUserMessageID)
			if persistedUserMessageID != "" && persistedUserMessageID != existingRequestMessageID {
				return req, errors.New("replacement request id does not match persisted ACP user message")
			}
			req.UserMessagePersisted = true
			req.PersistedUserMessageID = existingRequestMessageID
		}
		return req, nil
	}
	if len(req.DiscussDeliveryClaims) > 0 && !req.UserMessagePersisted {
		return req, errors.New("exact ACP delivery has no durable request message")
	}
	if req.UserMessagePersisted {
		if strings.TrimSpace(req.PersistedUserMessageID) == "" {
			return req, errors.New("persisted ACP user message has no durable request id")
		}
		return req, nil
	}
	if r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" {
		return req, nil
	}
	displayText := strings.TrimSpace(req.RawQuery)
	if displayText == "" {
		displayText = strings.TrimSpace(req.Query)
	}
	if displayText == "" && len(req.Attachments) == 0 {
		return req, nil
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
		return req, fmt.Errorf("marshal ACP leading user message: %w", err)
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
		return req, fmt.Errorf("persist ACP leading user message: %w", err)
	}
	req.UserMessagePersisted = true
	req.PersistedUserMessageID = persisted.ID
	return req, nil
}

func (r *Resolver) persistACPDecisionProjection(ctx context.Context, req conversation.ChatRequest, ev agentpkg.StreamEvent) bool {
	if req.TurnReplacement != nil || len(req.DiscussDeliveryClaims) > 0 {
		return false
	}
	if r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.PersistedUserMessageID) == "" {
		return false
	}
	output := sdkMessagesToModelMessages(acpclient.TranscriptFromEvents([]event.StreamEvent{ev}, ""))
	sessionMode, runtimeType := r.persistSessionRuntimeSnapshot(ctx, req)
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
		input := messagepkg.PersistInput{
			BotID:                   req.BotID,
			SessionID:               req.SessionID,
			SenderChannelIdentityID: "",
			Role:                    "assistant",
			Content:                 content,
			Metadata:                buildRouteMetadata(req),
			SessionMode:             sessionMode,
			RuntimeType:             runtimeType,
			TurnRequestMessageID:    req.PersistedUserMessageID,
		}
		var persistErr error
		if req.EventDeliveryClaim != nil {
			claims := make([]messagepkg.DeliveryClaim, 0, 1)
			persistErr = appendDeliveryClaim(&claims, make(map[string]string), "request", req.EventID, req.EventDeliveryClaim)
			if persistErr == nil {
				batcher, ok := r.messageService.(messagepkg.AtomicRoundPersister)
				if !ok {
					persistErr = errors.New("message service does not support claim-fenced projection persistence")
				} else {
					var persisted []messagepkg.Message
					var handled bool
					persisted, handled, persistErr = batcher.PersistRound(ctx, []messagepkg.PersistInput{input}, messagepkg.RoundPersistenceOptions{
						DeliveryClaims: claims,
					})
					if persistErr == nil && (!handled || len(persisted) != 1) {
						persistErr = errors.New("message service did not persist claim-fenced projection")
					}
				}
			}
		} else {
			_, persistErr = r.messageService.Persist(ctx, input)
		}
		if persistErr != nil {
			r.logger.Warn("persist ACP decision projection failed",
				slog.String("bot_id", req.BotID),
				slog.String("session_id", req.SessionID),
				slog.String("tool_call_id", ev.ToolCallID),
				slog.Any("error", persistErr))
			return false
		}
		return true
	}
	return false
}

type acpTranscriptCoveredStatuses map[string]map[string]struct{}

func (s acpTranscriptCoveredStatuses) add(toolCallID, status string) {
	statuses := s[toolCallID]
	if statuses == nil {
		statuses = map[string]struct{}{}
		s[toolCallID] = statuses
	}
	statuses[status] = struct{}{}
}

func (s acpTranscriptCoveredStatuses) has(toolCallID, status string) bool {
	_, ok := s[toolCallID][status]
	return ok
}

func (s acpTranscriptCoveredStatuses) clone() acpTranscriptCoveredStatuses {
	if len(s) == 0 {
		return nil
	}
	cloned := make(acpTranscriptCoveredStatuses, len(s))
	for toolCallID, statuses := range s {
		cloned[toolCallID] = make(map[string]struct{}, len(statuses))
		for status := range statuses {
			cloned[toolCallID][status] = struct{}{}
		}
	}
	return cloned
}

func filterACPProjectedOutput(messages []sdk.Message, covered acpTranscriptCoveredStatuses) []sdk.Message {
	if len(messages) == 0 || len(covered) == 0 {
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
			toolCallID := strings.TrimSpace(call.ToolCallID)
			if covered.has(toolCallID, acpDecisionToolCallStatus(call)) {
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

func acpDecisionToolCallStatus(call sdk.ToolCallPart) string {
	for _, key := range []string{"approval", "user_input"} {
		metadata, ok := call.ProviderMetadata[key].(map[string]any)
		if !ok {
			continue
		}
		status, _ := metadata["status"].(string)
		if status = strings.ToLower(strings.TrimSpace(status)); status != "" {
			return status
		}
	}
	return ""
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
	_, err := r.persistACPRoundResult(ctx, req, agentID, projectPath, result, promptErr)
	return err
}

func (r *Resolver) persistACPRoundResult(ctx context.Context, req conversation.ChatRequest, agentID, projectPath string, result acpclient.PromptResult, promptErr error) (bool, error) {
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
	persisted, err := r.storeRoundWithOptionsResult(ctx, req, round, "", storeRoundOptions{
		SkipMemory:              skipMemory,
		AllowEmptyAssistantText: true,
		MessageMetadataByIndex:  metadataByIndex,
		DiscussCursor:           discussCursorUpdateFromRequest(req),
	})
	if err == nil && promptErr == nil && req.UserMessagePersisted && !req.SkipMemoryExtraction {
		go r.storeMemory(context.WithoutCancel(ctx), req, round)
	}
	return err == nil && len(persisted) > 0, err
}

func discussCursorUpdateFromRequest(req conversation.ChatRequest) *messagepkg.DiscussCursorUpdate {
	if req.DiscussConsumedEventCursor <= 0 {
		return nil
	}
	claims := make([]messagepkg.DeliveryClaim, len(req.DiscussDeliveryClaims))
	for i, claim := range req.DiscussDeliveryClaims {
		claims[i] = messagepkg.DeliveryClaim{
			EventID:    claim.EventID,
			ClaimToken: claim.ClaimToken,
		}
	}
	return &messagepkg.DiscussCursorUpdate{
		SessionID:           req.SessionID,
		ScopeKey:            req.DiscussCursorScope,
		RouteID:             req.RouteID,
		Source:              req.CurrentChannel,
		ConsumedCursor:      req.DiscussConsumedCursor,
		ConsumedEventCursor: req.DiscussConsumedEventCursor,
		DeliveryClaims:      claims,
	}
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
