package channel

import "strings"

type TargetHint struct {
	Example string `json:"example,omitempty"`
	Label   string `json:"label,omitempty"`
}

type TargetSpec struct {
	Format string       `json:"format"`
	Hints  []TargetHint `json:"hints,omitempty"`
}

func NormalizeTarget(channelType ChannelType, raw string) (string, bool) {
	desc, ok := GetChannelDescriptor(channelType)
	if !ok || desc.NormalizeTarget == nil {
		return strings.TrimSpace(raw), false
	}
	normalized := strings.TrimSpace(desc.NormalizeTarget(raw))
	if normalized == "" {
		return "", false
	}
	return normalized, true
}
