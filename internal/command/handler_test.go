package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/subagent"
)

// --- fake services ---

type fakeRoleResolver struct {
	role string
	err  error
}

func (f *fakeRoleResolver) GetMemberRole(_ context.Context, _, _ string) (string, error) {
	return f.role, f.err
}

type fakeSubagentService struct {
	items []subagent.Subagent
}

type fakeScheduleService struct {
	items []schedule.Schedule
}

type fakeInboxService struct {
	count inbox.CountResult
}

// newTestHandler creates a Handler with nil services for use in tests.
func newTestHandler(roleResolver MemberRoleResolver) *Handler {
	return NewHandler(nil, roleResolver, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

// --- tests ---

func TestIsCommand(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil)
	tests := []struct {
		input string
		want  bool
	}{
		{"/help", true},
		{"/subagent list", true},
		{" /schedule list", true},
		{"@BotName /help", true},
		{"@_user_1 /schedule list", true},
		{"<@123456> /mcp list", true},
		{"/help@MemohBot", true},
		{"hello", false},
		{"", false},
		{"/", false},
		{"/ ", false},
		{"/unknown_cmd", false},
		{"check https://example.com/help", false},
		{"@bot hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := h.IsCommand(tt.input); got != tt.want {
				t.Errorf("IsCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExecute_Help(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Available commands") {
		t.Errorf("expected help text, got: %s", result)
	}
	if !strings.Contains(result, "/subagent") {
		t.Errorf("expected /subagent in help, got: %s", result)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/foobar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Unknown command") {
		t.Errorf("expected unknown command message, got: %s", result)
	}
}

func TestExecute_WithMentionPrefix(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "@BotName /help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Available commands") {
		t.Errorf("expected help text from mention-prefixed command, got: %s", result)
	}
}

func TestExecute_TelegramBotSuffix(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/help@MemohBot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Available commands") {
		t.Errorf("expected help text from telegram-style command, got: %s", result)
	}
}

func TestExecute_UnknownAction(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/subagent foobar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Unknown action") {
		t.Errorf("expected unknown action message, got: %s", result)
	}
	if !strings.Contains(result, "/subagent") {
		t.Errorf("expected subagent usage in message, got: %s", result)
	}
}

func TestExecute_WritePermissionDenied(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleMember})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/subagent create test desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Permission denied") {
		t.Errorf("expected permission denied, got: %s", result)
	}
}

func TestExecute_WritePermissionAllowedForOwner(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/subagent create")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "Permission denied") {
		t.Errorf("owner should not get permission denied, got: %s", result)
	}
	if !strings.Contains(result, "Usage:") {
		t.Errorf("expected usage hint for missing args, got: %s", result)
	}
}

func TestExecute_SettingsDefaultAction(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleMember})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/settings")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "Unknown action") {
		t.Errorf("expected settings get attempt, not unknown action, got: %s", result)
	}
}

func TestExecute_MissingArgs(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	tests := []struct {
		cmd      string
		contains string
	}{
		{"/subagent get", "Usage:"},
		{"/subagent create", "Usage:"},
		{"/subagent delete", "Usage:"},
		{"/schedule get", "Usage:"},
		{"/schedule create", "Usage:"},
		{"/schedule delete", "Usage:"},
		{"/mcp get", "Usage:"},
		{"/mcp delete", "Usage:"},
		{"/fs read", "not available"},
		{"/model set", "Usage:"},
		{"/model set-heartbeat", "Usage:"},
		{"/memory set", "Usage:"},
		{"/search set", "Usage:"},
		{"/browser set", "Usage:"},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			t.Parallel()
			result, err := h.Execute(context.Background(), "bot-1", "user-1", tt.cmd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in result, got: %s", tt.contains, result)
			}
		})
	}
}

func TestFormatItems(t *testing.T) {
	t.Parallel()
	result := formatItems([][]kv{
		{{"Name", "foo"}, {"Type", "bar"}},
		{{"Name", "longname"}, {"Type", "x"}},
	})
	if !strings.Contains(result, "- foo") {
		t.Errorf("expected '- foo' bullet, got: %s", result)
	}
	if !strings.Contains(result, "  Type: bar") {
		t.Errorf("expected indented 'Type: bar', got: %s", result)
	}
	if !strings.Contains(result, "- longname") {
		t.Errorf("expected '- longname' bullet, got: %s", result)
	}
}

func TestFormatItems_Empty(t *testing.T) {
	t.Parallel()
	result := formatItems(nil)
	if result != "" {
		t.Errorf("expected empty string for nil items, got: %q", result)
	}
}

func TestFormatKV(t *testing.T) {
	t.Parallel()
	result := formatKV([]kv{
		{"Name", "test"},
		{"ID", "123"},
	})
	if !strings.Contains(result, "- Name: test") {
		t.Errorf("expected '- Name: test', got: %s", result)
	}
	if !strings.Contains(result, "- ID: 123") {
		t.Errorf("expected '- ID: 123', got: %s", result)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("hello world", 5); got != "he..." {
		t.Errorf("truncate: got %q", got)
	}
	if got := truncate("hi", 5); got != "hi" {
		t.Errorf("truncate short: got %q", got)
	}
}

// Verify that the global help includes all resource groups.
func TestGlobalHelp_AllGroups(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil)
	help := h.registry.GlobalHelp()
	for _, group := range []string{
		"subagent", "schedule", "mcp", "inbox", "settings",
		"model", "memory", "search", "browser", "usage",
		"email", "heartbeat", "skill", "fs",
	} {
		if !strings.Contains(help, "/"+group) {
			t.Errorf("missing /%s in global help", group)
		}
	}
}

// Verify write commands are tagged with [owner] in usage.
func TestUsage_OwnerTag(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil)
	for _, name := range h.registry.order {
		group := h.registry.groups[name]
		usage := group.Usage()
		for _, subName := range group.order {
			sub := group.commands[subName]
			if sub.IsWrite && !strings.Contains(usage, "[owner]") {
				t.Errorf("/%s %s is a write command but usage missing [owner] tag", name, subName)
			}
		}
	}
}

// Verify new commands with nil services return graceful errors, not panics.
func TestNewCommands_NilServices(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: bots.MemberRoleOwner})
	cmds := []string{
		"/skill list",
		"/fs list",
		"/fs read /test.txt",
	}
	for _, cmd := range cmds {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			result, err := h.Execute(context.Background(), "bot-1", "user-1", cmd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == "" {
				t.Error("expected non-empty result")
			}
		})
	}
}

// suppress unused warnings.
var (
	_ = fakeSubagentService{items: []subagent.Subagent{{ID: "1", Name: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()}}}
	_ = fakeScheduleService{items: []schedule.Schedule{{ID: "1", Name: "test"}}}
	_ = fakeInboxService{count: inbox.CountResult{Unread: 1, Total: 2}}
	_ = mcp.Connection{}
	_ = settings.Settings{}
)
