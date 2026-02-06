package channel_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

const testChannelType = channel.ChannelType("test-config")

var registerTestChannelOnce sync.Once

func registerTestChannel() {
	registerTestChannelOnce.Do(func() {
		if _, ok := channel.GetChannelDescriptor(testChannelType); ok {
			return
		}
		_ = channel.RegisterChannel(channel.ChannelDescriptor{
			Type:                testChannelType,
			DisplayName:         "Test",
			NormalizeConfig:     normalizeTestConfig,
			NormalizeUserConfig: normalizeTestUserConfig,
			ResolveTarget:       resolveTestTarget,
			MatchBinding:        matchTestBinding,
			Capabilities: channel.ChannelCapabilities{
				Text: true,
			},
			ConfigSchema: channel.ConfigSchema{
				Version: 1,
				Fields: map[string]channel.FieldSchema{
					"value": {Type: channel.FieldString, Required: true},
				},
			},
			UserConfigSchema: channel.ConfigSchema{
				Version: 1,
				Fields: map[string]channel.FieldSchema{
					"user": {Type: channel.FieldString, Required: true},
				},
			},
		})
	})
}

func normalizeTestConfig(raw map[string]any) (map[string]any, error) {
	value := channel.ReadString(raw, "value")
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}
	return map[string]any{"value": value}, nil
}

func normalizeTestUserConfig(raw map[string]any) (map[string]any, error) {
	value := channel.ReadString(raw, "user")
	if value == "" {
		return nil, fmt.Errorf("user is required")
	}
	return map[string]any{"user": value}, nil
}

func resolveTestTarget(raw map[string]any) (string, error) {
	value := channel.ReadString(raw, "target")
	if value == "" {
		return "", fmt.Errorf("target is required")
	}
	return "resolved:" + value, nil
}

func matchTestBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	value := channel.ReadString(raw, "user")
	return value != "" && value == criteria.ExternalID
}

func TestParseChannelType(t *testing.T) {
	t.Parallel()
	registerTestChannel()

	got, err := channel.ParseChannelType(" test-config ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != testChannelType {
		t.Fatalf("unexpected channel type: %s", got)
	}
	if _, err := channel.ParseChannelType("unknown"); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNormalizeChannelConfig(t *testing.T) {
	t.Parallel()
	registerTestChannel()

	got, err := channel.NormalizeChannelConfig(testChannelType, map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["value"] != "ok" {
		t.Fatalf("unexpected value: %#v", got["value"])
	}
}

func TestNormalizeChannelConfigRequiresValue(t *testing.T) {
	t.Parallel()
	registerTestChannel()

	_, err := channel.NormalizeChannelConfig(testChannelType, map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNormalizeChannelUserConfig(t *testing.T) {
	t.Parallel()
	registerTestChannel()

	got, err := channel.NormalizeChannelUserConfig(testChannelType, map[string]any{"user": "alice"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["user"] != "alice" {
		t.Fatalf("unexpected user: %#v", got["user"])
	}
}

func TestNormalizeChannelUserConfigRequiresUser(t *testing.T) {
	t.Parallel()
	registerTestChannel()

	_, err := channel.NormalizeChannelUserConfig(testChannelType, map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
