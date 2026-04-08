package dingtalk

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/memohai/dingtalk-stream-sdk-go/chatbot"
	dtsdk "github.com/memohai/dingtalk-stream-sdk-go/client"

	"github.com/memohai/memoh/internal/channel"
)

// DingTalkAdapter implements the Memoh channel adapter for DingTalk bots.
// It uses the DingTalk Stream SDK (WebSocket) for inbound messages and
// the DingTalk OpenAPI (HTTP) for outbound messages.
type DingTalkAdapter struct {
	logger *slog.Logger

	// mu guards the clients and apiClients maps.
	mu         sync.RWMutex
	clients    map[string]*dtsdk.StreamClient
	apiClients map[string]*apiClient

	// webhookCache stores recent sessionWebhook contexts keyed by msgId.
	webhookCache *sessionWebhookCache
}

// NewDingTalkAdapter creates a new DingTalkAdapter.
func NewDingTalkAdapter(log *slog.Logger) *DingTalkAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &DingTalkAdapter{
		logger:       log.With(slog.String("adapter", "dingtalk")),
		clients:      make(map[string]*dtsdk.StreamClient),
		apiClients:   make(map[string]*apiClient),
		webhookCache: newSessionWebhookCache(30 * time.Minute),
	}
}

// SetAssetOpener is a no-op placeholder to match the adapter registration pattern.
func (*DingTalkAdapter) SetAssetOpener(_ any) {}

func (*DingTalkAdapter) Type() channel.ChannelType { return Type }

func (*DingTalkAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "DingTalk",
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Markdown:    true,
			Attachments: true,
			Media:       true,
			ChatTypes:   []string{channel.ConversationTypePrivate, channel.ConversationTypeGroup},
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"appKey":    {Type: channel.FieldString, Required: true, Title: "App Key (Client ID)"},
				"appSecret": {Type: channel.FieldSecret, Required: true, Title: "App Secret (Client Secret)"},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"user_id":              {Type: channel.FieldString, Title: "User ID (single chat)"},
				"open_conversation_id": {Type: channel.FieldString, Title: "Open Conversation ID (group chat)"},
				"display_name":         {Type: channel.FieldString, Title: "Display Name"},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "user:{userId} | group:{openConversationId}",
			Hints: []channel.TargetHint{
				{Label: "User", Example: "user:user123"},
				{Label: "Group", Example: "group:cidXXXXXXXX"},
			},
		},
	}
}

// NormalizeConfig validates and normalizes a DingTalk channel config map.
func (*DingTalkAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

// NormalizeUserConfig validates and normalizes a DingTalk user binding config map.
func (*DingTalkAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

// NormalizeTarget normalizes a raw target string to canonical form.
func (*DingTalkAdapter) NormalizeTarget(raw string) string { return normalizeTarget(raw) }

// ResolveTarget converts a user config map to a canonical delivery target string.
func (*DingTalkAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

// MatchBinding checks whether a user binding config matches the given criteria.
func (*DingTalkAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

// BuildUserConfig constructs a user config map from a channel Identity.
func (*DingTalkAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

// DiscoverSelf retrieves the bot's own identity from DingTalk.
// DiscoverSelf uses AppKey as the OpenAPI robotCode parameter (钉钉统一应用下二者一致)。
func (a *DingTalkAdapter) DiscoverSelf(ctx context.Context, credentials map[string]any) (map[string]any, string, error) {
	cfg, err := parseConfig(credentials)
	if err != nil {
		return nil, "", err
	}
	cli := newAPIClient(cfg.AppKey, cfg.AppSecret)
	info, err := cli.getBotInfo(ctx, cfg.AppKey)
	if err != nil {
		a.logger.Warn("dingtalk: getBotInfo failed, using appKey as identity",
			slog.String("app_key", cfg.AppKey),
			slog.Any("error", err),
		)
		return map[string]any{"app_key": cfg.AppKey}, cfg.AppKey, nil
	}
	externalID := strings.TrimSpace(info.Result.RobotCode)
	if externalID == "" {
		externalID = cfg.AppKey
	}
	return map[string]any{
		"app_key": cfg.AppKey,
		"name":    strings.TrimSpace(info.Result.Name),
	}, externalID, nil
}

// Connect establishes a DingTalk Stream WebSocket connection and begins receiving messages.
func (a *DingTalkAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}

	apiCli := newAPIClient(parsed.AppKey, parsed.AppSecret)

	streamCli := dtsdk.NewStreamClient(
		dtsdk.WithAppCredential(dtsdk.NewAppCredentialConfig(parsed.AppKey, parsed.AppSecret)),
		dtsdk.WithAutoReconnect(true),
	)

	streamCli.RegisterChatBotCallbackRouter(a.newChatBotHandler(cfg, handler))

	key := cfg.ID
	a.mu.Lock()
	a.clients[key] = streamCli
	a.apiClients[key] = apiCli
	a.mu.Unlock()

	if err := streamCli.Start(ctx); err != nil {
		a.mu.Lock()
		delete(a.clients, key)
		delete(a.apiClients, key)
		a.mu.Unlock()
		return nil, err
	}

	stop := func(context.Context) error {
		// Disable reconnect before closing to prevent the reconnect loop from restarting.
		streamCli.AutoReconnect = false
		streamCli.Close()
		a.mu.Lock()
		if current, ok := a.clients[key]; ok && current == streamCli {
			delete(a.clients, key)
		}
		delete(a.apiClients, key)
		a.mu.Unlock()
		return nil
	}
	return channel.NewConnection(cfg, stop), nil
}

// Send delivers an outbound message to a DingTalk user or group.
// It first tries the session webhook (if cached and valid), then falls back to the OpenAPI.
func (a *DingTalkAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return errors.New("dingtalk: target is required")
	}
	if msg.Message.IsEmpty() {
		return errors.New("dingtalk: message is empty")
	}

	// Session webhook fast path: immediate reply without access_token round-trip.
	if whCtx, ok := a.lookupWebhook(msg.Message.Reply); ok && whCtx.isValid() {
		body, bodyErr := buildWebhookBody(msg.Message)
		if bodyErr == nil {
			apiCli := a.getOrNewAPIClient(cfg)
			if webhookErr := apiCli.sendViaWebhook(ctx, whCtx.SessionWebhook, body); webhookErr == nil {
				return nil
			}
			// Webhook failed (possibly expired mid-flight); fall through to OpenAPI.
		}
	}

	return a.sendViaAPI(ctx, cfg, msg)
}

// sendViaAPI sends a message through the DingTalk OpenAPI.
func (a *DingTalkAdapter) sendViaAPI(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	apiCli := a.getOrNewAPIClient(cfg)

	msgKey, msgParam, err := buildAPIPayload(msg.Message)
	if err != nil {
		return err
	}

	kind, id, ok := parseTarget(msg.Target)
	if !ok {
		return errors.New("dingtalk: invalid target")
	}
	switch kind {
	case "user":
		return apiCli.sendToUser(ctx, parsed.AppKey, []string{id}, msgKey, msgParam)
	case "group":
		return apiCli.sendToGroup(ctx, parsed.AppKey, id, msgKey, msgParam)
	default:
		return errors.New("dingtalk: unknown target kind: " + kind)
	}
}

// OpenStream creates a new accumulating outbound stream for the given target.
func (a *DingTalkAdapter) OpenStream(ctx context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("dingtalk: target is required")
	}
	reply := opts.Reply
	if reply == nil && strings.TrimSpace(opts.SourceMessageID) != "" {
		reply = &channel.ReplyRef{
			Target:    target,
			MessageID: strings.TrimSpace(opts.SourceMessageID),
		}
	}
	return &dingtalkOutboundStream{
		adapter: a,
		cfg:     cfg,
		target:  target,
		reply:   reply,
	}, nil
}

// newChatBotHandler returns the DingTalk SDK chatbot callback for the given channel config.
func (a *DingTalkAdapter) newChatBotHandler(cfg channel.ChannelConfig, handler channel.InboundHandler) chatbot.IChatBotMessageHandler {
	return func(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		if data == nil {
			return nil, nil
		}

		// Cache the session webhook so that Send can use the fast-reply path.
		if strings.TrimSpace(data.MsgId) != "" && strings.TrimSpace(data.SessionWebhook) != "" {
			a.rememberWebhook(data.MsgId, sessionWebhookContext{
				SessionWebhook: data.SessionWebhook,
				ExpiredTime:    data.SessionWebhookExpiredTime,
				ConversationID: data.ConversationId,
				SenderID:       data.SenderId,
			})
		}

		if handler == nil {
			return nil, nil
		}
		msg, ok := buildInboundMessage(data)
		if !ok {
			return nil, nil
		}
		msg.BotID = cfg.BotID
		if err := handler(ctx, cfg, msg); err != nil {
			a.logger.Error("dingtalk: inbound handler error",
				slog.String("config_id", cfg.ID),
				slog.Any("error", err),
			)
		}
		return nil, nil
	}
}

func (a *DingTalkAdapter) getOrNewAPIClient(cfg channel.ChannelConfig) *apiClient {
	a.mu.RLock()
	cli := a.apiClients[cfg.ID]
	a.mu.RUnlock()
	if cli != nil {
		return cli
	}
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return newAPIClient("", "")
	}
	return newAPIClient(parsed.AppKey, parsed.AppSecret)
}

func (a *DingTalkAdapter) lookupWebhook(reply *channel.ReplyRef) (sessionWebhookContext, bool) {
	if reply == nil {
		return sessionWebhookContext{}, false
	}
	msgID := strings.TrimSpace(reply.MessageID)
	if msgID == "" {
		return sessionWebhookContext{}, false
	}
	return a.webhookCache.get(msgID)
}

func (a *DingTalkAdapter) rememberWebhook(msgID string, whCtx sessionWebhookContext) {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" || strings.TrimSpace(whCtx.SessionWebhook) == "" {
		return
	}
	if whCtx.CreatedAt.IsZero() {
		whCtx.CreatedAt = time.Now().UTC()
	}
	a.webhookCache.put(msgID, whCtx)
}
