package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type RetryLatestMessageInput struct {
	BotID                  string
	SessionID              string
	StreamID               string
	MessageID              string
	ActorChannelIdentityID string
	ActorUserID            string
	ChatToken              string
	Model                  string
	ReasoningEffort        string
	WorkspaceTargetID      string
	ToolHTTPURL            string
}

type EditLatestMessageInput struct {
	BotID                  string
	SessionID              string
	StreamID               string
	MessageID              string
	Text                   string
	Attachments            []conversation.ChatAttachment
	ActorChannelIdentityID string
	ActorUserID            string
	ChatToken              string
	Model                  string
	ReasoningEffort        string
	WorkspaceTargetID      string
	ToolHTTPURL            string
}

type ReplacementOperation struct {
	Kind                 string
	ReplaceFromMessageID string
	ReplacementUserTurn  *conversation.UITurn
}

type PreparedReplacementWS struct {
	Request          conversation.ChatRequest
	OldTurnID        string
	RequestMessageID string
	Reason           string
	Operation        ReplacementOperation
	sessionTurnHeld  bool
}

type replacementPersistenceContextKey struct{}

type replacementPersistenceState struct {
	oldTurnID                  string
	requestMessageID           string
	reason                     string
	replacedTailStartMessageID string
	forkAnchor                 *forkAnchorUpdate
	forkAnchorPrepared         bool
	atomicCommitted            bool
}

func replacementPersistenceFromContext(ctx context.Context) *replacementPersistenceState {
	state, _ := ctx.Value(replacementPersistenceContextKey{}).(*replacementPersistenceState)
	return state
}

// AdmitPreparedReplacementWS reserves the local session turn without blocking
// the WebSocket command loop. Cross-server serialization is completed by the
// runtime admission that follows.
func (r *Resolver) AdmitPreparedReplacementWS(ctx context.Context, prepared PreparedReplacementWS) (PreparedReplacementWS, func(), error) {
	if r == nil {
		return PreparedReplacementWS{}, func() {}, errors.New("resolver is not configured")
	}
	if strings.TrimSpace(prepared.OldTurnID) == "" || strings.TrimSpace(prepared.Request.SessionID) == "" {
		return PreparedReplacementWS{}, func() {}, errors.New("prepared replacement is invalid")
	}
	release, admitted := r.tryEnterIdleSessionTurn(ctx, prepared.Request.BotID, prepared.Request.SessionID)
	if !admitted {
		return PreparedReplacementWS{}, func() {}, errors.New("session already has an active turn")
	}
	if err := r.ensureLatestVisibleTurn(ctx, prepared.Request.SessionID, prepared.OldTurnID); err != nil {
		release()
		return PreparedReplacementWS{}, func() {}, errors.New("replacement target is no longer the latest turn")
	}
	prepared.sessionTurnHeld = true
	return prepared, release, nil
}

// ValidatePreparedReplacementWS performs the final database check after the
// cross-server runtime reservation has succeeded and before the operation is
// published.
func (r *Resolver) ValidatePreparedReplacementWS(ctx context.Context, prepared PreparedReplacementWS) error {
	if r == nil || strings.TrimSpace(prepared.OldTurnID) == "" || strings.TrimSpace(prepared.Request.SessionID) == "" {
		return errors.New("prepared replacement is invalid")
	}
	if err := r.ensureLatestVisibleTurn(ctx, prepared.Request.SessionID, prepared.OldTurnID); err != nil {
		return errors.New("replacement target is no longer the latest turn")
	}
	return nil
}

// WithAttachments replaces an admitted edit's transport attachments with their
// persisted media references before the operation is published.
func (prepared PreparedReplacementWS) WithAttachments(attachments []conversation.ChatAttachment) (PreparedReplacementWS, error) {
	if !strings.EqualFold(strings.TrimSpace(prepared.Reason), "edit") {
		return PreparedReplacementWS{}, errors.New("only prepared edits accept replacement attachments")
	}
	prepared.Request.Attachments = append([]conversation.ChatAttachment(nil), attachments...)
	turn := prepared.Operation.ReplacementUserTurn
	if turn == nil {
		return PreparedReplacementWS{}, errors.New("prepared edit has no replacement user turn")
	}
	turnCopy := *turn
	turnCopy.Attachments = uiAttachmentsFromChatAttachments(attachments)
	prepared.Operation.ReplacementUserTurn = &turnCopy
	return prepared, nil
}

func (r *Resolver) RetryLatestMessageWS(ctx context.Context, input RetryLatestMessageInput, eventCh chan<- WSStreamEvent, abortCh <-chan struct{}) error {
	prepared, err := r.PrepareRetryLatestMessageWS(ctx, input)
	if err != nil {
		return err
	}
	return r.StreamPreparedReplacementWS(ctx, prepared, eventCh, abortCh)
}

func (r *Resolver) PrepareRetryLatestMessageWS(ctx context.Context, input RetryLatestMessageInput) (PreparedReplacementWS, error) {
	if r == nil || r.messageService == nil {
		return PreparedReplacementWS{}, errors.New("message service not configured")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	messageID := strings.TrimSpace(input.MessageID)
	if sessionID == "" {
		return PreparedReplacementWS{}, errors.New("session id is required")
	}
	if messageID == "" {
		return PreparedReplacementWS{}, errors.New("message id is required")
	}

	turn, target, err := r.latestVisibleTurnAndMessage(ctx, sessionID, messageID)
	if err != nil {
		return PreparedReplacementWS{}, err
	}
	if !strings.EqualFold(target.Role, "assistant") {
		return PreparedReplacementWS{}, errors.New("only latest assistant messages can be retried")
	}
	if err := r.ensureLatestVisibleTurn(ctx, sessionID, turn.ID); err != nil {
		return PreparedReplacementWS{}, errors.New("only the latest assistant message can be retried")
	}
	requestMessageID := strings.TrimSpace(turn.RequestMessageID)
	if requestMessageID == "" {
		return PreparedReplacementWS{}, errors.New("retry target has no request message")
	}
	requestMessage, err := r.messageService.GetByIDBySession(ctx, sessionID, requestMessageID)
	if err != nil {
		return PreparedReplacementWS{}, err
	}
	if !strings.EqualFold(requestMessage.Role, "user") {
		return PreparedReplacementWS{}, errors.New("retry target request is not a user message")
	}
	cutoffMessageID := strings.TrimSpace(turn.AssistantMessageID)
	if cutoffMessageID == "" {
		cutoffMessageID = target.ID
	}

	req := conversation.ChatRequest{
		BotID:                        strings.TrimSpace(input.BotID),
		ChatID:                       strings.TrimSpace(input.BotID),
		SessionID:                    sessionID,
		StreamID:                     strings.TrimSpace(input.StreamID),
		UserID:                       strings.TrimSpace(input.ActorUserID),
		SourceChannelIdentityID:      strings.TrimSpace(input.ActorChannelIdentityID),
		ConversationType:             channel.ConversationTypePrivate,
		Query:                        visibleMessageText(requestMessage),
		RawQuery:                     visibleMessageText(requestMessage),
		Token:                        strings.TrimSpace(input.ChatToken),
		ChatToken:                    strings.TrimSpace(input.ChatToken),
		CurrentChannel:               "local",
		ReplyTarget:                  strings.TrimSpace(input.BotID),
		Channels:                     []string{"local"},
		Model:                        strings.TrimSpace(input.Model),
		ReasoningEffort:              strings.TrimSpace(input.ReasoningEffort),
		WorkspaceTargetID:            strings.TrimSpace(input.WorkspaceTargetID),
		ToolHTTPURL:                  strings.TrimSpace(input.ToolHTTPURL),
		ReusePersistedUserMessage:    true,
		PersistedUserMessageID:       requestMessage.ID,
		SkipHistoryTurn:              true,
		HistoryCutoffBeforeMessageID: cutoffMessageID,
		RequiredHistoryMessageID:     requestMessage.ID,
	}
	return PreparedReplacementWS{
		Request:          req,
		OldTurnID:        turn.ID,
		RequestMessageID: requestMessage.ID,
		Reason:           "retry",
		Operation: ReplacementOperation{
			Kind:                 "retry",
			ReplaceFromMessageID: cutoffMessageID,
		},
	}, nil
}

func (r *Resolver) EditLatestMessageWS(ctx context.Context, input EditLatestMessageInput, eventCh chan<- WSStreamEvent, abortCh <-chan struct{}) error {
	prepared, err := r.PrepareEditLatestMessageWS(ctx, input)
	if err != nil {
		return err
	}
	return r.StreamPreparedReplacementWS(ctx, prepared, eventCh, abortCh)
}

func (r *Resolver) PrepareEditLatestMessageWS(ctx context.Context, input EditLatestMessageInput) (PreparedReplacementWS, error) {
	if r == nil || r.messageService == nil {
		return PreparedReplacementWS{}, errors.New("message service not configured")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	messageID := strings.TrimSpace(input.MessageID)
	text := strings.TrimSpace(input.Text)
	if sessionID == "" {
		return PreparedReplacementWS{}, errors.New("session id is required")
	}
	if messageID == "" {
		return PreparedReplacementWS{}, errors.New("message id is required")
	}
	if text == "" && len(input.Attachments) == 0 {
		return PreparedReplacementWS{}, errors.New("message text or attachments required")
	}

	turn, target, err := r.latestVisibleTurnAndMessage(ctx, sessionID, messageID)
	if err != nil {
		return PreparedReplacementWS{}, err
	}
	if !strings.EqualFold(target.Role, "user") {
		return PreparedReplacementWS{}, errors.New("only latest user messages can be edited")
	}
	if strings.TrimSpace(turn.RequestMessageID) != target.ID {
		return PreparedReplacementWS{}, errors.New("edit target is not the request for its turn")
	}
	if err := r.ensureLatestVisibleTurn(ctx, sessionID, turn.ID); err != nil {
		return PreparedReplacementWS{}, errors.New("only the latest user message can be edited")
	}

	req := conversation.ChatRequest{
		BotID:                        strings.TrimSpace(input.BotID),
		ChatID:                       strings.TrimSpace(input.BotID),
		SessionID:                    sessionID,
		StreamID:                     strings.TrimSpace(input.StreamID),
		UserID:                       strings.TrimSpace(input.ActorUserID),
		SourceChannelIdentityID:      strings.TrimSpace(input.ActorChannelIdentityID),
		ConversationType:             channel.ConversationTypePrivate,
		Query:                        text,
		RawQuery:                     text,
		Token:                        strings.TrimSpace(input.ChatToken),
		ChatToken:                    strings.TrimSpace(input.ChatToken),
		CurrentChannel:               "local",
		ReplyTarget:                  strings.TrimSpace(input.BotID),
		Channels:                     []string{"local"},
		Attachments:                  input.Attachments,
		Model:                        strings.TrimSpace(input.Model),
		ReasoningEffort:              strings.TrimSpace(input.ReasoningEffort),
		WorkspaceTargetID:            strings.TrimSpace(input.WorkspaceTargetID),
		ToolHTTPURL:                  strings.TrimSpace(input.ToolHTTPURL),
		SkipHistoryTurn:              true,
		HistoryCutoffBeforeMessageID: target.ID,
	}
	replacementUserTurn := &conversation.UITurn{
		Role:        "user",
		Text:        text,
		Attachments: uiAttachmentsFromChatAttachments(input.Attachments),
		Timestamp:   time.Now().UTC(),
		Platform:    "local",
	}
	return PreparedReplacementWS{
		Request:   req,
		OldTurnID: turn.ID,
		Reason:    "edit",
		Operation: ReplacementOperation{
			Kind:                 "edit",
			ReplaceFromMessageID: target.ID,
			ReplacementUserTurn:  replacementUserTurn,
		},
	}, nil
}

func (r *Resolver) StreamPreparedReplacementWS(ctx context.Context, prepared PreparedReplacementWS, eventCh chan<- WSStreamEvent, abortCh <-chan struct{}) error {
	if strings.TrimSpace(prepared.OldTurnID) == "" || strings.TrimSpace(prepared.Reason) == "" {
		return errors.New("prepared replacement is invalid")
	}
	return r.streamReplacementWS(ctx, prepared.Request, prepared.OldTurnID, prepared.RequestMessageID, prepared.Reason, eventCh, abortCh, prepared.sessionTurnHeld)
}

func uiAttachmentsFromChatAttachments(attachments []conversation.ChatAttachment) []conversation.UIAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]conversation.UIAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, conversation.UIAttachment{
			Type:        strings.TrimSpace(attachment.Type),
			Path:        strings.TrimSpace(attachment.Path),
			URL:         strings.TrimSpace(attachment.URL),
			Base64:      strings.TrimSpace(attachment.Base64),
			Name:        strings.TrimSpace(attachment.Name),
			ContentHash: strings.TrimSpace(attachment.ContentHash),
			Mime:        strings.TrimSpace(attachment.Mime),
			Size:        attachment.Size,
			Metadata:    attachment.Metadata,
		})
	}
	return out
}

func (r *Resolver) latestVisibleTurnAndMessage(ctx context.Context, sessionID, messageID string) (messagepkg.HistoryTurn, messagepkg.Message, error) {
	target, err := r.messageService.GetByIDBySession(ctx, sessionID, messageID)
	if err != nil {
		return messagepkg.HistoryTurn{}, messagepkg.Message{}, fmt.Errorf("load message: %w", err)
	}
	turn, err := r.messageService.GetVisibleTurnByMessage(ctx, sessionID, messageID)
	if err != nil {
		return messagepkg.HistoryTurn{}, messagepkg.Message{}, fmt.Errorf("load visible turn: %w", err)
	}
	return turn, target, nil
}

func (r *Resolver) ensureLatestVisibleTurn(ctx context.Context, sessionID, turnID string) error {
	latest, err := r.messageService.GetLatestVisibleTurnBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load latest visible turn: %w", err)
	}
	if strings.TrimSpace(latest.ID) != strings.TrimSpace(turnID) {
		return errors.New("turn is not latest")
	}
	return nil
}

func (r *Resolver) streamReplacementWS(
	ctx context.Context,
	req conversation.ChatRequest,
	oldTurnID string,
	requestMessageID string,
	reason string,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
	sessionTurnHeld bool,
) error {
	persistence := &replacementPersistenceState{
		oldTurnID:                  oldTurnID,
		requestMessageID:           requestMessageID,
		reason:                     reason,
		replacedTailStartMessageID: req.HistoryCutoffBeforeMessageID,
	}
	ctx = context.WithValue(ctx, replacementPersistenceContextKey{}, persistence)
	var preflight func(context.Context) error
	if !sessionTurnHeld {
		preflight = func(ctx context.Context) error {
			return r.ensureLatestVisibleTurn(ctx, req.SessionID, oldTurnID)
		}
	}
	run := r.streamChatWSResultWithHooksAndTurn
	if r.streamReplacementFn != nil {
		run = r.streamReplacementFn
	}
	persisted, err := run(
		ctx,
		req,
		eventCh,
		abortCh,
		preflight,
		sessionTurnHeld,
	)
	if err != nil {
		return err
	}
	if len(persisted) > 0 && !persistence.atomicCommitted {
		return errors.New("replacement was not atomically persisted")
	}
	return nil
}

type forkAnchorUpdate struct {
	metadata map[string]any
}

func (r *Resolver) prepareForkAnchorUpdate(ctx context.Context, sessionID string, replacedTailStartMessageID string) (*forkAnchorUpdate, error) {
	sessionID = strings.TrimSpace(sessionID)
	replacedTailStartMessageID = strings.TrimSpace(replacedTailStartMessageID)
	if r == nil || r.sessionService == nil || r.messageService == nil || sessionID == "" || replacedTailStartMessageID == "" {
		return nil, nil
	}

	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for fork anchor update: %w", err)
	}
	fork, ok := forkedFromMetadata(sess.Metadata)
	if !ok {
		return nil, nil
	}
	currentAnchor := forkMetadataString(fork, "fork_message_id")
	if currentAnchor == "" {
		return nil, nil
	}

	replacedTail, err := r.messageService.ListVisibleFromBySession(ctx, sessionID, replacedTailStartMessageID)
	if err != nil {
		return nil, fmt.Errorf("load replaced tail for fork anchor update: %w", err)
	}
	if !messagesContainAnchor(replacedTail, currentAnchor) {
		return nil, nil
	}

	nextAnchor, err := r.latestInheritedAssistantBefore(ctx, sessionID, replacedTailStartMessageID, sess.CreatedAt)
	if err != nil {
		return nil, err
	}
	if nextAnchor == currentAnchor {
		return nil, nil
	}

	nextFork := cloneMetadataMap(fork)
	if nextAnchor != "" {
		nextFork["fork_message_id"] = nextAnchor
	} else {
		delete(nextFork, "fork_message_id")
	}

	nextMetadata := cloneMetadataMap(sess.Metadata)
	nextMetadata["forked_from"] = nextFork
	return &forkAnchorUpdate{metadata: nextMetadata}, nil
}

func (r *Resolver) latestInheritedAssistantBefore(ctx context.Context, sessionID string, beforeMessageID string, sessionCreatedAt time.Time) (string, error) {
	messages, err := r.messageService.ListBeforeMessageBySession(ctx, sessionID, beforeMessageID, 10000)
	if err != nil {
		return "", fmt.Errorf("load previous messages for fork anchor update: %w", err)
	}
	hasCutoff := !sessionCreatedAt.IsZero()
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			continue
		}
		if hasCutoff && (msg.CreatedAt.IsZero() || msg.CreatedAt.After(sessionCreatedAt)) {
			continue
		}
		return strings.TrimSpace(msg.ID), nil
	}
	return "", nil
}

func messagesContainAnchor(messages []messagepkg.Message, anchor string) bool {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return false
	}
	for _, msg := range messages {
		if strings.TrimSpace(msg.ID) == anchor || strings.TrimSpace(msg.ExternalMessageID) == anchor {
			return true
		}
	}
	return false
}

func forkedFromMetadata(metadata map[string]any) (map[string]any, bool) {
	raw, ok := metadata["forked_from"]
	if !ok || raw == nil {
		return nil, false
	}
	switch value := raw.(type) {
	case map[string]any:
		return value, true
	case map[string]string:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func forkMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func cloneMetadataMap(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func visibleMessageText(msg messagepkg.Message) string {
	if text := strings.TrimSpace(msg.DisplayContent); text != "" {
		return text
	}
	var modelMessage conversation.ModelMessage
	if err := json.Unmarshal(msg.Content, &modelMessage); err == nil {
		if text := strings.TrimSpace(modelMessage.TextContent()); text != "" {
			return text
		}
	}
	return strings.TrimSpace(string(msg.Content))
}
