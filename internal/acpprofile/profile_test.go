package acpprofile

import "testing"

func TestListIncludesClaudeCode(t *testing.T) {
	items := List()
	if len(items) < 2 {
		t.Fatalf("profiles len = %d, want at least 2", len(items))
	}
	profile, ok := Lookup(AgentClaudeCodeID)
	if !ok {
		t.Fatalf("Claude Code profile was not registered")
	}
	if profile.Command != "claude-agent-acp" {
		t.Fatalf("Claude Code command = %q", profile.Command)
	}
	if len(profile.ManagedFields) == 0 || !profile.ManagedFields[0].Required {
		t.Fatalf("Claude Code profile should expose required API key field: %#v", profile.ManagedFields)
	}
	if len(profile.SetupModes) != 3 || profile.SetupModes[0] != setupModeAPIKey || profile.SetupModes[1] != setupModeOAuth || profile.SetupModes[2] != setupModeSelf {
		t.Fatalf("Claude Code setup modes = %#v", profile.SetupModes)
	}
}

func TestMetadataAgentEnabled(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{
			name: "agent config enabled",
			metadata: map[string]any{
				MetadataKeyACP: map[string]any{
					"agents": map[string]any{
						AgentCodexID: map[string]any{"enabled": true},
					},
				},
			},
			want: true,
		},
		{
			name: "agent config disabled",
			metadata: map[string]any{
				MetadataKeyACP: map[string]any{
					"agents": map[string]any{
						AgentCodexID: map[string]any{"enabled": false},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetadataAgentEnabled(tt.metadata, AgentCodexID); got != tt.want {
				t.Fatalf("MetadataAgentEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSensitiveMergeAndScrub(t *testing.T) {
	existing := map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key":  "sk-oldsecret",
						"base_url": "https://example.test",
					},
				},
			},
		},
	}
	incoming := map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key":  "sk-...cret",
						"base_url": "https://new.example",
					},
				},
			},
		},
	}

	merged := MergeSensitiveFieldsForUpdate(existing, incoming)
	setup := ParseAgentSetup(merged, AgentCodexID)
	if got := setup.Managed["api_key"]; got != "sk-oldsecret" {
		t.Fatalf("api_key = %q, want preserved old secret", got)
	}
	if got := setup.Managed["base_url"]; got != "https://new.example" {
		t.Fatalf("base_url = %q, want new value", got)
	}

	scrubbed := ScrubMetadataForResponse(merged)
	setup = ParseAgentSetup(scrubbed, AgentCodexID)
	if got := setup.Managed["api_key"]; got == "sk-oldsecret" || got == "" {
		t.Fatalf("scrubbed api_key = %q, want masked", got)
	}
}

func TestMergeSensitiveFieldsThreeState(t *testing.T) {
	existing := map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key":  "sk-existing",
						"base_url": "https://old.example",
					},
				},
			},
		},
	}

	preserve := MergeSensitiveFieldsForUpdate(existing, map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"base_url": "https://new.example",
					},
				},
			},
		},
	})
	if got := ParseAgentSetup(preserve, AgentCodexID).Managed["api_key"]; got != "sk-existing" {
		t.Fatalf("missing api_key update preserved %q, want existing secret", got)
	}

	cleared := MergeSensitiveFieldsForUpdate(existing, map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key": nil,
					},
				},
			},
		},
	})
	if _, ok := ParseAgentSetup(cleared, AgentCodexID).Managed["api_key"]; ok {
		t.Fatalf("nil api_key update should clear existing secret")
	}

	overwritten := MergeSensitiveFieldsForUpdate(existing, map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key": "sk-new",
					},
				},
			},
		},
	})
	if got := ParseAgentSetup(overwritten, AgentCodexID).Managed["api_key"]; got != "sk-new" {
		t.Fatalf("new api_key update = %q, want overwrite", got)
	}

	dottedSecret := MergeSensitiveFieldsForUpdate(existing, map[string]any{
		MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				AgentCodexID: map[string]any{
					"enabled": true,
					"managed": map[string]any{
						"api_key": "https://acme.example.com/v1/...",
					},
				},
			},
		},
	})
	if got := ParseAgentSetup(dottedSecret, AgentCodexID).Managed["api_key"]; got != "https://acme.example.com/v1/..." {
		t.Fatalf("dotted api_key update = %q, want literal value", got)
	}
}
