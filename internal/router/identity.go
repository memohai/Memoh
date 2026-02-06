package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contacts"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
)

type IdentityDecision struct {
	Stop  bool
	Reply channel.Message
}

type InboundIdentity struct {
	BotID           string
	SessionID       string
	ChannelConfigID string
	ExternalID      string
	UserID          string
	ContactID       string
	Contact         contacts.Contact
}

type IdentityState struct {
	Identity InboundIdentity
	Decision *IdentityDecision
}

type identityContextKey struct{}

func WithIdentityState(ctx context.Context, state IdentityState) context.Context {
	return context.WithValue(ctx, identityContextKey{}, state)
}

func IdentityStateFromContext(ctx context.Context) (IdentityState, bool) {
	if ctx == nil {
		return IdentityState{}, false
	}
	raw := ctx.Value(identityContextKey{})
	if raw == nil {
		return IdentityState{}, false
	}
	state, ok := raw.(IdentityState)
	return state, ok
}

type IdentityResolver struct {
	store        channel.ConfigStore
	contacts     ContactService
	policy       PolicyService
	preauth      PreauthService
	logger       *slog.Logger
	unboundReply string
	preauthReply string
}

type PolicyService interface {
	Resolve(ctx context.Context, botID string) (policy.Decision, error)
}

type PreauthService interface {
	Get(ctx context.Context, token string) (preauth.Key, error)
	MarkUsed(ctx context.Context, id string) (preauth.Key, error)
}

func NewIdentityResolver(log *slog.Logger, store channel.ConfigStore, contacts ContactService, policyService PolicyService, preauthService PreauthService, unboundReply, preauthReply string) *IdentityResolver {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(unboundReply) == "" {
		unboundReply = "当前不允许陌生人访问，请联系管理员。"
	}
	if strings.TrimSpace(preauthReply) == "" {
		preauthReply = "授权成功，请继续使用。"
	}
	return &IdentityResolver{
		store:        store,
		contacts:     contacts,
		policy:       policyService,
		preauth:      preauthService,
		logger:       log.With(slog.String("component", "channel_identity")),
		unboundReply: unboundReply,
		preauthReply: preauthReply,
	}
}

func (r *IdentityResolver) Middleware() channel.Middleware {
	return func(next channel.InboundHandler) channel.InboundHandler {
		return func(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error {
			state, err := r.Resolve(ctx, cfg, msg)
			if err != nil {
				return err
			}
			return next(WithIdentityState(ctx, state), cfg, msg)
		}
	}
}

func (r *IdentityResolver) Resolve(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (IdentityState, error) {
	if r.store == nil || r.contacts == nil || r.policy == nil {
		return IdentityState{}, fmt.Errorf("identity resolver not configured")
	}

	botID := strings.TrimSpace(msg.BotID)
	if botID == "" {
		botID = cfg.BotID
	}
	normalizedMsg := msg
	normalizedMsg.BotID = botID

	sessionID := normalizedMsg.SessionID()
	channelConfigID := cfg.ID
	if channel.IsConfigless(msg.Channel) {
		channelConfigID = ""
	}
	externalID := extractExternalIdentity(msg)

	state := IdentityState{
		Identity: InboundIdentity{
			BotID:           botID,
			SessionID:       sessionID,
			ChannelConfigID: channelConfigID,
			ExternalID:      externalID,
		},
	}

	session, err := r.store.GetChannelSession(ctx, sessionID)
	if err != nil && r.logger != nil {
		r.logger.Error("get user by session failed", slog.String("session_id", sessionID), slog.Any("error", err))
	}
	userID := strings.TrimSpace(session.UserID)
	contactID := strings.TrimSpace(session.ContactID)

	if userID == "" {
		userID, err = r.store.ResolveUserBinding(ctx, msg.Channel, channel.BindingCriteriaFromIdentity(msg.Sender))
		if err == nil && userID != "" {
			_ = r.store.UpsertChannelSession(ctx, sessionID, botID, channelConfigID, userID, contactID, string(msg.Channel), strings.TrimSpace(msg.ReplyTarget), extractThreadID(msg), buildSessionMetadata(msg))
		}
	}

	var contact contacts.Contact
	if contactID == "" && userID != "" {
		contact, err = r.contacts.GetByUserID(ctx, botID, userID)
		if err != nil {
			displayName := extractDisplayName(msg)
			contact, err = r.contacts.Create(ctx, contacts.CreateRequest{
				BotID:       botID,
				UserID:      userID,
				DisplayName: displayName,
				Status:      "active",
			})
		}
		if err == nil {
			contactID = contact.ID
			if externalID != "" {
				_, _ = r.contacts.UpsertChannel(ctx, botID, contactID, msg.Channel.String(), externalID, nil)
			}
		}
	}

	if contactID == "" && externalID != "" {
		binding, err := r.contacts.GetByChannelIdentity(ctx, botID, msg.Channel.String(), externalID)
		if err == nil {
			contactID = binding.ContactID
		}
	}

	if contactID == "" {
		decision, err := r.policy.Resolve(ctx, botID)
		if err != nil {
			return state, err
		}
		if decision.AllowGuest {
			displayName := extractDisplayName(msg)
			contact, err = r.contacts.CreateGuest(ctx, botID, displayName)
			if err == nil {
				contactID = contact.ID
				if externalID != "" {
					_, _ = r.contacts.UpsertChannel(ctx, botID, contactID, msg.Channel.String(), externalID, nil)
				}
			}
		} else {
			if handled, decision, err := r.tryHandlePreauthKey(ctx, normalizedMsg, externalID); handled {
				state.Decision = &decision
				return state, err
			}
			state.Decision = &IdentityDecision{
				Stop:  true,
				Reply: channel.Message{Text: r.unboundReply},
			}
			return state, nil
		}
	}

	if contactID != "" && contact.ID == "" {
		loaded, err := r.contacts.GetByID(ctx, contactID)
		if err == nil {
			contact = loaded
		}
	}

	if contactID != "" {
		_ = r.store.UpsertChannelSession(ctx, sessionID, botID, channelConfigID, userID, contactID, string(msg.Channel), strings.TrimSpace(msg.ReplyTarget), extractThreadID(msg), buildSessionMetadata(msg))
	}

	state.Identity.UserID = userID
	state.Identity.ContactID = contactID
	state.Identity.Contact = contact
	return state, nil
}

func (r *IdentityResolver) tryHandlePreauthKey(ctx context.Context, msg channel.InboundMessage, externalID string) (bool, IdentityDecision, error) {
	tokenText := strings.TrimSpace(msg.Message.PlainText())
	if tokenText == "" || r.preauth == nil {
		return false, IdentityDecision{}, nil
	}
	key, err := r.preauth.Get(ctx, tokenText)
	if err != nil {
		if errors.Is(err, preauth.ErrKeyNotFound) {
			return false, IdentityDecision{}, nil
		}
		return true, IdentityDecision{}, err
	}
	reply := func(text string) IdentityDecision {
		return IdentityDecision{
			Stop:  true,
			Reply: channel.Message{Text: text},
		}
	}
	if !key.UsedAt.IsZero() {
		return true, reply("预授权码已使用。"), nil
	}
	if !key.ExpiresAt.IsZero() && time.Now().UTC().After(key.ExpiresAt) {
		return true, reply("预授权码已过期，请重新获取。"), nil
	}
	if key.BotID != msg.BotID {
		return true, reply("预授权码不匹配。"), nil
	}
	if externalID == "" {
		return true, reply("无法识别当前账号，授权失败。"), nil
	}
	displayName := extractDisplayName(msg)
	contact, err := r.contacts.CreateGuest(ctx, msg.BotID, displayName)
	if err != nil {
		return true, reply("授权失败，请稍后重试。"), nil
	}
	if _, err := r.contacts.UpsertChannel(ctx, msg.BotID, contact.ID, msg.Channel.String(), externalID, nil); err != nil {
		return true, reply("授权失败，请稍后重试。"), nil
	}
	_ = r.store.UpsertChannelSession(ctx, msg.SessionID(), msg.BotID, "", "", contact.ID, msg.Channel.String(), strings.TrimSpace(msg.ReplyTarget), extractThreadID(msg), buildSessionMetadata(msg))
	_, _ = r.preauth.MarkUsed(ctx, key.ID)
	return true, reply(r.preauthReply), nil
}

func extractExternalIdentity(msg channel.InboundMessage) string {
	if strings.TrimSpace(msg.Sender.ExternalID) != "" {
		return strings.TrimSpace(msg.Sender.ExternalID)
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("open_id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("user_id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("username")); value != "" {
		return value
	}
	return strings.TrimSpace(msg.Sender.DisplayName)
}

func extractDisplayName(msg channel.InboundMessage) string {
	if strings.TrimSpace(msg.Sender.DisplayName) != "" {
		return strings.TrimSpace(msg.Sender.DisplayName)
	}
	if strings.TrimSpace(msg.Sender.ExternalID) != "" {
		return strings.TrimSpace(msg.Sender.ExternalID)
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("username")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("user_id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("open_id")); value != "" {
		return value
	}
	return ""
}

func extractThreadID(msg channel.InboundMessage) string {
	if msg.Message.Thread != nil && strings.TrimSpace(msg.Message.Thread.ID) != "" {
		return strings.TrimSpace(msg.Message.Thread.ID)
	}
	if strings.TrimSpace(msg.Conversation.ThreadID) != "" {
		return strings.TrimSpace(msg.Conversation.ThreadID)
	}
	return ""
}

func buildSessionMetadata(msg channel.InboundMessage) map[string]any {
	metadata := map[string]any{}
	if strings.TrimSpace(msg.Source) != "" {
		metadata["source"] = strings.TrimSpace(msg.Source)
	}
	if strings.TrimSpace(msg.Message.ID) != "" {
		metadata["message_id"] = strings.TrimSpace(msg.Message.ID)
	}
	if strings.TrimSpace(msg.Conversation.Type) != "" {
		metadata["conversation_type"] = strings.TrimSpace(msg.Conversation.Type)
	}
	if strings.TrimSpace(msg.Conversation.Name) != "" {
		metadata["conversation_name"] = strings.TrimSpace(msg.Conversation.Name)
	}
	if !msg.ReceivedAt.IsZero() {
		metadata["received_at"] = msg.ReceivedAt.UTC().Format(time.RFC3339Nano)
	}
	return metadata
}
