package whatsapp

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow/types"

	"github.com/memohai/memoh/internal/channel"
)

type Config struct {
	StoreID     string
	NeedsRelink bool
}

type UserConfig struct {
	JID   string
	Phone string
}

func normalizeConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if cfg.StoreID != "" {
		out["storeId"] = cfg.StoreID
	}
	if cfg.NeedsRelink {
		out["needsRelink"] = true
	}
	return out, nil
}

func parseConfig(raw map[string]any) (Config, error) {
	storeID := strings.TrimSpace(channel.ReadString(raw, "storeId", "store_id"))
	needsRelink := readBool(raw, "needsRelink", "needs_relink")
	if storeID == "" && !needsRelink {
		return Config{}, errors.New("whatsapp storeId is required; use the QR setup flow")
	}
	if storeID != "" {
		if err := validateStoreID(storeID); err != nil {
			return Config{}, err
		}
	}
	return Config{StoreID: storeID, NeedsRelink: needsRelink}, nil
}

func readBool(raw map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
		default:
			return strings.EqualFold(fmt.Sprintf("%v", v), "true") || fmt.Sprintf("%v", v) == "1"
		}
	}
	return false
}

func normalizeUserConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if cfg.JID != "" {
		out["jid"] = cfg.JID
	}
	if cfg.Phone != "" {
		out["phone"] = cfg.Phone
	}
	return out, nil
}

func parseUserConfig(raw map[string]any) (UserConfig, error) {
	jid := normalizeTarget(channel.ReadString(raw, "jid"))
	phone := normalizePhone(channel.ReadString(raw, "phone"))
	if jid == "" && phone == "" {
		return UserConfig{}, errors.New("whatsapp user config requires jid or phone")
	}
	if jid == "" {
		jid = phone + "@s.whatsapp.net"
	}
	if err := validatePrivateTarget(jid); err != nil {
		return UserConfig{}, err
	}
	return UserConfig{JID: jid, Phone: phone}, nil
}

func resolveTarget(raw map[string]any) (string, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return "", err
	}
	if cfg.JID != "" {
		return cfg.JID, nil
	}
	if cfg.Phone != "" {
		return cfg.Phone + "@s.whatsapp.net", nil
	}
	return "", errors.New("whatsapp binding is incomplete")
}

func matchBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return false
	}
	if value := normalizeTarget(criteria.Attribute("jid")); value != "" && value == cfg.JID {
		return true
	}
	if value := normalizePhone(criteria.Attribute("phone")); value != "" && value == cfg.Phone {
		return true
	}
	if subject := normalizeTarget(criteria.SubjectID); subject != "" && subject == cfg.JID {
		return true
	}
	return false
}

func buildUserConfig(identity channel.Identity) map[string]any {
	out := map[string]any{}
	if jid := normalizeTarget(identity.Attribute("jid")); jid != "" {
		if validatePrivateTarget(jid) == nil {
			out["jid"] = jid
		}
	}
	if phone := normalizePhone(identity.Attribute("phone")); phone != "" {
		out["phone"] = phone
	}
	return out
}

func validatePrivateTarget(target string) error {
	_, err := parsePrivateTarget(target)
	return err
}

func parsePrivateTarget(target string) (types.JID, error) {
	normalized := normalizeTarget(target)
	if normalized == "" {
		return types.JID{}, errors.New("whatsapp target is required")
	}
	jid, err := types.ParseJID(normalized)
	if err != nil {
		return types.JID{}, fmt.Errorf("invalid whatsapp target: %w", err)
	}
	if !isPrivateUserServer(jid.Server) {
		return types.JID{}, fmt.Errorf("whatsapp only supports private chat targets, got %s", jid.Server)
	}
	if strings.TrimSpace(jid.User) == "" {
		return types.JID{}, errors.New("whatsapp private chat target requires a user id")
	}
	if jid.RawAgent != 0 || jid.Device != 0 {
		return types.JID{}, errors.New("whatsapp private chat target must be a bare user jid")
	}
	return jid.ToNonAD(), nil
}

func normalizeTarget(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "wa:")
	value = strings.TrimPrefix(value, "whatsapp:")
	value = strings.TrimPrefix(value, "jid:")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "@") {
		return strings.ToLower(value)
	}
	phone := normalizePhone(value)
	if phone == "" {
		return strings.ToLower(value)
	}
	return phone + "@s.whatsapp.net"
}

func normalizePhone(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "+")
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
