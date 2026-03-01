package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

const (
	defaultDirectoryLimit = 50
	maxDirectoryLimit     = 200
)

func directoryLimit(n int) int {
	if n <= 0 {
		return defaultDirectoryLimit
	}
	if n > maxDirectoryLimit {
		return maxDirectoryLimit
	}
	return n
}

// ListPeers returns users the bot can reach. Telegram Bot API does not provide a list of users; returns empty.
func (a *TelegramAdapter) ListPeers(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

// ListGroups returns chats the bot is in. Telegram Bot API does not provide a list of chats; returns empty.
func (a *TelegramAdapter) ListGroups(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

// ListGroupMembers returns group managers for the given group (Telegram only exposes this list, not all members).
func (a *TelegramAdapter) ListGroupMembers(ctx context.Context, cfg channel.ChannelConfig, groupID string, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return nil, err
	}
	chatID, superGroupUsername := parseTelegramChatInput(strings.TrimSpace(groupID))
	if chatID == 0 && superGroupUsername == "" {
		return nil, fmt.Errorf("telegram list group members: invalid group id %q", groupID)
	}
	config := tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: chatID, SuperGroupUsername: superGroupUsername},
	}
	members, err := bot.GetChatAdministrators(config)
	if err != nil {
		return nil, fmt.Errorf("telegram get chat managers: %w", err)
	}
	limit := directoryLimit(query.Limit)
	entries := make([]channel.DirectoryEntry, 0, limit)
	for i, m := range members {
		if i >= limit {
			break
		}
		if m.User == nil {
			continue
		}
		e := a.telegramUserToEntryWithAvatar(bot, m.User)
		if query.Query != "" && !strings.Contains(strings.ToLower(e.Name+e.Handle), strings.ToLower(query.Query)) {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ResolveEntry resolves an input string to a user or group DirectoryEntry using getChat / getChatMember.
func (a *TelegramAdapter) ResolveEntry(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return channel.DirectoryEntry{}, err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return channel.DirectoryEntry{}, err
	}
	input = strings.TrimSpace(input)
	switch kind {
	case channel.DirectoryEntryUser:
		return a.resolveTelegramUser(ctx, bot, input)
	case channel.DirectoryEntryGroup:
		return a.resolveTelegramGroup(ctx, bot, input)
	default:
		return channel.DirectoryEntry{}, fmt.Errorf("telegram resolve entry: unsupported kind %q", kind)
	}
}

func (a *TelegramAdapter) resolveTelegramUser(ctx context.Context, bot *tgbotapi.BotAPI, input string) (channel.DirectoryEntry, error) {
	chatID, userID := parseTelegramUserInput(input)
	if chatID == 0 && userID == 0 {
		return channel.DirectoryEntry{}, fmt.Errorf("telegram resolve entry user: invalid input %q", input)
	}
	if userID != 0 {
		config := tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
				ChatID:             chatID,
				SuperGroupUsername: "",
				UserID:             userID,
			},
		}
		member, err := bot.GetChatMember(config)
		if err != nil {
			return channel.DirectoryEntry{}, fmt.Errorf("telegram get chat member: %w", err)
		}
		if member.User == nil {
			return channel.DirectoryEntry{}, fmt.Errorf("telegram get chat member: empty user")
		}
		return a.telegramUserToEntryWithAvatar(bot, member.User), nil
	}
	chatConfig := tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: chatID}}
	chat, err := bot.GetChat(chatConfig)
	if err != nil {
		return channel.DirectoryEntry{}, fmt.Errorf("telegram get chat: %w", err)
	}
	if !chat.IsPrivate() {
		return channel.DirectoryEntry{}, fmt.Errorf("telegram resolve entry user: chat %d is not private", chatID)
	}
	name := strings.TrimSpace(chat.FirstName + " " + chat.LastName)
	if name == "" {
		name = strings.TrimSpace(chat.Title)
	}
	handle := strings.TrimSpace(chat.UserName)
	if handle != "" && !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	idStr := strconv.FormatInt(chat.ID, 10)
	return channel.DirectoryEntry{
		Kind:      channel.DirectoryEntryUser,
		ID:        idStr,
		Name:      name,
		Handle:    handle,
		AvatarURL: a.resolveUserAvatarURL(bot, chat.ID),
		Metadata: map[string]any{
			"chat_id":  idStr,
			"username": chat.UserName,
		},
	}, nil
}

func (a *TelegramAdapter) resolveTelegramGroup(ctx context.Context, bot *tgbotapi.BotAPI, input string) (channel.DirectoryEntry, error) {
	chatID, superGroupUsername := parseTelegramChatInput(input)
	if chatID == 0 && superGroupUsername == "" {
		return channel.DirectoryEntry{}, fmt.Errorf("telegram resolve entry group: invalid input %q", input)
	}
	config := tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: chatID, SuperGroupUsername: superGroupUsername},
	}
	chat, err := bot.GetChat(config)
	if err != nil {
		return channel.DirectoryEntry{}, fmt.Errorf("telegram get chat: %w", err)
	}
	idStr := strconv.FormatInt(chat.ID, 10)
	name := strings.TrimSpace(chat.Title)
	if name == "" {
		name = strings.TrimSpace(chat.FirstName + " " + chat.LastName)
	}
	handle := strings.TrimSpace(chat.UserName)
	if handle != "" && !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	avatarURL := a.resolveChatPhotoURL(bot, chat.Photo)
	return channel.DirectoryEntry{
		Kind:      channel.DirectoryEntryGroup,
		ID:        idStr,
		Name:      name,
		Handle:    handle,
		AvatarURL: avatarURL,
		Metadata:  map[string]any{"chat_id": idStr, "type": chat.Type},
	}, nil
}

// resolveChatPhotoURL resolves a Telegram ChatPhoto to a direct URL.
func (a *TelegramAdapter) resolveChatPhotoURL(bot *tgbotapi.BotAPI, photo *tgbotapi.ChatPhoto) string {
	if photo == nil {
		return ""
	}
	fileID := photo.BigFileID
	if fileID == "" {
		fileID = photo.SmallFileID
	}
	if fileID == "" {
		return ""
	}
	url, err := a.getFileDirectURL(bot, fileID)
	if err != nil {
		return ""
	}
	return url
}

// parseTelegramChatInput parses input as chat_id (numeric) or @channel_username. Returns (chatID, superGroupUsername).
func parseTelegramChatInput(input string) (chatID int64, superGroupUsername string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, ""
	}
	if strings.HasPrefix(input, "@") {
		return 0, input
	}
	id, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, ""
	}
	return id, ""
}

// parseTelegramUserInput parses input as "chat_id" (private chat) or "chat_id:user_id". Returns (chatID, userID); userID 0 means resolve as private chat.
func parseTelegramUserInput(input string) (chatID, userID int64) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, 0
	}
	if idx := strings.Index(input, ":"); idx >= 0 {
		left := strings.TrimSpace(input[:idx])
		right := strings.TrimSpace(input[idx+1:])
		cid, err1 := strconv.ParseInt(left, 10, 64)
		uid, err2 := strconv.ParseInt(right, 10, 64)
		if err1 == nil && err2 == nil && cid != 0 && uid != 0 {
			return cid, uid
		}
	}
	id, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, 0
	}
	return id, 0
}

func (a *TelegramAdapter) telegramUserToEntryWithAvatar(bot *tgbotapi.BotAPI, u *tgbotapi.User) channel.DirectoryEntry {
	entry := telegramUserToEntry(u)
	if bot != nil && u != nil {
		entry.AvatarURL = a.resolveUserAvatarURL(bot, u.ID)
	}
	return entry
}

func telegramUserToEntry(u *tgbotapi.User) channel.DirectoryEntry {
	if u == nil {
		return channel.DirectoryEntry{Kind: channel.DirectoryEntryUser}
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	handle := strings.TrimSpace(u.UserName)
	if handle != "" && !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	idStr := strconv.FormatInt(u.ID, 10)
	meta := map[string]any{"user_id": idStr}
	if u.UserName != "" {
		meta["username"] = u.UserName
	}
	return channel.DirectoryEntry{
		Kind:     channel.DirectoryEntryUser,
		ID:       idStr,
		Name:     name,
		Handle:   handle,
		Metadata: meta,
	}
}
