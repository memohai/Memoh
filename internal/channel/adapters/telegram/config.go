package telegram

import (
	"errors"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

// Config holds the Telegram bot credentials extracted from a channel configuration.
type Config struct {
	BotToken string
}

// UserConfig holds the identifiers used to target a Telegram user or group.
type UserConfig struct {
	Username string
	UserID   string
	ChatID   string
}

func normalizeConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"botToken": cfg.BotToken,
	}, nil
}

func normalizeUserConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if cfg.Username != "" {
		result["username"] = cfg.Username
	}
	if cfg.UserID != "" {
		result["user_id"] = cfg.UserID
	}
	if cfg.ChatID != "" {
		result["chat_id"] = cfg.ChatID
	}
	return result, nil
}

func resolveTarget(raw map[string]any) (string, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return "", err
	}
	if cfg.ChatID != "" {
		return cfg.ChatID, nil
	}
	if cfg.UserID != "" {
		return cfg.UserID, nil
	}
	if cfg.Username != "" {
		name := cfg.Username
		if !strings.HasPrefix(name, "@") {
			name = "@" + name
		}
		return name, nil
	}
	return "", errors.New("telegram binding is incomplete")
}

func matchBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return false
	}
	if value := strings.TrimSpace(criteria.Attribute("chat_id")); value != "" && value == cfg.ChatID {
		return true
	}
	if value := strings.TrimSpace(criteria.Attribute("user_id")); value != "" && value == cfg.UserID {
		return true
	}
	if value := strings.TrimSpace(criteria.Attribute("username")); value != "" && strings.EqualFold(value, cfg.Username) {
		return true
	}
	if criteria.SubjectID != "" {
		if criteria.SubjectID == cfg.ChatID || criteria.SubjectID == cfg.UserID || strings.EqualFold(criteria.SubjectID, cfg.Username) {
			return true
		}
	}
	return false
}

func buildUserConfig(identity channel.Identity) map[string]any {
	result := map[string]any{}
	if value := strings.TrimSpace(identity.Attribute("username")); value != "" {
		result["username"] = value
	}
	if value := strings.TrimSpace(identity.Attribute("user_id")); value != "" {
		result["user_id"] = value
	}
	if value := strings.TrimSpace(identity.Attribute("chat_id")); value != "" {
		result["chat_id"] = value
	}
	return result
}

func parseConfig(raw map[string]any) (Config, error) {
	token := strings.TrimSpace(channel.ReadString(raw, "botToken", "bot_token"))
	if token == "" {
		return Config{}, errors.New("telegram botToken is required")
	}
	return Config{BotToken: token}, nil
}

func parseUserConfig(raw map[string]any) (UserConfig, error) {
	username := strings.TrimSpace(channel.ReadString(raw, "username"))
	userID := strings.TrimSpace(channel.ReadString(raw, "userId", "user_id"))
	chatID := strings.TrimSpace(channel.ReadString(raw, "chatId", "chat_id"))
	if username == "" && userID == "" && chatID == "" {
		return UserConfig{}, errors.New("telegram user config requires username, user_id, or chat_id")
	}
	return UserConfig{
		Username: username,
		UserID:   userID,
		ChatID:   chatID,
	}, nil
}

func normalizeTarget(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "@") {
		return value
	}
	value = strings.TrimPrefix(value, "tg:")
	value = strings.TrimPrefix(value, "telegram:")
	value = strings.TrimPrefix(value, "t.me/")
	value = strings.TrimPrefix(value, "https://t.me/")
	value = strings.TrimPrefix(value, "http://t.me/")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "@") {
		return value
	}
	isNumeric := true
	for _, r := range value {
		if r < '0' || r > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		return value
	}
	return "@" + value
}
