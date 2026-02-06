package feishu

import "testing"

func TestNormalizeConfig(t *testing.T) {
	t.Parallel()

	got, err := NormalizeConfig(map[string]any{
		"app_id":             "app",
		"app_secret":         "secret",
		"encrypt_key":        "enc",
		"verification_token": "verify",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["appId"] != "app" || got["appSecret"] != "secret" {
		t.Fatalf("unexpected feishu config: %#v", got)
	}
	if got["encryptKey"] != "enc" || got["verificationToken"] != "verify" {
		t.Fatalf("unexpected feishu security config: %#v", got)
	}
}

func TestNormalizeConfigRequiresApp(t *testing.T) {
	t.Parallel()

	_, err := NormalizeConfig(map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNormalizeUserConfig(t *testing.T) {
	t.Parallel()

	got, err := NormalizeUserConfig(map[string]any{
		"open_id": "ou_123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got["open_id"] != "ou_123" {
		t.Fatalf("unexpected open_id: %#v", got["open_id"])
	}
}

func TestNormalizeUserConfigRequiresBinding(t *testing.T) {
	t.Parallel()

	_, err := NormalizeUserConfig(map[string]any{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestResolveTarget(t *testing.T) {
	t.Parallel()

	target, err := ResolveTarget(map[string]any{
		"open_id": "ou_123",
		"user_id": "u_123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if target != "open_id:ou_123" {
		t.Fatalf("unexpected target: %s", target)
	}
}

func TestNormalizeTarget(t *testing.T) {
	t.Parallel()

	if got := normalizeTarget("ou_123"); got != "open_id:ou_123" {
		t.Fatalf("unexpected normalized target: %s", got)
	}
	if got := normalizeTarget("chat_id:oc_123"); got != "chat_id:oc_123" {
		t.Fatalf("unexpected normalized target: %s", got)
	}
}
