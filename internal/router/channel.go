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
	"github.com/memohai/memoh/internal/contacts"
)

// ChatGateway 抽象聊天能力，避免路由层直接依赖具体实现。
type ChatGateway interface {
	Chat(ctx context.Context, req chat.ChatRequest) (chat.ChatResponse, error)
}

type ContactService interface {
	GetByID(ctx context.Context, contactID string) (contacts.Contact, error)
	GetByUserID(ctx context.Context, botID, userID string) (contacts.Contact, error)
	GetByChannelIdentity(ctx context.Context, botID, platform, externalID string) (contacts.ContactChannel, error)
	Create(ctx context.Context, req contacts.CreateRequest) (contacts.Contact, error)
	CreateGuest(ctx context.Context, botID, displayName string) (contacts.Contact, error)
	UpsertChannel(ctx context.Context, botID, contactID, platform, externalID string, metadata map[string]any) (contacts.ContactChannel, error)
}

const (
	silentReplyToken       = "NO_REPLY"
	minDuplicateTextLength = 10
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// ChannelInboundProcessor 将 channel 入站消息路由到 chat，并返回可发送的回复。
type ChannelInboundProcessor struct {
	chat      ChatGateway
	logger    *slog.Logger
	jwtSecret string
	tokenTTL  time.Duration
	identity  *IdentityResolver
}

func NewChannelInboundProcessor(log *slog.Logger, store channel.ConfigStore, chatGateway ChatGateway, contactService ContactService, policyService PolicyService, preauthService PreauthService, jwtSecret string, tokenTTL time.Duration) *ChannelInboundProcessor {
	if log == nil {
		log = slog.Default()
	}
	if tokenTTL <= 0 {
		tokenTTL = 5 * time.Minute
	}
	identityResolver := NewIdentityResolver(log, store, contactService, policyService, preauthService, "", "")
	return &ChannelInboundProcessor{
		chat:      chatGateway,
		logger:    log.With(slog.String("component", "channel_router")),
		jwtSecret: strings.TrimSpace(jwtSecret),
		tokenTTL:  tokenTTL,
		identity:  identityResolver,
	}
}

func (p *ChannelInboundProcessor) IdentityMiddleware() channel.Middleware {
	if p == nil || p.identity == nil {
		return nil
	}
	return p.identity.Middleware()
}

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

	sessionToken := ""
	if p.jwtSecret != "" && strings.TrimSpace(msg.ReplyTarget) != "" {
		signed, _, err := auth.GenerateSessionToken(auth.SessionToken{
			BotID:       identity.BotID,
			Platform:    msg.Channel.String(),
			ReplyTarget: strings.TrimSpace(msg.ReplyTarget),
			SessionID:   identity.SessionID,
			ContactID:   identity.ContactID,
		}, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue session token failed", slog.Any("error", err))
			}
		} else {
			sessionToken = signed
		}
	}

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
	desc, _ := channel.GetChannelDescriptor(msg.Channel)
	resp, err := p.chat.Chat(ctx, chat.ChatRequest{
		BotID:           identity.BotID,
		SessionID:       identity.SessionID,
		Token:           token,
		UserID:          identity.UserID,
		ContactID:       identity.ContactID,
		ContactName:     strings.TrimSpace(identity.Contact.DisplayName),
		ContactAlias:    strings.TrimSpace(identity.Contact.Alias),
		ReplyTarget:     strings.TrimSpace(msg.ReplyTarget),
		SessionToken:    sessionToken,
		Query:           text,
		CurrentPlatform: msg.Channel.String(),
		Platforms:       []string{msg.Channel.String()},
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Error("chat gateway failed", slog.String("channel", msg.Channel.String()), slog.String("user_id", identity.UserID), slog.Any("error", err))
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
	sentTexts, suppressReplies := collectMessageToolContext(resp.Messages, msg.Channel, target)
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
				Type:     partType,
				Text:     part.Text,
				URL:      part.URL,
				Styles:   normalizeContentPartStyles(part.Styles),
				Language: part.Language,
				UserID:   part.UserID,
				Emoji:    part.Emoji,
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
	Platform string           `json:"platform"`
	Target   string           `json:"target"`
	UserID   string           `json:"user_id"`
	Text     string           `json:"text"`
	Message  *channel.Message `json:"message"`
}

type toolCall struct {
	Name      string
	Arguments string
}

func collectMessageToolContext(messages []chat.GatewayMessage, channelType channel.ChannelType, replyTarget string) ([]string, bool) {
	if len(messages) == 0 {
		return nil, false
	}
	sentTexts := make([]string, 0)
	suppressReplies := false
	for _, msg := range messages {
		for _, call := range extractToolCalls(msg) {
			if call.Name != "send_message" {
				continue
			}
			var args sendMessageToolArgs
			if !parseToolArguments(call.Arguments, &args) {
				continue
			}
			messageText := strings.TrimSpace(extractSendMessageText(args))
			if messageText != "" {
				sentTexts = append(sentTexts, messageText)
			}
			if shouldSuppressForToolCall(args, channelType, replyTarget) {
				suppressReplies = true
			}
		}
	}
	return sentTexts, suppressReplies
}

func extractToolCalls(msg chat.GatewayMessage) []toolCall {
	calls := make([]toolCall, 0)
	if msg == nil {
		return calls
	}
	if rawCalls, ok := msg["tool_calls"].([]any); ok {
		for _, raw := range rawCalls {
			call, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, args := parseToolCall(call)
			if name == "" {
				continue
			}
			calls = append(calls, toolCall{Name: name, Arguments: args})
		}
	}
	if fn, ok := msg["function_call"].(map[string]any); ok {
		name := readString(fn["name"])
		args := readString(fn["arguments"])
		if name != "" {
			calls = append(calls, toolCall{Name: name, Arguments: args})
		}
	}
	if fn, ok := msg["functionCall"].(map[string]any); ok {
		name := readString(fn["name"])
		args := readString(fn["arguments"])
		if name != "" {
			calls = append(calls, toolCall{Name: name, Arguments: args})
		}
	}
	return calls
}

func parseToolCall(call map[string]any) (string, string) {
	if call == nil {
		return "", ""
	}
	name := ""
	args := ""
	if fn, ok := call["function"].(map[string]any); ok {
		name = readString(fn["name"])
		args = readString(fn["arguments"])
	}
	if name == "" {
		name = readString(call["name"])
	}
	if args == "" {
		args = readString(call["arguments"])
	}
	return name, args
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

func shouldSuppressForToolCall(args sendMessageToolArgs, channelType channel.ChannelType, replyTarget string) bool {
	platform := strings.TrimSpace(args.Platform)
	if platform == "" {
		platform = string(channelType)
	}
	if !strings.EqualFold(platform, string(channelType)) {
		return false
	}
	target := strings.TrimSpace(args.Target)
	if target == "" && strings.TrimSpace(args.UserID) == "" {
		target = replyTarget
	}
	if strings.TrimSpace(target) == "" || strings.TrimSpace(replyTarget) == "" {
		return false
	}
	normalizedTarget := normalizeReplyTarget(channelType, target)
	normalizedReply := normalizeReplyTarget(channelType, replyTarget)
	if normalizedTarget == "" || normalizedReply == "" {
		return false
	}
	return normalizedTarget == normalizedReply
}

func normalizeReplyTarget(channelType channel.ChannelType, target string) string {
	normalized, ok := channel.NormalizeTarget(channelType, target)
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

func readString(value any) string {
	if raw, ok := value.(string); ok {
		return raw
	}
	return ""
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
