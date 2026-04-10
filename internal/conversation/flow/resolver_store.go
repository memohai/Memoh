package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func (r *Resolver) storeRound(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string) error {
	fullRound := make([]conversation.ModelMessage, 0, len(messages))

	// When the user message was already persisted by a channel adapter, skip
	// the duplicate from the round. Otherwise keep it so that user + assistant
	// messages are written atomically (deferred persistence).
	skipUserQuery := req.UserMessagePersisted
	for _, m := range messages {
		if skipUserQuery && m.Role == "user" && strings.TrimSpace(m.TextContent()) == strings.TrimSpace(req.Query) {
			skipUserQuery = false // only skip the first matching user message
			continue
		}
		fullRound = append(fullRound, m)
	}

	// Filter out empty assistant messages (content: []) that result from LLM
	// returning no useful output (e.g., context window overflow). These provide
	// no value and pollute the conversation history, causing subsequent turns
	// to also produce empty responses.
	filtered := make([]conversation.ModelMessage, 0, len(fullRound))
	for _, m := range fullRound {
		if m.Role == "assistant" && isEmptyAssistantMessage(m) {
			r.logger.Warn("skipping empty assistant message in storeRound",
				slog.String("bot_id", req.BotID),
			)
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return nil
	}

	r.storeMessages(ctx, req, filtered, modelID)
	go r.storeMemory(context.WithoutCancel(ctx), req, filtered)

	return nil
}

// isEmptyAssistantMessage returns true if an assistant message has no
// meaningful content: no text, no tool calls, and no attachments.
func isEmptyAssistantMessage(m conversation.ModelMessage) bool {
	if len(m.ToolCalls) > 0 {
		return false
	}
	text := strings.TrimSpace(m.TextContent())
	if text != "" {
		return false
	}
	// Check if content is empty array "[]" or null/empty
	content := strings.TrimSpace(string(m.Content))
	return content == "" || content == "[]" || content == "null"
}

// StoreRound persists SDK messages as a complete round (assistant + tool
// output) into bot_history_messages with full metadata, usage tracking,
// and memory extraction. Used by the discuss driver so it shares the same
// persistence quality as chat mode.
func (r *Resolver) StoreRound(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform string, sdkMessages []sdk.Message, modelID string) error {
	modelMessages := sdkMessagesToModelMessages(sdkMessages)
	req := conversation.ChatRequest{
		BotID:                   botID,
		ChatID:                  botID,
		SessionID:               sessionID,
		SourceChannelIdentityID: channelIdentityID,
		CurrentChannel:          currentPlatform,
		UserMessagePersisted:    true,
	}
	return r.storeRound(ctx, req, modelMessages, modelID)
}

func (r *Resolver) storeMessages(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string) {
	if r.messageService == nil {
		return
	}
	if strings.TrimSpace(req.BotID) == "" {
		return
	}

	// Check bot setting for full tool result persistence.
	pruneToolResults := true
	if botSettings, err := r.loadBotSettings(ctx, req.BotID); err == nil {
		pruneToolResults = !botSettings.PersistFullToolResults
	}
	meta := buildRouteMetadata(req)
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)

	// Determine the last assistant message index for outbound asset attachment.
	lastAssistantIdx := -1
	if req.OutboundAssetCollector != nil {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" {
				lastAssistantIdx = i
				break
			}
		}
	}
	var outboundAssets []messagepkg.AssetRef
	if lastAssistantIdx >= 0 {
		outboundAssets = outboundAssetRefsToMessageRefs(req.OutboundAssetCollector())
	}

	for i, msg := range messages {
		msg = normalizeUserMessageContent(msg)

		// Prune tool results at store time to reduce DB bloat.
		// This prevents ~10KB+ tool outputs from being stored verbatim.
		if pruneToolResults {
			if pruned, changed := pruneMessageForGateway(msg); changed {
				msg = pruned
			}
		}

		content, err := json.Marshal(msg)
		if err != nil {
			r.logger.Warn("storeMessages: marshal failed", slog.Any("error", err))
			continue
		}
		messageSenderChannelIdentityID := ""
		messageSenderUserID := ""
		externalMessageID := ""
		sourceReplyToMessageID := ""
		messageEventID := ""
		displayText := ""
		assets := []messagepkg.AssetRef(nil)
		if msg.Role == "user" {
			messageSenderChannelIdentityID = senderChannelIdentityID
			messageSenderUserID = senderUserID
			externalMessageID = req.ExternalMessageID
			messageEventID = req.EventID
			if req.RawQuery != "" {
				displayText = req.RawQuery
			} else {
				displayText = strings.TrimSpace(req.Query)
			}
			if strings.TrimSpace(msg.TextContent()) == strings.TrimSpace(req.Query) {
				assets = chatAttachmentsToAssetRefs(req.Attachments)
			}
		} else if strings.TrimSpace(req.ExternalMessageID) != "" {
			sourceReplyToMessageID = req.ExternalMessageID
		}
		if i == lastAssistantIdx && len(outboundAssets) > 0 {
			assets = append(assets, outboundAssets...)
		}
		if _, err := r.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:                   req.BotID,
			SessionID:               req.SessionID,
			SenderChannelIdentityID: messageSenderChannelIdentityID,
			SenderUserID:            messageSenderUserID,
			ExternalMessageID:       externalMessageID,
			SourceReplyToMessageID:  sourceReplyToMessageID,
			Role:                    msg.Role,
			Content:                 content,
			Metadata:                meta,
			Usage:                   msg.Usage,
			Assets:                  assets,
			ModelID:                 modelID,
			EventID:                 messageEventID,
			DisplayText:             displayText,
		}); err != nil {
			r.logger.Warn("persist message failed", slog.Any("error", err))
		}
	}
}

// outboundAssetRefsToMessageRefs converts outbound asset refs from the streaming
// collector into message-level asset refs for persistence.
func outboundAssetRefsToMessageRefs(refs []conversation.OutboundAssetRef) []messagepkg.AssetRef {
	if len(refs) == 0 {
		return nil
	}
	result := make([]messagepkg.AssetRef, 0, len(refs))
	for _, ref := range refs {
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		role := ref.Role
		if strings.TrimSpace(role) == "" {
			role = "attachment"
		}
		result = append(result, messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        role,
			Ordinal:     ref.Ordinal,
			Mime:        ref.Mime,
			SizeBytes:   ref.SizeBytes,
			StorageKey:  ref.StorageKey,
			Name:        ref.Name,
			Metadata:    ref.Metadata,
		})
	}
	return result
}

// chatAttachmentsToAssetRefs converts ChatAttachment slice to message AssetRef slice.
// Only attachments that carry a content_hash are included.
func chatAttachmentsToAssetRefs(attachments []conversation.ChatAttachment) []messagepkg.AssetRef {
	if len(attachments) == 0 {
		return nil
	}
	refs := make([]messagepkg.AssetRef, 0, len(attachments))
	for i, att := range attachments {
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		ref := messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        "attachment",
			Ordinal:     i,
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
			Name:        strings.TrimSpace(att.Name),
			Metadata:    att.Metadata,
		}
		if att.Metadata != nil {
			if sk, ok := att.Metadata["storage_key"].(string); ok {
				ref.StorageKey = sk
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func buildRouteMetadata(req conversation.ChatRequest) map[string]any {
	if strings.TrimSpace(req.RouteID) == "" && strings.TrimSpace(req.CurrentChannel) == "" {
		return nil
	}
	meta := map[string]any{}
	if strings.TrimSpace(req.RouteID) != "" {
		meta["route_id"] = req.RouteID
	}
	if strings.TrimSpace(req.CurrentChannel) != "" {
		meta["platform"] = req.CurrentChannel
	}
	return meta
}

func (r *Resolver) resolvePersistSenderIDs(ctx context.Context, req conversation.ChatRequest) (string, string) {
	channelIdentityID := strings.TrimSpace(req.SourceChannelIdentityID)
	userID := strings.TrimSpace(req.UserID)

	senderChannelIdentityID := ""
	if r.isExistingChannelIdentityID(ctx, channelIdentityID) {
		senderChannelIdentityID = channelIdentityID
	}

	senderUserID := ""
	if r.isExistingUserID(ctx, userID) {
		senderUserID = userID
	}
	if senderUserID == "" && senderChannelIdentityID != "" {
		if linked := r.linkedUserIDFromChannelIdentity(ctx, senderChannelIdentityID); linked != "" {
			senderUserID = linked
		}
	}
	return senderChannelIdentityID, senderUserID
}

// LinkOutboundAssets links bot-generated assets to the latest assistant
// message. When sessionID is provided, the search is scoped to that session;
// otherwise it falls back to a bot-wide search.
// Used by the WebSocket path where attachment ingestion happens after message
// persistence.
func (r *Resolver) LinkOutboundAssets(ctx context.Context, botID, sessionID string, assets []messagepkg.AssetRef) {
	if r.messageService == nil || len(assets) == 0 || strings.TrimSpace(botID) == "" {
		return
	}
	var (
		msgs []messagepkg.Message
		err  error
	)
	if strings.TrimSpace(sessionID) != "" {
		msgs, err = r.messageService.ListLatestBySession(ctx, sessionID, 5)
	} else {
		msgs, err = r.messageService.ListLatest(ctx, botID, 5)
	}
	if err != nil {
		r.logger.Warn("LinkOutboundAssets: list latest failed", slog.Any("error", err))
		return
	}
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			if linkErr := r.messageService.LinkAssets(ctx, msg.ID, assets); linkErr != nil {
				r.logger.Warn("LinkOutboundAssets: link failed", slog.Any("error", linkErr))
			}
			return
		}
	}
	r.logger.Warn("LinkOutboundAssets: no assistant message found", slog.String("bot_id", botID))
}
