package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messagesource"
)

func (r *Resolver) storeMessages(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string, opts storeRoundOptions) []messagepkg.Message {
	if r.messageService == nil {
		return nil
	}
	if strings.TrimSpace(req.BotID) == "" {
		return nil
	}

	// Check bot setting for full tool result persistence.
	pruneToolResults := true
	if botSettings, err := r.loadBotSettings(ctx, req.BotID); err == nil {
		pruneToolResults = !botSettings.PersistFullToolResults
	}
	meta := buildRouteMetadata(req)
	senderChannelIdentityID, senderUserID := r.resolvePersistSenderIDs(ctx, req)
	sessionMode, runtimeType := r.persistSessionRuntimeSnapshot(ctx, req)

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

	turnRequestMessageID := ""
	if req.UserMessagePersisted || req.ReusePersistedUserMessage {
		turnRequestMessageID = strings.TrimSpace(req.PersistedUserMessageID)
	}
	latestExternalMessageID := strings.TrimSpace(req.ExternalMessageID)
	legacyOriginalUserEligible := !req.UserMessagePersisted && !req.ReusePersistedUserMessage
	persistInputs := make([]messagepkg.PersistInput, 0, len(messages))
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
		persistMeta := meta
		sourceContext := messagesource.Context{}
		if msg.Role == "user" {
			// Only the user message whose text matches req.Query is the
			// "real" turn-leading query from the user. Other user-role
			// messages in this round are synthetic — typically:
			//   1. Mid-turn IM platform injects (the user typed again
			//      while the bot was working).
			//   2. The image-only user message that the read-media tool
			//      decoration appends after a successful image read so
			//      that the next LLM step can see the image.
			// For (2) the message has no text content; for both (1) and
			// (2), splatting req.RawQuery / req.ExternalMessageID /
			// req.EventID across them was wrong: it forced the UI to
			// display the original query text on a synthetic image-only
			// turn (the read-tool case), and falsely linked unrelated
			// messages to the same inbound IM event.
			ownText := strings.TrimSpace(msg.TextContent())
			isOriginalSkillActivation := req.UserMessageKind == conversation.UserMessageKindSkillActivation &&
				strings.TrimSpace(req.Query) == "" &&
				ownText == "" &&
				i == 0
			isOriginalQuery := legacyOriginalUserEligible && i == 0 &&
				((ownText != "" && ownText == strings.TrimSpace(req.Query)) || isOriginalSkillActivation)
			updatesLatestExternalMessage := false

			if receipt := msg.UserReceipt; receipt != nil {
				origin := receipt.Origin.Values()
				updatesLatestExternalMessage = true
				messageSenderChannelIdentityID = origin.SenderChannelIdentityID
				messageSenderUserID = origin.SenderUserID
				externalMessageID = origin.ExternalMessageID
				sourceReplyToMessageID = origin.SourceReplyToMessageID
				messageEventID = origin.EventID
				displayText = receipt.DisplayText
				assets = chatAttachmentsToAssetRefs(receipt.Attachments)
				persistMeta = receipt.Metadata
				sourceContext = origin.Context
			} else if isOriginalQuery {
				updatesLatestExternalMessage = true
				messageSenderChannelIdentityID = senderChannelIdentityID
				messageSenderUserID = senderUserID
				externalMessageID = req.ExternalMessageID
				sourceReplyToMessageID = req.SourceReplyToMessageID
				messageEventID = req.EventID
				switch {
				case strings.TrimSpace(req.UserVisibleText) != "" || req.UserMessageKind == conversation.UserMessageKindSkillActivation:
					displayText = strings.TrimSpace(req.UserVisibleText)
				case req.RawQuery != "":
					displayText = req.RawQuery
				default:
					displayText = strings.TrimSpace(req.Query)
				}
				assets = chatAttachmentsToAssetRefs(req.Attachments)
				persistMeta = mergeMetadata(meta, buildInteractionMetadata(req))
			} else {
				// Use the message's own text as display text. For the
				// read-media image-only injection this is empty, so
				// DisplayContent stays empty and ConvertMessagesToUITurns
				// drops the turn entirely (no text + no assets).
				displayText = ownText
				persistMeta = nil
			}
			if updatesLatestExternalMessage {
				latestExternalMessageID = strings.TrimSpace(externalMessageID)
			}
		} else if latestExternalMessageID != "" {
			sourceReplyToMessageID = latestExternalMessageID
		}
		if i == lastAssistantIdx && len(outboundAssets) > 0 {
			assets = append(assets, outboundAssets...)
		}
		if extraMeta := opts.MessageMetadataByIndex[i]; len(extraMeta) > 0 {
			persistMeta = mergeMetadata(persistMeta, extraMeta)
		}
		persistInputs = append(persistInputs, messagepkg.PersistInput{
			BotID:                   req.BotID,
			SessionID:               req.SessionID,
			SenderChannelIdentityID: messageSenderChannelIdentityID,
			SenderUserID:            messageSenderUserID,
			ExternalMessageID:       externalMessageID,
			SourceReplyToMessageID:  sourceReplyToMessageID,
			Role:                    msg.Role,
			Content:                 content,
			Metadata:                persistMeta,
			Usage:                   msg.Usage,
			Assets:                  assets,
			ModelID:                 modelID,
			EventID:                 messageEventID,
			SourceContext:           sourceContext,
			DisplayText:             displayText,
			SessionMode:             sessionMode,
			RuntimeType:             runtimeType,
			TurnRequestMessageID:    turnRequestMessageID,
			SkipHistoryTurn:         req.SkipHistoryTurn,
		})
	}
	if batcher, ok := r.messageService.(messagepkg.ToolTailRoundPersister); ok {
		if persisted, handled, err := batcher.PersistToolTailRound(ctx, persistInputs); handled || err != nil {
			if err != nil {
				r.logger.Warn("persist tool tail round failed", slog.Any("error", err))
				return nil
			}
			return persisted
		}
	}
	return r.persistMessageInputs(ctx, persistInputs, turnRequestMessageID)
}

func (r *Resolver) persistMessageInputs(ctx context.Context, inputs []messagepkg.PersistInput, initialTurnRequestMessageID string) []messagepkg.Message {
	persisted := make([]messagepkg.Message, 0, len(inputs))
	turnRequestMessageID := strings.TrimSpace(initialTurnRequestMessageID)
	for _, input := range inputs {
		if !input.SkipHistoryTurn {
			input.TurnRequestMessageID = turnRequestMessageID
		}
		persistedMessage, err := r.messageService.Persist(ctx, input)
		if err != nil {
			r.logger.Warn("persist message failed", slog.Any("error", err))
			continue
		}
		if strings.EqualFold(strings.TrimSpace(input.Role), "user") && !input.SkipHistoryTurn {
			turnRequestMessageID = persistedMessage.ID
		}
		persisted = append(persisted, persistedMessage)
	}
	return persisted
}
