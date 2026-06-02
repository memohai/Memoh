package agent

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSystemPromptIncludesPlatformIdentitiesInChat(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType:               "chat",
		Now:                       time.Unix(1, 0).UTC(),
		Timezone:                  "UTC",
		PlatformIdentitiesSection: "## Platform Identities\n\n<identity channel=\"telegram\" username=\"@memoh\"/>",
	})

	if !strings.Contains(prompt, "## Platform Identities") {
		t.Fatalf("expected platform identities heading in prompt")
	}
	if !strings.Contains(prompt, `<identity channel="telegram" username="@memoh"/>`) {
		t.Fatalf("expected platform identity XML in prompt")
	}
}

func TestGenerateSystemPromptIncludesCommonAndModeContracts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sessionType string
		want        []string
	}{
		{
			sessionType: "chat",
			want: []string{
				"You are an AI agent running inside a private Memoh workspace.",
				"## Session mode: chat",
				"Your text output is sent directly to the current conversation.",
			},
		},
		{
			sessionType: "discuss",
			want: []string{
				"You are an AI agent running inside a private Memoh workspace.",
				"## Session mode: discuss",
				"Use `send` to speak in the conversation.",
			},
		},
		{
			sessionType: "schedule",
			want: []string{
				"You are an AI agent running inside a private Memoh workspace.",
				"## Session mode: schedule",
				"Your normal text output is logged only.",
			},
		},
		{
			sessionType: "heartbeat",
			want: []string{
				"You are an AI agent running inside a private Memoh workspace.",
				"## Session mode: heartbeat",
				"If nothing needs attention, output exactly `HEARTBEAT_OK`.",
			},
		},
		{
			sessionType: "subagent",
			want: []string{
				"You are an AI agent running inside a private Memoh workspace.",
				"## Session mode: subagent",
				"You are a task-focused worker spawned by a parent agent.",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.sessionType, func(t *testing.T) {
			t.Parallel()
			prompt := GenerateSystemPrompt(SystemPromptParams{
				SessionType: tc.sessionType,
				Now:         time.Unix(1, 0).UTC(),
				Timezone:    "UTC",
			})
			for _, want := range tc.want {
				if !strings.Contains(prompt, want) {
					t.Fatalf("expected prompt for %s to contain %q", tc.sessionType, want)
				}
			}
		})
	}
}

func TestGenerateSystemPromptIncludesServiceOwnedBotInfo(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType: "chat",
		Bot: BotInfo{
			ID:          "bot-1",
			Name:        "research-bot",
			DisplayName: "Research Bot",
			Timezone:    "Asia/Shanghai",
		},
		Now:      time.Unix(1, 0).UTC(),
		Timezone: "UTC",
	})

	for _, want := range []string{
		"## Bot",
		"Service-provided bot identity.",
		"Use `display_name` as your user-facing name when it is present; otherwise use `name`.",
		"Do not invent another name.",
		`"id": "bot-1"`,
		`"name": "research-bot"`,
		`"display_name": "Research Bot"`,
		`"timezone": "Asia/Shanghai"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q", want)
		}
	}
}

func TestGenerateSystemPromptOmitsLegacyCoreFiles(t *testing.T) {
	t.Parallel()

	for _, sessionType := range []string{"chat", "discuss", "schedule", "heartbeat", "subagent"} {
		sessionType := sessionType
		t.Run(sessionType, func(t *testing.T) {
			t.Parallel()
			prompt := GenerateSystemPrompt(SystemPromptParams{
				SessionType: sessionType,
				Now:         time.Unix(1, 0).UTC(),
				Timezone:    "UTC",
			})
			for _, legacy := range []string{"IDENTITY.md", "SOUL.md", "TOOLS.md"} {
				if strings.Contains(prompt, legacy) {
					t.Fatalf("expected prompt for %s to omit legacy file %s", sessionType, legacy)
				}
			}
		})
	}
}

func TestGenerateSystemPromptIncludesDisplayToolsWhenEnabled(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType:    "chat",
		Now:            time.Unix(1, 0).UTC(),
		Timezone:       "UTC",
		DisplayEnabled: true,
	})

	if !strings.Contains(prompt, "## Workspace browser & desktop") {
		t.Fatalf("expected display tools section in prompt")
	}
	if !strings.Contains(prompt, "browser_observe") {
		t.Fatalf("expected browser tool mention in prompt")
	}
}

func TestGenerateSystemPromptOmitsDisplayToolsWhenDisabled(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType: "chat",
		Now:         time.Unix(1, 0).UTC(),
		Timezone:    "UTC",
	})

	if strings.Contains(prompt, "## Workspace browser & desktop") {
		t.Fatalf("expected display tools section to be omitted")
	}
}

func TestGenerateSystemPromptIncludesPlatformIdentitiesInDiscuss(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType:               "discuss",
		Now:                       time.Unix(1, 0).UTC(),
		Timezone:                  "UTC",
		PlatformIdentitiesSection: "## Platform Identities\n\n<identity channel=\"discord\" username=\"@memoh\"/>",
	})

	if !strings.Contains(prompt, "## Platform Identities") {
		t.Fatalf("expected platform identities heading in discuss prompt")
	}
	if !strings.Contains(prompt, `<identity channel="discord" username="@memoh"/>`) {
		t.Fatalf("expected platform identity XML in discuss prompt")
	}
}
