package personalwechat

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

const (
	defaultBridgeExecutable = "node"
	defaultBridgeScript     = "packages/personal-wechat-bridge/bin/personal-wechat-bridge.mjs"
	defaultSessionName      = "MemohPersonalWeChat"
	defaultDataDir          = ".data/personal-wechat"
	defaultMediaDir         = "media"
)

type adapterConfig struct {
	BridgeExecutable     string
	BridgeScript         string
	BridgeArgs           string
	DataDir              string
	MediaDir             string
	SessionName          string
	BotMentionName       string
	AllowPrivate         bool
	AllowGroups          bool
	ContactWhitelist     []string
	GroupWhitelist       []string
	DiagnosticRawPayload bool
}

type userConfig struct {
	Target string
	UserID string
	RoomID string
}

func normalizeConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"bridgeExecutable": cfg.BridgeExecutable,
		"bridgeScript":     cfg.BridgeScript,
		"dataDir":          cfg.DataDir,
		"mediaDir":         cfg.MediaDir,
		"sessionName":      cfg.SessionName,
		"allowPrivate":     cfg.AllowPrivate,
		"allowGroups":      cfg.AllowGroups,
	}
	if cfg.BridgeArgs != "" {
		out["bridgeArgs"] = cfg.BridgeArgs
	}
	if cfg.BotMentionName != "" {
		out["botMentionName"] = cfg.BotMentionName
	}
	if len(cfg.ContactWhitelist) > 0 {
		out["contactWhitelist"] = strings.Join(cfg.ContactWhitelist, ",")
	}
	if len(cfg.GroupWhitelist) > 0 {
		out["groupWhitelist"] = strings.Join(cfg.GroupWhitelist, ",")
	}
	if cfg.DiagnosticRawPayload {
		out["diagnosticRawPayload"] = true
	}
	return out, nil
}

func parseConfig(raw map[string]any) (adapterConfig, error) {
	cfg := adapterConfig{
		BridgeExecutable: strings.TrimSpace(channel.ReadString(raw, "bridgeExecutable", "bridge_executable")),
		BridgeScript:     strings.TrimSpace(channel.ReadString(raw, "bridgeScript", "bridge_script")),
		BridgeArgs:       strings.TrimSpace(channel.ReadString(raw, "bridgeArgs", "bridge_args")),
		DataDir:          strings.TrimSpace(channel.ReadString(raw, "dataDir", "data_dir")),
		MediaDir:         strings.TrimSpace(channel.ReadString(raw, "mediaDir", "media_dir")),
		SessionName:      strings.TrimSpace(channel.ReadString(raw, "sessionName", "session_name")),
		BotMentionName:   strings.TrimSpace(channel.ReadString(raw, "botMentionName", "bot_mention_name")),
		AllowPrivate:     true,
		AllowGroups:      true,
	}
	if cfg.BridgeExecutable == "" {
		cfg.BridgeExecutable = defaultBridgeExecutable
	}
	if cfg.BridgeScript == "" {
		cfg.BridgeScript = defaultBridgeScript
	}
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	if cfg.MediaDir == "" {
		cfg.MediaDir = filepath.Join(cfg.DataDir, defaultMediaDir)
	}
	if cfg.SessionName == "" {
		cfg.SessionName = defaultSessionName
	}
	if v, ok := readBool(raw, "allowPrivate", "allow_private"); ok {
		cfg.AllowPrivate = v
	}
	if v, ok := readBool(raw, "allowGroups", "allow_groups"); ok {
		cfg.AllowGroups = v
	}
	if v, ok := readBool(raw, "diagnosticRawPayload", "diagnostic_raw_payload"); ok {
		cfg.DiagnosticRawPayload = v
	}
	cfg.ContactWhitelist = splitCSV(channel.ReadString(raw, "contactWhitelist", "contact_whitelist"))
	cfg.GroupWhitelist = splitCSV(channel.ReadString(raw, "groupWhitelist", "group_whitelist"))
	if cfg.BridgeExecutable == "" {
		return adapterConfig{}, errors.New("personal_wechat bridgeExecutable is required")
	}
	if cfg.BridgeScript == "" {
		return adapterConfig{}, errors.New("personal_wechat bridgeScript is required")
	}
	return cfg, nil
}

func normalizeUserConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"target": cfg.Target}
	if cfg.UserID != "" {
		out["user_id"] = cfg.UserID
	}
	if cfg.RoomID != "" {
		out["room_id"] = cfg.RoomID
	}
	return out, nil
}

func parseUserConfig(raw map[string]any) (userConfig, error) {
	target := normalizeTarget(channel.ReadString(raw, "target"))
	userID := strings.TrimSpace(channel.ReadString(raw, "userId", "user_id"))
	roomID := strings.TrimSpace(channel.ReadString(raw, "roomId", "room_id"))
	if target == "" {
		switch {
		case roomID != "":
			target = "room:" + roomID
		case userID != "":
			target = "contact:" + userID
		}
	}
	if target == "" {
		return userConfig{}, errors.New("personal_wechat user config requires target, user_id, or room_id")
	}
	return userConfig{Target: target, UserID: userID, RoomID: roomID}, nil
}

func normalizeTarget(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "personal_wechat:")
	value = strings.TrimPrefix(value, "wechat_personal:")
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "room:") || strings.HasPrefix(value, "contact:") {
		return value
	}
	return "contact:" + value
}

func resolveTarget(raw map[string]any) (string, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return "", err
	}
	return cfg.Target, nil
}

func matchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	cfg, err := parseUserConfig(config)
	if err != nil {
		return false
	}
	if criteria.SubjectID != "" {
		if cfg.UserID == criteria.SubjectID || cfg.RoomID == criteria.SubjectID || strings.TrimPrefix(cfg.Target, "contact:") == criteria.SubjectID {
			return true
		}
	}
	if v := criteria.Attribute("user_id"); v != "" && (v == cfg.UserID || cfg.Target == "contact:"+v) {
		return true
	}
	if v := criteria.Attribute("room_id"); v != "" && (v == cfg.RoomID || cfg.Target == "room:"+v) {
		return true
	}
	if v := criteria.Attribute("target"); v != "" && normalizeTarget(v) == cfg.Target {
		return true
	}
	return false
}

func buildUserConfig(identity channel.Identity) map[string]any {
	out := map[string]any{}
	if target := strings.TrimSpace(identity.Attribute("target")); target != "" {
		out["target"] = normalizeTarget(target)
	}
	if userID := strings.TrimSpace(identity.Attribute("user_id")); userID != "" {
		out["user_id"] = userID
		if _, ok := out["target"]; !ok {
			out["target"] = "contact:" + userID
		}
	}
	if roomID := strings.TrimSpace(identity.Attribute("room_id")); roomID != "" {
		out["room_id"] = roomID
		if _, ok := out["target"]; !ok {
			out["target"] = "room:" + roomID
		}
	}
	if _, ok := out["target"]; !ok && strings.TrimSpace(identity.SubjectID) != "" {
		out["target"] = "contact:" + strings.TrimSpace(identity.SubjectID)
	}
	return out
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func readBool(raw map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v, true
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(v))
			if err == nil {
				return parsed, true
			}
		}
	}
	return false, false
}
