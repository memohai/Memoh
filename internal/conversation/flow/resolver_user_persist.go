package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// ApplyUserMessageHookAndPersistUserTurn applies the normal user-message hook
// before writing a user turn ahead of agent execution. Web skill activations
// use this so a denied hook cannot leave a persisted user-only special turn.
func (r *Resolver) ApplyUserMessageHookAndPersistUserTurn(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, messagepkg.Message, error) {
	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	nextReq, err := r.applyUserMessageHook(ctx, req)
	if err != nil {
		return req, messagepkg.Message{}, err
	}
	persisted, err := r.persistUserTurn(ctx, nextReq)
	if err != nil {
		return nextReq, messagepkg.Message{}, err
	}
	return nextReq, persisted, nil
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
	displayText := strings.TrimSpace(req.Query)
	if strings.TrimSpace(req.UserVisibleText) != "" || req.UserMessageKind == conversation.UserMessageKindSkillActivation {
		displayText = strings.TrimSpace(req.UserVisibleText)
	} else if strings.TrimSpace(req.RawQuery) != "" {
		displayText = strings.TrimSpace(req.RawQuery)
	}
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	return r.messageService.Persist(ctx, messagepkg.PersistInput{
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
	})
}

func persistedUserTurnText(req conversation.ChatRequest) string {
	if req.UserMessageKind == conversation.UserMessageKindSkillActivation {
		return strings.TrimSpace(req.UserVisibleText)
	}
	return req.Query
}
