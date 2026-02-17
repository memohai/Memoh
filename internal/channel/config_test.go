package channel_test

import (
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

const testChannelType = channel.Type("test-config")

// testConfigAdapter implements Adapter, ConfigNormalizer, TargetResolver, BindingMatcher for tests.
type testConfigAdapter struct{}

func (a *testConfigAdapter) Type() channel.Type { return testChannelType }
func (a *testConfigAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        testChannelType,
		DisplayName: "Test",
		Capabilities: channel.Capabilities{
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
	}
}

func (a *testConfigAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	value := channel.ReadString(raw, "value")
	if value == "" {
		return nil, errors.New("value is required")
	}
	return map[string]any{"value": value}, nil
}

func (a *testConfigAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	value := channel.ReadString(raw, "user")
	if value == "" {
		return nil, errors.New("user is required")
	}
	return map[string]any{"user": value}, nil
}

func (a *testConfigAdapter) NormalizeTarget(raw string) string { return raw }

func (a *testConfigAdapter) ResolveTarget(raw map[string]any) (string, error) {
	value := channel.ReadString(raw, "target")
	if value == "" {
		return "", errors.New("target is required")
	}
	return "resolved:" + value, nil
}

func (a *testConfigAdapter) MatchBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	value := channel.ReadString(raw, "user")
	return value != "" && value == criteria.SubjectID
}

func (a *testConfigAdapter) BuildUserConfig(_ channel.Identity) map[string]any {
	return map[string]any{}
}

func newTestConfigRegistry() *channel.Registry {
	reg := channel.NewRegistry()
	reg.MustRegister(&testConfigAdapter{})
	return reg
}

func TestParseChannelType(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()

	got, err := reg.ParseChannelType(" test-config ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != testChannelType {
		t.Fatalf("unexpected channel type: %s", got)
	}
	if _, err := reg.ParseChannelType("unknown"); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNormalizeChannelConfig(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()

	got, err := reg.NormalizeConfig(testChannelType, map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["value"] != "ok" {
		t.Fatalf("unexpected value: %#v", got["value"])
	}
}

func TestNormalizeChannelConfigRequiresValue(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()

	_, err := reg.NormalizeConfig(testChannelType, map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNormalizeChannelUserConfig(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()

	got, err := reg.NormalizeUserConfig(testChannelType, map[string]any{"user": "alice"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["user"] != "alice" {
		t.Fatalf("unexpected user: %#v", got["user"])
	}
}

func TestNormalizeChannelUserConfigRequiresUser(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()

	_, err := reg.NormalizeUserConfig(testChannelType, map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
