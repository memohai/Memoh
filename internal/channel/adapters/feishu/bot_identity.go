package feishu

import (
	"context"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

type cachedBotOpenID struct {
	region string
	appID  string
	openID string
}

func resolveConfiguredBotOpenID(cfg channel.ChannelConfig) string {
	if value := strings.TrimSpace(channel.ReadString(cfg.SelfIdentity, "open_id", "openId")); value != "" {
		return value
	}
	external := strings.TrimSpace(cfg.ExternalIdentity)
	if external == "" {
		return ""
	}
	if strings.HasPrefix(external, "open_id:") {
		return strings.TrimSpace(strings.TrimPrefix(external, "open_id:"))
	}
	// Legacy records may persist raw open_id without prefix.
	if !strings.Contains(external, ":") {
		return external
	}
	return ""
}

func (a *FeishuAdapter) resolveBotOpenID(ctx context.Context, cfg channel.ChannelConfig) string {
	configID := strings.TrimSpace(cfg.ID)
	if openID := resolveConfiguredBotOpenID(cfg); openID != "" {
		return openID
	}

	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a != nil && a.logger != nil {
			a.logger.Warn("decode config failed during bot identity resolve", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return ""
	}

	if a != nil && configID != "" {
		if raw, ok := a.botOpenIDs.Load(configID); ok {
			if cached, ok := loadCachedBotOpenID(raw, feishuCfg.AppID, feishuCfg.Region); ok {
				return cached
			}
		}
	}
	discovered, externalID, err := a.DiscoverSelf(ctx, cfg.Credentials)
	if err != nil {
		if a != nil && a.logger != nil {
			a.logger.Warn("discover self fallback failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return ""
	}
	if discoveredOpenID := strings.TrimSpace(channel.ReadString(discovered, "open_id", "openId")); discoveredOpenID != "" {
		if a != nil && configID != "" {
			a.botOpenIDs.Store(configID, cachedBotOpenID{
				region: feishuCfg.Region,
				appID:  feishuCfg.AppID,
				openID: discoveredOpenID,
			})
		}
		return discoveredOpenID
	}
	openID := resolveConfiguredBotOpenID(channel.ChannelConfig{ExternalIdentity: externalID})
	if a != nil && configID != "" && openID != "" {
		a.botOpenIDs.Store(configID, cachedBotOpenID{
			region: feishuCfg.Region,
			appID:  feishuCfg.AppID,
			openID: openID,
		})
	}
	return openID
}

func loadCachedBotOpenID(raw any, appID, region string) (string, bool) {
	entry, ok := raw.(cachedBotOpenID)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(entry.region) != strings.TrimSpace(region) {
		return "", false
	}
	if strings.TrimSpace(entry.appID) != strings.TrimSpace(appID) {
		return "", false
	}
	openID := strings.TrimSpace(entry.openID)
	if openID == "" {
		return "", false
	}
	return openID, true
}
