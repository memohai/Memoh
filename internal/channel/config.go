package channel

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NormalizeChannelConfig(channelType ChannelType, raw map[string]any) (map[string]any, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	desc, ok := GetChannelDescriptor(channelType)
	if !ok {
		return nil, fmt.Errorf("unsupported channel type: %s", channelType)
	}
	if desc.NormalizeConfig == nil {
		return raw, nil
	}
	return desc.NormalizeConfig(raw)
}

func NormalizeChannelUserConfig(channelType ChannelType, raw map[string]any) (map[string]any, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	desc, ok := GetChannelDescriptor(channelType)
	if !ok {
		return nil, fmt.Errorf("unsupported channel type: %s", channelType)
	}
	if desc.NormalizeUserConfig == nil {
		return raw, nil
	}
	return desc.NormalizeUserConfig(raw)
}

func ResolveTargetFromUserConfig(channelType ChannelType, config map[string]any) (string, error) {
	desc, ok := GetChannelDescriptor(channelType)
	if !ok || desc.ResolveTarget == nil {
		return "", fmt.Errorf("unsupported channel type: %s", channelType)
	}
	return desc.ResolveTarget(config)
}

func MatchUserBinding(channelType ChannelType, config map[string]any, criteria BindingCriteria) bool {
	desc, ok := GetChannelDescriptor(channelType)
	if !ok || desc.MatchBinding == nil {
		return false
	}
	return desc.MatchBinding(config, criteria)
}

func BuildUserBindingConfig(channelType ChannelType, identity Identity) map[string]any {
	desc, ok := GetChannelDescriptor(channelType)
	if !ok || desc.BuildUserConfig == nil {
		return map[string]any{}
	}
	return desc.BuildUserConfig(identity)
}

func DecodeConfigMap(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func ReadString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			switch v := value.(type) {
			case string:
				return v
			default:
				encoded, err := json.Marshal(v)
				if err == nil {
					return strings.Trim(string(encoded), "\"")
				}
			}
		}
	}
	return ""
}
