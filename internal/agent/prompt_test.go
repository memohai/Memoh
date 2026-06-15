package agent

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
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
				"Speak in the conversation only through an available messaging capability.",
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

// TestGenerateSystemPromptDoesNotEnumerateConditionalTools enforces the
// Description-first contract: the system prompt must not name any tool that is
// only conditionally registered. Tool usage lives in each sdk.Tool.Description,
// so the prompt can never claim a tool that the current session lacks. This
// guardrail prevents reintroducing prompt-vs-registration drift (the original
// speak / search_memory / schedule bugs).
func TestGenerateSystemPromptDoesNotEnumerateConditionalTools(t *testing.T) {
	t.Parallel()

	// All concrete tool names belong in tool descriptions or ToolUsage gated by
	// the actual registered tool set, never static prompt templates.
	conditionalTools := make([]string, 0, len(agenttools.BuiltInToolNames()))
	for _, name := range agenttools.BuiltInToolNames() {
		conditionalTools = append(conditionalTools, name.String())
	}
	sort.Strings(conditionalTools)

	for _, sessionType := range []string{"chat", "discuss", "schedule", "heartbeat", "subagent"} {
		sessionType := sessionType
		t.Run(sessionType, func(t *testing.T) {
			t.Parallel()
			prompt := GenerateSystemPrompt(SystemPromptParams{
				SessionType:               sessionType,
				Now:                       time.Unix(1, 0).UTC(),
				Timezone:                  "UTC",
				PlatformIdentitiesSection: "## Platform Identities\n\n<identity channel=\"telegram\" username=\"@memoh\"/>",
			})
			for _, name := range conditionalTools {
				if promptEnumeratesTool(prompt, name) {
					t.Fatalf("system prompt for %s must not enumerate conditional tool %q; put its usage in the tool's sdk.Tool.Description instead", sessionType, name)
				}
			}
		})
	}
}

func promptEnumeratesTool(prompt, name string) bool {
	token := regexp.QuoteMeta(name)
	if strings.Contains(prompt, "`"+name+"`") {
		return true
	}
	if regexp.MustCompile(`(^|[^A-Za-z0-9_])` + token + `\s*\(`).MatchString(prompt) {
		return true
	}
	if strings.Contains(name, "_") {
		return regexp.MustCompile(`(^|[^A-Za-z0-9_])` + token + `([^A-Za-z0-9_]|$)`).MatchString(prompt)
	}
	return regexp.MustCompile(`(?i)\b(call|tool|tools|use|using|invoke|invoking|available)\s+` + token + `\b|\b` + token + `\s+(tool|tools|capability|capabilities)\b`).MatchString(prompt)
}

func TestPromptEnumeratesToolDetectsCallSyntax(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		prompt string
		name   string
	}{
		{prompt: "spawn({ tasks: [] })", name: "spawn"},
		{prompt: "send({ text: \"hello\" })", name: "send"},
		{prompt: "read(\"/tmp/image.png\")", name: "read"},
	} {
		if !promptEnumeratesTool(tc.prompt, tc.name) {
			t.Fatalf("expected %q to enumerate %q", tc.prompt, tc.name)
		}
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
