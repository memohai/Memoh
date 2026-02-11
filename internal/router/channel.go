package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/chat"
)

// ChatGateway abstracts the chat capability to avoid direct coupling in the router.
type ChatGateway interface {
	Chat(ctx context.Context, req chat.ChatRequest) (chat.ChatResponse, error)
}

const (
	silentReplyToken       = "NO_REPLY"
	minDuplicateTextLength = 10
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// ChatService resolves and manages chats.
type ChatService interface {
	ResolveChat(ctx context.Context, botID, platform, conversationID, threadID, conversationType, userID, channelConfigID, replyTarget string) (chat.ResolveChatResult, error)
	PersistMessage(ctx context.Context, chatID, botID, routeID, senderChannelIdentityID, senderUserID, platform, externalMessageID, role string, content json.RawMessage, metadata map[string]any) (chat.Message, error)
}

// ChannelInboundProcessor routes channel inbound messages to the chat gateway.
type ChannelInboundProcessor struct {
	chat        ChatGateway
	chatService ChatService
	registry    *channel.Registry
	logger      *slog.Logger
	jwtSecret   string
	tokenTTL    time.Duration
	identity    *IdentityResolver
}

// NewChannelInboundProcessor creates a processor with channel identity-based resolution.
func NewChannelInboundProcessor(
	log *slog.Logger,
	registry *channel.Registry,
	chatService ChatService,
	chatGateway ChatGateway,
	channelIdentityService ChannelIdentityService,
	memberService BotMemberService,
	policyService PolicyService,
	preauthService PreauthService,
	bindService BindService,
	jwtSecret string,
	tokenTTL time.Duration,
) *ChannelInboundProcessor {
	if log == nil {
		log = slog.Default()
	}
	if tokenTTL <= 0 {
		tokenTTL = 5 * time.Minute
	}
	identityResolver := NewIdentityResolver(log, registry, channelIdentityService, memberService, policyService, preauthService, bindService, "", "")
	return &ChannelInboundProcessor{
		chat:        chatGateway,
		chatService: chatService,
		registry:    registry,
		logger:      log.With(slog.String("component", "channel_router")),
		jwtSecret:   strings.TrimSpace(jwtSecret),
		tokenTTL:    tokenTTL,
		identity:    identityResolver,
	}
}

// IdentityMiddleware returns the identity resolution middleware.
func (p *ChannelInboundProcessor) IdentityMiddleware() channel.Middleware {
	if p == nil || p.identity == nil {
		return nil
	}
	return p.identity.Middleware()
}

// HandleInbound processes an inbound channel message through identity resolution and chat gateway.
func (p *ChannelInboundProcessor) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, sender channel.ReplySender) error {
	if p.chat == nil {
		return fmt.Errorf("channel inbound processor not configured")
	}
	if sender == nil {
		return fmt.Errorf("reply sender not configured")
	}
	text := buildInboundQuery(msg.Message)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	state, err := p.requireIdentity(ctx, cfg, msg)
	if err != nil {
		return err
	}
	if state.Decision != nil && state.Decision.Stop {
		if !state.Decision.Reply.IsEmpty() {
			return sender.Send(ctx, channel.OutboundMessage{
				Target:  strings.TrimSpace(msg.ReplyTarget),
				Message: state.Decision.Reply,
			})
		}
		return nil
	}

	identity := state.Identity

	// Resolve or create the chat via chat_routes.
	if p.chatService == nil {
		return fmt.Errorf("chat service not configured")
	}
	resolved, err := p.chatService.ResolveChat(ctx, identity.BotID,
		msg.Channel.String(), msg.Conversation.ID, extractThreadID(msg),
		msg.Conversation.Type, identity.UserID, identity.ChannelConfigID,
		strings.TrimSpace(msg.ReplyTarget))
	if err != nil {
		return fmt.Errorf("resolve chat: %w", err)
	}
	if !shouldTriggerAssistantResponse(msg) && !identity.ForceReply {
		p.persistInboundOnly(ctx, resolved, identity, msg, text)
		return nil
	}

	// Issue chat token for reply routing.
	chatToken := ""
	if p.jwtSecret != "" && strings.TrimSpace(msg.ReplyTarget) != "" {
		signed, _, err := auth.GenerateChatToken(auth.ChatToken{
			BotID:             identity.BotID,
			ChatID:            resolved.ChatID,
			RouteID:           resolved.RouteID,
			UserID:            identity.UserID,
			ChannelIdentityID: identity.ChannelIdentityID,
		}, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue chat token failed", slog.Any("error", err))
			}
		} else {
			chatToken = signed
		}
	}

	// Issue user JWT for downstream calls.
	token := ""
	if identity.UserID != "" && p.jwtSecret != "" {
		signed, _, err := auth.GenerateToken(identity.UserID, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue channel token failed", slog.Any("error", err))
			}
		} else {
			token = "Bearer " + signed
		}
	}

	var desc channel.Descriptor
	if p.registry != nil {
		desc, _ = p.registry.GetDescriptor(msg.Channel)
	}
	resp, err := p.chat.Chat(ctx, chat.ChatRequest{
		BotID:                   identity.BotID,
		ChatID:                  resolved.ChatID,
		Token:                   token,
		UserID:                  identity.UserID,
		SourceChannelIdentityID: identity.ChannelIdentityID,
		DisplayName:             identity.DisplayName,
		RouteID:                 resolved.RouteID,
		ChatToken:               chatToken,
		ExternalMessageID:       strings.TrimSpace(msg.Message.ID),
		Query:                   text,
		CurrentChannel:          msg.Channel.String(),
		Channels:                []string{msg.Channel.String()},
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Error(
				"chat gateway failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("channel_identity_id", identity.ChannelIdentityID),
				slog.String("user_id", identity.UserID),
				slog.Any("error", err),
			)
		}
		return err
	}
	outputs := chat.ExtractAssistantOutputs(resp.Messages)
	if len(outputs) == 0 {
		return nil
	}
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return fmt.Errorf("reply target missing")
	}
	sentTexts, suppressReplies := collectMessageToolContext(p.registry, resp.Messages, msg.Channel, target)
	if suppressReplies {
		return nil
	}
	for _, output := range outputs {
		outMessage := buildChannelMessage(output, desc.Capabilities)
		if outMessage.IsEmpty() {
			continue
		}
		plainText := strings.TrimSpace(outMessage.PlainText())
		if isSilentReplyText(plainText) {
			continue
		}
		if isMessagingToolDuplicate(plainText, sentTexts) {
			continue
		}
		if err := sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: outMessage,
		}); err != nil {
			return err
		}
	}
	return nil
}

func shouldTriggerAssistantResponse(msg channel.InboundMessage) bool {
	if isDirectConversationType(msg.Conversation.Type) {
		return true
	}
	if metadataBool(msg.Metadata, "is_mentioned") {
		return true
	}
	if metadataBool(msg.Metadata, "is_reply_to_bot") {
		return true
	}
	return hasCommandPrefix(msg.Message.PlainText(), msg.Metadata)
}

func isDirectConversationType(conversationType string) bool {
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	return ct == "" || ct == "p2p" || ct == "private" || ct == "direct"
}

func hasCommandPrefix(text string, metadata map[string]any) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	prefixes := []string{"/"}
	if metadata != nil {
		if raw, ok := metadata["command_prefix"]; ok {
			if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" {
				prefixes = []string{value}
			}
		}
		if raw, ok := metadata["command_prefixes"]; ok {
			if parsed := parseCommandPrefixes(raw); len(parsed) > 0 {
				prefixes = parsed
			}
		}
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func parseCommandPrefixes(raw any) []string {
	if items, ok := raw.([]string); ok {
		result := make([]string, 0, len(items))
		for _, item := range items {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			result = append(result, value)
		}
		return result
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, ok := metadata[key]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (p *ChannelInboundProcessor) persistInboundOnly(ctx context.Context, resolved chat.ResolveChatResult, identity InboundIdentity, msg channel.InboundMessage, query string) {
	if p.chatService == nil {
		return
	}
	chatID := strings.TrimSpace(resolved.ChatID)
	botID := strings.TrimSpace(identity.BotID)
	if chatID == "" || botID == "" {
		return
	}
	payload, err := json.Marshal(chat.ModelMessage{
		Role:    "user",
		Content: chat.NewTextContent(query),
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("marshal passive inbound failed", slog.Any("error", err))
		}
		return
	}
	meta := map[string]any{
		"route_id":     resolved.RouteID,
		"platform":     msg.Channel.String(),
		"trigger_mode": "passive_sync",
	}
	if _, err := p.chatService.PersistMessage(
		ctx,
		chatID,
		botID,
		strings.TrimSpace(resolved.RouteID),
		strings.TrimSpace(identity.ChannelIdentityID),
		strings.TrimSpace(identity.UserID),
		msg.Channel.String(),
		strings.TrimSpace(msg.Message.ID),
		"user",
		payload,
		meta,
	); err != nil && p.logger != nil {
		p.logger.Warn("persist passive inbound failed", slog.Any("error", err))
	}
}

func buildChannelMessage(output chat.AssistantOutput, capabilities channel.ChannelCapabilities) channel.Message {
	msg := channel.Message{}
	if strings.TrimSpace(output.Content) != "" {
		msg.Text = strings.TrimSpace(output.Content)
		if containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	if len(output.Parts) == 0 {
		return msg
	}
	if capabilities.RichText {
		parts := make([]channel.MessagePart, 0, len(output.Parts))
		for _, part := range output.Parts {
			if !contentPartHasValue(part) {
				continue
			}
			partType := normalizeContentPartType(part.Type)
			parts = append(parts, channel.MessagePart{
				Type:              partType,
				Text:              part.Text,
				URL:               part.URL,
				Styles:            normalizeContentPartStyles(part.Styles),
				Language:          part.Language,
				ChannelIdentityID: part.ChannelIdentityID,
				Emoji:             part.Emoji,
			})
		}
		if len(parts) > 0 {
			msg.Parts = parts
			msg.Format = channel.MessageFormatRich
		}
		return msg
	}
	textParts := make([]string, 0, len(output.Parts))
	for _, part := range output.Parts {
		if !contentPartHasValue(part) {
			continue
		}
		textParts = append(textParts, strings.TrimSpace(contentPartText(part)))
	}
	if len(textParts) > 0 {
		msg.Text = strings.Join(textParts, "\n")
		if msg.Format == "" && containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	return msg
}

func containsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	patterns := []string{
		`\\*\\*[^*]+\\*\\*`,
		`\\*[^*]+\\*`,
		`~~[^~]+~~`,
		"`[^`]+`",
		"```[\\s\\S]*```",
		`\\[.+\\]\\(.+\\)`,
		`(?m)^#{1,6}\\s`,
		`(?m)^[-*]\\s`,
		`(?m)^\\d+\\.\\s`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

func contentPartHasValue(part chat.ContentPart) bool {
	if strings.TrimSpace(part.Text) != "" {
		return true
	}
	if strings.TrimSpace(part.URL) != "" {
		return true
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return true
	}
	return false
}

func contentPartText(part chat.ContentPart) string {
	if strings.TrimSpace(part.Text) != "" {
		return part.Text
	}
	if strings.TrimSpace(part.URL) != "" {
		return part.URL
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return part.Emoji
	}
	return ""
}

func buildInboundQuery(message channel.Message) string {
	text := strings.TrimSpace(message.PlainText())
	if len(message.Attachments) == 0 {
		return text
	}
	lines := make([]string, 0, len(message.Attachments)+1)
	if text != "" {
		lines = append(lines, text)
	}
	for _, att := range message.Attachments {
		label := strings.TrimSpace(att.Name)
		if label == "" {
			label = strings.TrimSpace(att.URL)
		}
		if label == "" {
			label = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[attachment:%s] %s", att.Type, label))
	}
	return strings.Join(lines, "\n")
}

func normalizeContentPartType(raw string) channel.MessagePartType {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "link":
		return channel.MessagePartLink
	case "code_block":
		return channel.MessagePartCodeBlock
	case "mention":
		return channel.MessagePartMention
	case "emoji":
		return channel.MessagePartEmoji
	default:
		return channel.MessagePartText
	}
}

func normalizeContentPartStyles(styles []string) []channel.MessageTextStyle {
	if len(styles) == 0 {
		return nil
	}
	result := make([]channel.MessageTextStyle, 0, len(styles))
	for _, style := range styles {
		switch strings.TrimSpace(strings.ToLower(style)) {
		case "bold":
			result = append(result, channel.MessageStyleBold)
		case "italic":
			result = append(result, channel.MessageStyleItalic)
		case "strikethrough", "lineThrough":
			result = append(result, channel.MessageStyleStrikethrough)
		case "code":
			result = append(result, channel.MessageStyleCode)
		default:
			continue
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type sendMessageToolArgs struct {
	Platform          string           `json:"platform"`
	Target            string           `json:"target"`
	ChannelIdentityID string           `json:"channel_identity_id"`
	Text              string           `json:"text"`
	Message           *channel.Message `json:"message"`
}

func collectMessageToolContext(registry *channel.Registry, messages []chat.ModelMessage, channelType channel.ChannelType, replyTarget string) ([]string, bool) {
	if len(messages) == 0 {
		return nil, false
	}
	var sentTexts []string
	suppressReplies := false
	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != "send_message" {
				continue
			}
			var args sendMessageToolArgs
			if !parseToolArguments(tc.Function.Arguments, &args) {
				continue
			}
			if text := strings.TrimSpace(extractSendMessageText(args)); text != "" {
				sentTexts = append(sentTexts, text)
			}
			if shouldSuppressForToolCall(registry, args, channelType, replyTarget) {
				suppressReplies = true
			}
		}
	}
	return sentTexts, suppressReplies
}

func parseToolArguments(raw string, out any) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err == nil {
		return true
	}
	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	if strings.TrimSpace(decoded) == "" {
		return false
	}
	return json.Unmarshal([]byte(decoded), out) == nil
}

func extractSendMessageText(args sendMessageToolArgs) string {
	if strings.TrimSpace(args.Text) != "" {
		return strings.TrimSpace(args.Text)
	}
	if args.Message == nil {
		return ""
	}
	return strings.TrimSpace(args.Message.PlainText())
}

func shouldSuppressForToolCall(registry *channel.Registry, args sendMessageToolArgs, channelType channel.ChannelType, replyTarget string) bool {
	platform := strings.TrimSpace(args.Platform)
	if platform == "" {
		platform = string(channelType)
	}
	if !strings.EqualFold(platform, string(channelType)) {
		return false
	}
	target := strings.TrimSpace(args.Target)
	if target == "" && strings.TrimSpace(args.ChannelIdentityID) == "" {
		target = replyTarget
	}
	if strings.TrimSpace(target) == "" || strings.TrimSpace(replyTarget) == "" {
		return false
	}
	normalizedTarget := normalizeReplyTarget(registry, channelType, target)
	normalizedReply := normalizeReplyTarget(registry, channelType, replyTarget)
	if normalizedTarget == "" || normalizedReply == "" {
		return false
	}
	return normalizedTarget == normalizedReply
}

func normalizeReplyTarget(registry *channel.Registry, channelType channel.ChannelType, target string) string {
	if registry == nil {
		return strings.TrimSpace(target)
	}
	normalized, ok := registry.NormalizeTarget(channelType, target)
	if ok && strings.TrimSpace(normalized) != "" {
		return strings.TrimSpace(normalized)
	}
	return strings.TrimSpace(target)
}

func isSilentReplyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	token := []rune(silentReplyToken)
	value := []rune(trimmed)
	if len(value) < len(token) {
		return false
	}
	if hasTokenPrefix(value, token) {
		return true
	}
	if hasTokenSuffix(value, token) {
		return true
	}
	return false
}

func hasTokenPrefix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	for i := range token {
		if value[i] != token[i] {
			return false
		}
	}
	if len(value) == len(token) {
		return true
	}
	return !isWordChar(value[len(token)])
}

func hasTokenSuffix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	start := len(value) - len(token)
	for i := range token {
		if value[start+i] != token[i] {
			return false
		}
	}
	if start == 0 {
		return true
	}
	return !isWordChar(value[start-1])
}

func isWordChar(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func normalizeTextForComparison(text string) string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return ""
	}
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(trimmed, " "))
}

func isMessagingToolDuplicate(text string, sentTexts []string) bool {
	if len(sentTexts) == 0 {
		return false
	}
	normalized := normalizeTextForComparison(text)
	if len(normalized) < minDuplicateTextLength {
		return false
	}
	for _, sent := range sentTexts {
		sentNormalized := normalizeTextForComparison(sent)
		if len(sentNormalized) < minDuplicateTextLength {
			continue
		}
		if strings.Contains(normalized, sentNormalized) || strings.Contains(sentNormalized, normalized) {
			return true
		}
	}
	return false
}

func (p *ChannelInboundProcessor) requireIdentity(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (IdentityState, error) {
	if state, ok := IdentityStateFromContext(ctx); ok {
		return state, nil
	}
	if p.identity == nil {
		return IdentityState{}, fmt.Errorf("identity resolver not configured")
	}
	return p.identity.Resolve(ctx, cfg, msg)
}
