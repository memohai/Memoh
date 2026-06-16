package personalwechat

import "testing"

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.BridgeExecutable != defaultBridgeExecutable {
		t.Fatalf("BridgeExecutable = %q", cfg.BridgeExecutable)
	}
	if !cfg.AllowPrivate || !cfg.AllowGroups {
		t.Fatalf("defaults should allow private and group chats: %#v", cfg)
	}
	if cfg.MediaDir == "" || cfg.DataDir == "" {
		t.Fatalf("expected data/media dirs: %#v", cfg)
	}
}

func TestNormalizeTarget(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"wxid_abc":                    "contact:wxid_abc",
		"contact:wxid_abc":            "contact:wxid_abc",
		"room:123@chatroom":           "room:123@chatroom",
		"personal_wechat:wxid_abc":    "contact:wxid_abc",
		"wechat_personal:room:r1":     "room:r1",
		" personal_wechat:room:r2  ":  "room:r2",
		" personal_wechat:contact:u ": "contact:u",
	}
	for in, want := range cases {
		if got := normalizeTarget(in); got != want {
			t.Fatalf("normalizeTarget(%q) = %q, want %q", in, got, want)
		}
	}
}
