package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func (r *Resolver) storeRound(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string) error {
	return r.storeRoundWithOptions(ctx, req, messages, modelID, storeRoundOptions{})
}

type storeRoundOptions struct {
	AllowPendingToolCalls   bool
	SkipMemory              bool
	AllowEmptyAssistantText bool
	MessageMetadataByIndex  map[int]map[string]any
}

func (r *Resolver) storeRoundWithOptions(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string, opts storeRoundOptions) error {
	_, err := r.storeRoundWithOptionsResult(ctx, req, messages, modelID, opts)
	return err
}

func (r *Resolver) storeRoundWithOptionsResult(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage, modelID string, opts storeRoundOptions) ([]messagepkg.Message, error) {
	fullRound := make([]conversation.ModelMessage, 0, len(messages))

	// When the user message was already persisted by a channel adapter, skip
	// the duplicate from the round. Otherwise keep it so that user + assistant
	// messages are written atomically (deferred persistence).
	skipUserQuery := req.UserMessagePersisted || req.ReusePersistedUserMessage
	for _, m := range messages {
		if skipUserQuery && m.Role == "user" && strings.TrimSpace(m.TextContent()) == strings.TrimSpace(req.Query) {
			skipUserQuery = false // only skip the first matching user message
			continue
		}
		fullRound = append(fullRound, m)
	}
	if !opts.AllowPendingToolCalls {
		fullRound = repairToolCallClosures(fullRound, syntheticToolClosureError)
	}

	// Filter out empty assistant messages (content: []) that result from LLM
	// returning no useful output (e.g., context window overflow). These provide
	// no value and pollute the conversation history, causing subsequent turns
	// to also produce empty responses.
	filtered := make([]conversation.ModelMessage, 0, len(fullRound))
	for _, m := range fullRound {
		if m.Role == "assistant" && isEmptyAssistantMessage(m) && !opts.AllowEmptyAssistantText {
			r.logger.Warn("skipping empty assistant message in storeRound",
				slog.String("bot_id", req.BotID),
			)
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	persisted := r.storeMessages(ctx, req, filtered, modelID, opts)
	if !opts.SkipMemory && !req.SkipMemoryExtraction {
		go r.storeMemory(context.WithoutCancel(ctx), req, filtered)
	}

	return persisted, nil
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
	var plain string
	if err := json.Unmarshal(m.Content, &plain); err == nil && strings.TrimSpace(plain) == "" {
		return true
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
	meta = mergeMetadata(meta, workspaceTargetMetadata(req.WorkspaceTarget))
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
		if msg.Role == "user" {
			messageSenderChannelIdentityID = senderChannelIdentityID
			messageSenderUserID = senderUserID

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
			isOriginalQuery := (ownText != "" && ownText == strings.TrimSpace(req.Query)) || isOriginalSkillActivation

			if isOriginalQuery {
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
			}
		} else if strings.TrimSpace(req.ExternalMessageID) != "" {
			sourceReplyToMessageID = req.ExternalMessageID
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

func workspaceTargetMetadata(target *conversation.WorkspaceTarget) map[string]any {
	if target == nil || strings.TrimSpace(target.TargetID) == "" {
		return nil
	}
	return map[string]any{
		"execution_location": map[string]any{
			"target_id":      strings.TrimSpace(target.TargetID),
			"kind":           strings.TrimSpace(target.Kind),
			"name":           strings.TrimSpace(target.Name),
			"workspace_path": strings.TrimSpace(target.WorkspacePath),
		},
	}
}

func (r *Resolver) persistSessionWorkspaceTarget(ctx context.Context, req conversation.ChatRequest) error {
	if r == nil || r.sessionService == nil || req.WorkspaceTarget == nil || strings.TrimSpace(req.SessionID) == "" {
		return nil
	}
	sess, err := r.sessionService.Get(ctx, req.SessionID)
	if err != nil {
		return err
	}
	metadata := make(map[string]any, len(sess.Metadata)+2)
	for key, value := range sess.Metadata {
		metadata[key] = value
	}
	target := req.WorkspaceTarget
	metadata["workspace_target_id"] = strings.TrimSpace(target.TargetID)
	metadata["workspace_target"] = map[string]any{
		"target_id":      strings.TrimSpace(target.TargetID),
		"kind":           strings.TrimSpace(target.Kind),
		"name":           strings.TrimSpace(target.Name),
		"workspace_path": strings.TrimSpace(target.WorkspacePath),
	}
	_, err = r.sessionService.UpdateMetadata(ctx, req.SessionID, metadata)
	return err
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

func (r *Resolver) persistSessionRuntimeSnapshot(ctx context.Context, req conversation.ChatRequest) (string, string) {
	sessionMode := strings.TrimSpace(req.SessionType)
	runtimeType := strings.TrimSpace(req.RuntimeType)
	if sessionMode != "" && runtimeType != "" {
		return sessionMode, runtimeType
	}
	if r != nil && r.sessionService != nil && strings.TrimSpace(req.SessionID) != "" {
		if sess, err := r.sessionService.Get(ctx, req.SessionID); err == nil {
			if sessionMode == "" {
				sessionMode = strings.TrimSpace(sess.SessionMode)
			}
			if runtimeType == "" {
				runtimeType = strings.TrimSpace(sess.RuntimeType)
			}
		}
	}
	if sessionMode == "" {
		sessionMode = "chat"
	}
	if runtimeType == "" {
		runtimeType = "model"
	}
	return sessionMode, runtimeType
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
		ref.StorageKey = attachmentpkg.MetadataString(att.Metadata, attachmentpkg.MetadataKeyStorageKey)
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

func buildInteractionMetadata(req conversation.ChatRequest) map[string]any {
	meta := map[string]any{}
	reply := map[string]any{}
	if v := strings.TrimSpace(req.SourceReplyToMessageID); v != "" {
		reply["message_id"] = v
	}
	if v := strings.TrimSpace(req.ReplySender); v != "" {
		reply["sender"] = v
	}
	if v := strings.TrimSpace(req.ReplyPreview); v != "" {
		reply["preview"] = v
	}
	if attachments := chatAttachmentMetadata(req.ReplyAttachments); len(attachments) > 0 {
		reply["attachments"] = attachments
	}
	if len(reply) > 0 {
		meta["reply"] = reply
	}

	forward := map[string]any{}
	if v := strings.TrimSpace(req.ForwardMessageID); v != "" {
		forward["message_id"] = v
	}
	if v := strings.TrimSpace(req.ForwardFromUserID); v != "" {
		forward["from_user_id"] = v
	}
	if v := strings.TrimSpace(req.ForwardFromConversationID); v != "" {
		forward["from_conversation_id"] = v
	}
	if v := strings.TrimSpace(req.ForwardSender); v != "" {
		forward["sender"] = v
	}
	if req.ForwardDate > 0 {
		forward["date"] = req.ForwardDate
	}
	if len(forward) > 0 {
		meta["forward"] = forward
	}
	if requestedSkills := publicRequestedSkillMetadata(req.RequestedSkills); len(requestedSkills) > 0 {
		meta["model_requested_skills"] = requestedSkills
	}
	if kind := strings.TrimSpace(req.UserMessageKind); kind != "" {
		meta["user_message_kind"] = kind
	}
	if activation := publicSkillActivationMetadata(req.SkillActivation); activation != nil {
		meta["skill_activation"] = activation
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func publicSkillActivationMetadata(activation *conversation.SkillActivation) map[string]any {
	if activation == nil {
		return nil
	}
	out := map[string]any{}
	if prompt := strings.TrimSpace(activation.Prompt); prompt != "" {
		out["prompt"] = prompt
	}
	skills := make([]map[string]any, 0, len(activation.Skills))
	for _, skill := range activation.Skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		item := map[string]any{"name": name}
		if displayName := strings.TrimSpace(skill.DisplayName); displayName != "" {
			item["display_name"] = displayName
		}
		if description := strings.TrimSpace(skill.Description); description != "" {
			item["description"] = description
		}
		if sourceKind := strings.TrimSpace(skill.SourceKind); sourceKind != "" {
			item["source_kind"] = sourceKind
		}
		if state := strings.TrimSpace(skill.State); state != "" {
			item["state"] = state
		}
		skills = append(skills, item)
	}
	if len(skills) > 0 {
		out["skills"] = skills
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func publicRequestedSkillMetadata(items []conversation.RequestedSkillContext) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := strings.TrimSpace(item.Identity)
		if key == "" {
			key = name + "\x00" + strings.TrimSpace(item.SourceKind) + "\x00" + strings.TrimSpace(item.OpaqueSourceID)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		entry := map[string]any{"name": name}
		if sourceKind := strings.TrimSpace(item.SourceKind); sourceKind != "" {
			entry["source_kind"] = sourceKind
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func chatAttachmentMetadata(attachments []conversation.ChatAttachment) []map[string]any {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(attachments))
	for _, att := range attachments {
		item := conversation.BundleFromChatAttachment(att).ToMap()
		if len(item) > 0 {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeMetadata(base, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
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
	return senderChannelIdentityID, senderUserID
}

// LinkOutboundAssets links bot-generated assets to the assistant message that
// produced them. Assets carrying a `tool_call_id` metadata entry are anchored
// to the assistant message containing that tool call, so a history rebuild
// keeps the live ordering (e.g. a generated image stays above the closing
// text). Assets without one fall back to the latest assistant message. When
// sessionID is provided, the search is scoped to that session; otherwise it
// falls back to a bot-wide search.
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
	// A single turn can span many rows (the originating tool-call assistant
	// message, its tool-result row, and any follow-up assistant/tool messages
	// such as sending the image to several channels). The window must comfortably
	// cover a whole turn so tool-call anchoring below finds the originating
	// message instead of silently falling back to the latest assistant message.
	const anchorSearchWindow = 50
	if strings.TrimSpace(sessionID) != "" {
		msgs, err = r.messageService.ListLatestBySession(ctx, sessionID, anchorSearchWindow)
	} else {
		msgs, err = r.messageService.ListLatest(ctx, botID, anchorSearchWindow)
	}
	if err != nil {
		r.logger.Warn("LinkOutboundAssets: list latest failed", slog.Any("error", err))
		return
	}

	latestAssistantID := ""
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			latestAssistantID = msg.ID
			break
		}
	}
	if latestAssistantID == "" {
		r.logger.Warn("LinkOutboundAssets: no assistant message found", slog.String("bot_id", botID))
		return
	}

	byMessage := map[string][]messagepkg.AssetRef{}
	var order []string
	for _, asset := range assets {
		targetID := latestAssistantID
		if toolCallID, _ := asset.Metadata["tool_call_id"].(string); strings.TrimSpace(toolCallID) != "" {
			if id := findAssistantMessageForToolCall(msgs, strings.TrimSpace(toolCallID)); id != "" {
				targetID = id
			} else {
				r.logger.Debug("LinkOutboundAssets: tool call not found in search window, anchoring to latest assistant",
					slog.String("tool_call_id", strings.TrimSpace(toolCallID)))
			}
		}
		if _, ok := byMessage[targetID]; !ok {
			order = append(order, targetID)
		}
		byMessage[targetID] = append(byMessage[targetID], asset)
	}
	for _, id := range order {
		group := byMessage[id]
		for i := range group {
			group[i].Ordinal = i
		}
		if linkErr := r.messageService.LinkAssets(ctx, id, group); linkErr != nil {
			r.logger.Warn("LinkOutboundAssets: link failed", slog.Any("error", linkErr))
		}
	}
}

// findAssistantMessageForToolCall returns the ID of the assistant message
// whose serialized content contains the given tool call ID as an exact JSON
// string token. Tool call IDs are unique opaque tokens, so a quoted match
// cannot collide with ordinary reply text.
func findAssistantMessageForToolCall(msgs []messagepkg.Message, toolCallID string) string {
	needle := `"` + toolCallID + `"`
	for _, msg := range msgs {
		if msg.Role != "assistant" || len(msg.Content) == 0 {
			continue
		}
		if strings.Contains(string(msg.Content), needle) {
			return msg.ID
		}
	}
	return ""
}
