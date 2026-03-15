package feishu

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/memohai/memoh/internal/channel"
)

type feishuSenderProfile struct {
	displayName string
	username    string
	avatarURL   string
}

const (
	feishuChatMembersPageSize = 100
	senderProfileCacheTTL     = 5 * time.Minute
	senderProfileSweepWindow  = 1 * time.Minute
)

type feishuSenderProfileLookup interface {
	LookupContact(ctx context.Context, openID, userID string) (feishuSenderProfile, error)
	LookupGroupMember(ctx context.Context, chatID, memberIDType, memberID string) (feishuSenderProfile, error)
}

type larkSenderProfileLookup struct {
	client *lark.Client
}

type cachedSenderProfile struct {
	profile   feishuSenderProfile
	expiresAt time.Time
}

func (l larkSenderProfileLookup) LookupContact(ctx context.Context, openID, userID string) (feishuSenderProfile, error) {
	return lookupSenderProfileFromContact(ctx, l.client, openID, userID)
}

func (l larkSenderProfileLookup) LookupGroupMember(ctx context.Context, chatID, memberIDType, memberID string) (feishuSenderProfile, error) {
	return lookupSenderProfileFromGroupMember(ctx, l.client, chatID, memberIDType, memberID)
}

// enrichSenderProfile fills sender display name / username for inbound messages.
// In group chats it prefers chat-specific aliases, then falls back to the global
// contact profile when no group-scoped name is available.
func (a *FeishuAdapter) enrichSenderProfile(ctx context.Context, cfg channel.ChannelConfig, event *larkim.P2MessageReceiveV1, msg *channel.InboundMessage) {
	if msg == nil {
		return
	}
	needDisplay := strings.TrimSpace(msg.Sender.DisplayName) == "" &&
		strings.TrimSpace(msg.Sender.Attribute("display_name")) == "" &&
		strings.TrimSpace(msg.Sender.Attribute("name")) == ""
	needUsername := strings.TrimSpace(msg.Sender.Attribute("username")) == ""
	if !needDisplay && !needUsername {
		return
	}

	openID := strings.TrimSpace(msg.Sender.Attribute("open_id"))
	userID := strings.TrimSpace(msg.Sender.Attribute("user_id"))
	if openID == "" && userID == "" {
		return
	}

	chatID := ""
	if event != nil && event.Event != nil && event.Event.Message != nil && event.Event.Message.ChatId != nil {
		chatID = strings.TrimSpace(*event.Event.Message.ChatId)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cacheKey := senderProfileCacheKey(cfg, openID, userID, chatID)
	if cached, ok := a.loadCachedSenderProfile(cacheKey); ok {
		applySenderProfile(msg, cached)
		return
	}

	profile, err := a.lookupSenderProfile(lookupCtx, cfg, openID, userID, chatID)
	if err != nil {
		if a.logger != nil {
			a.logger.Debug("feishu sender profile lookup failed",
				slog.String("config_id", cfg.ID),
				slog.String("open_id", openID),
				slog.String("user_id", userID),
				slog.String("chat_id", chatID),
				slog.Any("error", err),
			)
		}
	}
	profile, cacheable := finalizeSenderProfile(profile, openID, userID)
	if cacheable {
		a.storeCachedSenderProfile(cacheKey, profile)
	}
	applySenderProfile(msg, profile)
}

func (*FeishuAdapter) lookupSenderProfile(ctx context.Context, cfg channel.ChannelConfig, openID, userID, chatID string) (feishuSenderProfile, error) {
	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return feishuSenderProfile{}, err
	}
	client := feishuCfg.newClient()
	return lookupSenderProfileWithLookup(ctx, larkSenderProfileLookup{client: client}, openID, userID, chatID)
}

func lookupSenderProfileWithLookup(ctx context.Context, lookup feishuSenderProfileLookup, openID, userID, chatID string) (feishuSenderProfile, error) {
	if lookup == nil {
		return feishuSenderProfile{}, errors.New("sender profile lookup not configured")
	}

	var lastErr error
	chatID = strings.TrimSpace(chatID)
	chatID = strings.TrimPrefix(chatID, "chat_id:")

	if chatID != "" {
		if chatProfile, err := lookupGroupProfile(ctx, lookup, chatID, openID, userID); err == nil {
			if hasSenderProfile(chatProfile) {
				return chatProfile, nil
			}
		} else {
			lastErr = err
		}
	}

	if profile, err := lookup.LookupContact(ctx, openID, userID); err == nil {
		if hasSenderProfile(profile) {
			return profile, nil
		}
	} else {
		lastErr = err
	}

	if lastErr != nil {
		return feishuSenderProfile{}, lastErr
	}
	return feishuSenderProfile{}, nil
}

func lookupGroupProfile(ctx context.Context, lookup feishuSenderProfileLookup, chatID, openID, userID string) (feishuSenderProfile, error) {
	var lastErr error
	if chatID != "" && openID != "" {
		if profile, err := lookup.LookupGroupMember(ctx, chatID, "open_id", openID); err == nil {
			if hasSenderProfile(profile) {
				return profile, nil
			}
		} else {
			lastErr = err
		}
	}
	if chatID != "" && userID != "" {
		if profile, err := lookup.LookupGroupMember(ctx, chatID, "user_id", userID); err == nil {
			if hasSenderProfile(profile) {
				return profile, nil
			}
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return feishuSenderProfile{}, lastErr
	}
	return feishuSenderProfile{}, nil
}

func senderProfileCacheKey(cfg channel.ChannelConfig, openID, userID, chatID string) string {
	chatID = strings.TrimPrefix(strings.TrimSpace(chatID), "chat_id:")
	return strings.Join([]string{
		senderProfileCacheScope(cfg),
		chatID,
		strings.TrimSpace(openID),
		strings.TrimSpace(userID),
	}, "|")
}

func senderProfileCacheScope(cfg channel.ChannelConfig) string {
	parts := []string{strings.TrimSpace(cfg.ID)}
	if feishuCfg, err := parseConfig(cfg.Credentials); err == nil {
		parts = append(parts, strings.TrimSpace(feishuCfg.Region), strings.TrimSpace(feishuCfg.AppID))
	}
	return strings.Join(parts, "|")
}

func hasSenderProfile(profile feishuSenderProfile) bool {
	return strings.TrimSpace(profile.displayName) != "" ||
		strings.TrimSpace(profile.username) != "" ||
		strings.TrimSpace(profile.avatarURL) != ""
}

func finalizeSenderProfile(profile feishuSenderProfile, openID, userID string) (feishuSenderProfile, bool) {
	if hasSenderProfile(profile) {
		return profile, true
	}
	return fallbackSenderProfile(openID, userID), false
}

func (a *FeishuAdapter) loadCachedSenderProfile(key string) (feishuSenderProfile, bool) {
	if a == nil || strings.TrimSpace(key) == "" {
		return feishuSenderProfile{}, false
	}
	raw, ok := a.senderProfiles.Load(key)
	if !ok {
		return feishuSenderProfile{}, false
	}
	entry, ok := raw.(cachedSenderProfile)
	if !ok {
		a.senderProfiles.Delete(key)
		return feishuSenderProfile{}, false
	}
	if time.Now().After(entry.expiresAt) {
		a.senderProfiles.Delete(key)
		return feishuSenderProfile{}, false
	}
	return entry.profile, true
}

func (a *FeishuAdapter) storeCachedSenderProfile(key string, profile feishuSenderProfile) {
	if a == nil || strings.TrimSpace(key) == "" || !hasSenderProfile(profile) {
		return
	}
	now := time.Now()
	a.senderProfiles.Store(key, cachedSenderProfile{
		profile:   profile,
		expiresAt: now.Add(senderProfileCacheTTL),
	})
	a.maybeSweepExpiredSenderProfiles(now)
}

func (a *FeishuAdapter) maybeSweepExpiredSenderProfiles(now time.Time) {
	if a == nil {
		return
	}
	a.senderProfileSweepMu.Lock()
	defer a.senderProfileSweepMu.Unlock()
	if !a.senderProfileSweepAt.IsZero() && now.Sub(a.senderProfileSweepAt) < senderProfileSweepWindow {
		return
	}
	a.senderProfileSweepAt = now
	a.senderProfiles.Range(func(key, value any) bool {
		entry, ok := value.(cachedSenderProfile)
		if !ok || now.After(entry.expiresAt) {
			a.senderProfiles.Delete(key)
		}
		return true
	})
}

func lookupSenderProfileFromContact(ctx context.Context, client *lark.Client, openID, userID string) (feishuSenderProfile, error) {
	lookupID := strings.TrimSpace(openID)
	idType := larkcontact.UserIdTypeOpenId
	if lookupID == "" {
		lookupID = strings.TrimSpace(userID)
		idType = larkcontact.UserIdTypeUserId
	}
	if lookupID == "" {
		return feishuSenderProfile{}, errors.New("empty sender id")
	}
	req := larkcontact.NewGetUserReqBuilder().
		UserIdType(idType).
		UserId(lookupID).
		Build()
	resp, err := client.Contact.User.Get(ctx, req)
	if err != nil {
		return feishuSenderProfile{}, err
	}
	if resp == nil || !resp.Success() {
		code := 0
		msg := ""
		if resp != nil {
			code = resp.Code
			msg = strings.TrimSpace(resp.Msg)
		}
		return feishuSenderProfile{}, fmt.Errorf("feishu get user failed: code=%d msg=%s", code, msg)
	}
	if resp.Data == nil || resp.Data.User == nil {
		return feishuSenderProfile{}, errors.New("feishu get user returned empty user")
	}
	displayName := ptrStr(resp.Data.User.Name)
	username := ptrStr(resp.Data.User.Nickname)
	if username == "" {
		username = displayName
	}
	return feishuSenderProfile{
		displayName: displayName,
		username:    username,
		avatarURL:   feishuAvatarURL(resp.Data.User.Avatar),
	}, nil
}

func lookupSenderProfileFromGroupMember(ctx context.Context, client *lark.Client, chatID, memberIDType, memberID string) (feishuSenderProfile, error) {
	memberIDType = strings.TrimSpace(memberIDType)
	memberID = strings.TrimSpace(memberID)
	if memberIDType == "" || memberID == "" {
		return feishuSenderProfile{}, errors.New("empty member lookup input")
	}
	pageToken := ""
	for page := 0; page < 5; page++ {
		builder := larkim.NewGetChatMembersReqBuilder().
			ChatId(chatID).
			MemberIdType(memberIDType).
			PageSize(feishuChatMembersPageSize)
		if pageToken != "" {
			builder = builder.PageToken(pageToken)
		}
		resp, err := client.Im.ChatMembers.Get(ctx, builder.Build())
		if err != nil {
			return feishuSenderProfile{}, err
		}
		if resp == nil || !resp.Success() {
			code := 0
			msg := ""
			if resp != nil {
				code = resp.Code
				msg = strings.TrimSpace(resp.Msg)
			}
			return feishuSenderProfile{}, fmt.Errorf("feishu get chat members failed: code=%d msg=%s", code, msg)
		}
		if resp.Data == nil {
			return feishuSenderProfile{}, nil
		}
		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			if strings.TrimSpace(ptrStr(item.MemberId)) != memberID {
				continue
			}
			name := ptrStr(item.Name)
			username := firstNameFallback(name)
			if username == "" {
				username = name
			}
			return feishuSenderProfile{
				displayName: name,
				username:    username,
			}, nil
		}
		hasMore := resp.Data.HasMore != nil && *resp.Data.HasMore
		if !hasMore || resp.Data.PageToken == nil {
			break
		}
		pageToken = strings.TrimSpace(*resp.Data.PageToken)
		if pageToken == "" {
			break
		}
	}
	return feishuSenderProfile{}, nil
}

func fallbackSenderProfile(openID, userID string) feishuSenderProfile {
	openID = strings.TrimSpace(openID)
	userID = strings.TrimSpace(userID)
	username := userID
	if username == "" {
		username = openID
	}
	if username == "" {
		return feishuSenderProfile{}
	}
	displayName := username
	return feishuSenderProfile{
		displayName: displayName,
		username:    username,
	}
}

func firstNameFallback(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func applySenderProfile(msg *channel.InboundMessage, profile feishuSenderProfile) {
	if msg == nil {
		return
	}
	displayName := strings.TrimSpace(profile.displayName)
	username := strings.TrimSpace(profile.username)
	if username == "" {
		username = displayName
	}
	if msg.Sender.Attributes == nil {
		msg.Sender.Attributes = map[string]string{}
	}
	if displayName != "" {
		if strings.TrimSpace(msg.Sender.DisplayName) == "" {
			msg.Sender.DisplayName = displayName
		}
		if strings.TrimSpace(msg.Sender.Attributes["display_name"]) == "" {
			msg.Sender.Attributes["display_name"] = displayName
		}
		if strings.TrimSpace(msg.Sender.Attributes["name"]) == "" {
			msg.Sender.Attributes["name"] = displayName
		}
	}
	if username != "" && strings.TrimSpace(msg.Sender.Attributes["username"]) == "" {
		msg.Sender.Attributes["username"] = username
	}
	if avatarURL := strings.TrimSpace(profile.avatarURL); avatarURL != "" {
		if strings.TrimSpace(msg.Sender.Attributes["avatar_url"]) == "" {
			msg.Sender.Attributes["avatar_url"] = avatarURL
		}
	}
}
