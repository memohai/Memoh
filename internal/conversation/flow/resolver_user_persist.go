package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// ApplyUserMessageHookAndPersistUserTurn applies the normal user-message hook
// before writing a user turn ahead of agent execution. Web skill activations
// use this so a denied hook cannot leave a persisted user-only special turn.
func (r *Resolver) ApplyUserMessageHookAndPersistUserTurn(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, messagepkg.Message, error) {
	nextReq, err := r.PrepareUserMessageWS(ctx, req)
	if err != nil {
		return req, messagepkg.Message{}, err
	}
	persisted, err := r.persistUserTurn(ctx, nextReq)
	if err != nil {
		return nextReq, messagepkg.Message{}, err
	}
	nextReq.UserMessagePersisted = true
	nextReq.PersistedUserMessageID = persisted.ID
	return nextReq, persisted, nil
}

// PrepareUserMessageWS applies the user-message hook before runtime admission
// without writing history. The resulting request can be used to publish the
// canonical runtime request turn and must not run the hook a second time.
func (r *Resolver) PrepareUserMessageWS(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	nextReq, err := r.applyUserMessageHook(ctx, req)
	if err != nil {
		return req, err
	}
	nextReq.UserMessageHookApplied = true
	return nextReq, nil
}

// RuntimeRequestUserTurn returns the canonical, non-durable user turn shown
// while an ordinary WebSocket run is active.
func RuntimeRequestUserTurn(req conversation.ChatRequest, timestamp time.Time) *conversation.UITurn {
	turn := &conversation.UITurn{
		Role:              "user",
		Text:              userTurnDisplayText(req),
		UserMessageKind:   strings.TrimSpace(req.UserMessageKind),
		SkillActivation:   req.SkillActivation,
		Attachments:       uiAttachmentsFromChatAttachments(req.Attachments),
		Timestamp:         timestamp,
		Platform:          strings.TrimSpace(req.CurrentChannel),
		SenderUserID:      strings.TrimSpace(req.UserID),
		ExternalMessageID: strings.TrimSpace(req.ExternalMessageID),
	}
	if req.RuntimeTurn != nil {
		turn.ID = strings.TrimSpace(req.RuntimeTurn.Request.MessageID)
		turn.TurnPosition = req.RuntimeTurn.Request.TurnPosition
		turn.TurnMessageSeq = req.RuntimeTurn.Request.TurnMessageSeq
	}
	return turn
}

// ReserveRuntimeTurn assigns the request row and turn coordinates after
// runtime ownership has been durably fenced but before admission is published.
func (r *Resolver) ReserveRuntimeTurn(ctx context.Context, req conversation.ChatRequest, requestMessageID string) (conversation.ChatRequest, messagepkg.RuntimeTurnReservation, error) {
	reserver, ok := r.messageService.(messagepkg.RuntimeTurnReserver)
	if !ok {
		return req, messagepkg.RuntimeTurnReservation{}, errors.New("message service does not support runtime turn reservations")
	}
	reservation, err := reserver.ReserveRuntimeTurn(ctx, req.BotID, req.SessionID, requestMessageID)
	if err != nil {
		return req, messagepkg.RuntimeTurnReservation{}, err
	}
	req.RuntimeTurn = &reservation
	return req, reservation, nil
}

// persistUserTurn writes a user turn before agent execution. It is used after
// the user-message hook has already accepted the request.
func (r *Resolver) persistUserTurn(ctx context.Context, req conversation.ChatRequest) (messagepkg.Message, error) {
	if r == nil || r.messageService == nil {
		return messagepkg.Message{}, errors.New("message service not configured")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return messagepkg.Message{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return messagepkg.Message{}, errors.New("session id is required")
	}
	modelMessage := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(persistedUserTurnText(req)),
	}
	modelMessage = normalizeUserMessageContent(modelMessage)
	content, err := json.Marshal(modelMessage)
	if err != nil {
		return messagepkg.Message{}, err
	}
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	sessionMode, runtimeType := r.persistSessionRuntimeSnapshot(ctx, req)
	return r.messageService.Persist(ctx, messagepkg.PersistInput{
		BotID:                   req.BotID,
		SessionID:               req.SessionID,
		SenderChannelIdentityID: senderChannelIdentityID,
		SenderUserID:            senderUserID,
		ExternalMessageID:       req.ExternalMessageID,
		SourceReplyToMessageID:  req.SourceReplyToMessageID,
		Role:                    "user",
		Content:                 content,
		Metadata:                mergeMetadata(mergeMetadata(buildRouteMetadata(req), buildInteractionMetadata(req)), workspaceTargetMetadata(req.WorkspaceTarget)),
		Assets:                  chatAttachmentsToAssetRefs(req.Attachments),
		EventID:                 req.EventID,
		DisplayText:             userTurnDisplayText(req),
		SessionMode:             sessionMode,
		RuntimeType:             runtimeType,
	})
}

func userTurnDisplayText(req conversation.ChatRequest) string {
	if strings.TrimSpace(req.UserVisibleText) != "" || req.UserMessageKind == conversation.UserMessageKindSkillActivation {
		return strings.TrimSpace(req.UserVisibleText)
	}
	if strings.TrimSpace(req.RawQuery) != "" {
		return strings.TrimSpace(req.RawQuery)
	}
	return strings.TrimSpace(req.Query)
}

func persistedUserTurnText(req conversation.ChatRequest) string {
	if req.UserMessageKind == conversation.UserMessageKindSkillActivation {
		return strings.TrimSpace(req.UserVisibleText)
	}
	return req.Query
}
