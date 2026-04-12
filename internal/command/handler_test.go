package command

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/settings"
)

// --- fake services ---

type fakeRoleResolver struct {
	role string
	err  error
}

func (f *fakeRoleResolver) GetMemberRole(_ context.Context, _, _ string) (string, error) {
	return f.role, f.err
}

type fakeScheduleService struct {
	items []schedule.Schedule
}

type fakeCommandQueries struct {
	latestSessionID  pgtype.UUID
	latestSessionErr error
	messageCount     int64
	latestUsage      int64
	latestUsageErr   error
	cacheRow         dbsqlc.GetSessionCacheStatsRow
	cacheErr         error
	skills           []string
}

func (f *fakeCommandQueries) GetLatestSessionIDByBot(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	return f.latestSessionID, f.latestSessionErr
}

func (f *fakeCommandQueries) CountMessagesBySession(_ context.Context, _ pgtype.UUID) (int64, error) {
	return f.messageCount, nil
}

func (f *fakeCommandQueries) GetLatestAssistantUsage(_ context.Context, _ pgtype.UUID) (int64, error) {
	if f.latestUsageErr != nil {
		return 0, f.latestUsageErr
	}
	return f.latestUsage, nil
}

func (f *fakeCommandQueries) GetSessionCacheStats(_ context.Context, _ pgtype.UUID) (dbsqlc.GetSessionCacheStatsRow, error) {
	if f.cacheErr != nil {
		return dbsqlc.GetSessionCacheStatsRow{}, f.cacheErr
	}
	return f.cacheRow, nil
}

func (f *fakeCommandQueries) GetSessionUsedSkills(_ context.Context, _ pgtype.UUID) ([]string, error) {
	return f.skills, nil
}

func (*fakeCommandQueries) GetTokenUsageByDayAndType(_ context.Context, _ dbsqlc.GetTokenUsageByDayAndTypeParams) ([]dbsqlc.GetTokenUsageByDayAndTypeRow, error) {
	return nil, nil
}

func (*fakeCommandQueries) GetTokenUsageByModel(_ context.Context, _ dbsqlc.GetTokenUsageByModelParams) ([]dbsqlc.GetTokenUsageByModelRow, error) {
	return nil, nil
}

// newTestHandler creates a Handler with nil services for use in tests.
func newTestHandler(roleResolver MemberRoleResolver) *Handler {
	return NewHandler(nil, roleResolver, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func newTestHandlerWithQueries(roleResolver MemberRoleResolver, queries CommandQueries) *Handler {
	return NewHandler(nil, roleResolver, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, queries, nil, nil, nil)
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Available commands") {
		t.Errorf("expected help text, got: %s", result)
	}
	if strings.Contains(result, "set-heartbeat") {
		t.Errorf("top-level help should not expand nested actions, got: %s", result)
	}
	if !strings.Contains(result, "- /model - Manage bot models") {
		t.Errorf("expected top-level model entry, got: %s", result)
	}
}

func TestExecute_HelpGroup(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/help model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "/model - Manage bot models") {
		t.Errorf("expected group help, got: %s", result)
	}
	if !strings.Contains(result, "- set - Set the chat model [owner]") {
		t.Errorf("expected compact action summary, got: %s", result)
	}
}

func TestExecute_HelpAction(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/help model set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Usage: /model set <model_id> | <provider_name> <model_name>") {
		t.Errorf("expected action usage, got: %s", result)
	}
	if !strings.Contains(result, "Access: owner only") {
		t.Errorf("expected owner hint, got: %s", result)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/schedule foobar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Unknown action") {
		t.Errorf("expected unknown action message, got: %s", result)
	}
	if !strings.Contains(result, "/schedule") {
		t.Errorf("expected schedule usage in message, got: %s", result)
	}
}

func TestExecute_WritePermissionDenied(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: ""})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/schedule create test desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Permission denied") {
		t.Errorf("expected permission denied, got: %s", result)
	}
}

func TestExecute_WritePermissionAllowedForOwner(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/schedule create")
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
	h := newTestHandler(&fakeRoleResolver{role: ""})
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	tests := []struct {
		cmd      string
		contains string
	}{
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
	if !strings.Contains(result, "- foo | Type: bar") {
		t.Errorf("expected compact line entry, got: %s", result)
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
		"schedule", "mcp", "settings",
		"model", "memory", "search", "browser", "usage",
		"email", "heartbeat", "skill", "fs", "access",
	} {
		if !strings.Contains(help, "/"+group) {
			t.Errorf("missing /%s in global help", group)
		}
	}
}

func TestExecuteWithInput_Access(t *testing.T) {
	t.Parallel()
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
	result, err := h.ExecuteWithInput(context.Background(), ExecuteInput{
		BotID:             "bot-1",
		ChannelIdentityID: "channel-id-1",
		UserID:            "user-id-1",
		Text:              "/access",
		ChannelType:       "discord",
		ConversationType:  "thread",
		ConversationID:    "conv-1",
		ThreadID:          "thread-1",
		RouteID:           "route-1",
		SessionID:         "session-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "- Channel Identity: channel-id-1") {
		t.Errorf("expected channel identity in access output, got: %s", result)
	}
	if !strings.Contains(result, "- Write Commands: yes") {
		t.Errorf("expected write access in access output, got: %s", result)
	}
}

func TestExecute_StatusLatest(t *testing.T) {
	t.Parallel()
	sessionUUID := pgtype.UUID{}
	copy(sessionUUID.Bytes[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	sessionUUID.Valid = true
	h := newTestHandlerWithQueries(&fakeRoleResolver{role: "owner"}, &fakeCommandQueries{
		latestSessionID: sessionUUID,
		messageCount:    42,
		latestUsage:     1200,
		cacheRow: dbsqlc.GetSessionCacheStatsRow{
			CacheReadTokens:  300,
			CacheWriteTokens: 150,
			TotalInputTokens: 1200,
		},
		skills: []string{"search", "browser"},
	})
	result, err := h.Execute(context.Background(), "11111111-1111-1111-1111-111111111111", "user-1", "/status latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "- Scope: latest bot session") {
		t.Errorf("expected latest scope, got: %s", result)
	}
	if !strings.Contains(result, "- Messages: 42") {
		t.Errorf("expected message count, got: %s", result)
	}
}

func TestExecute_StatusLatestNoRows(t *testing.T) {
	t.Parallel()
	h := newTestHandlerWithQueries(&fakeRoleResolver{role: "owner"}, &fakeCommandQueries{
		latestSessionErr: pgx.ErrNoRows,
	})
	result, err := h.Execute(context.Background(), "11111111-1111-1111-1111-111111111111", "user-1", "/status latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No session found for this bot.") {
		t.Errorf("expected no session message, got: %s", result)
	}
}

func TestExecute_StatusShowWithoutSession(t *testing.T) {
	t.Parallel()
	h := newTestHandlerWithQueries(&fakeRoleResolver{role: "owner"}, &fakeCommandQueries{})
	result, err := h.Execute(context.Background(), "bot-1", "user-1", "/status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No active session found for this conversation.") {
		t.Errorf("expected route-aware no session message, got: %s", result)
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
	h := newTestHandler(&fakeRoleResolver{role: "owner"})
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
	_ = fakeScheduleService{items: []schedule.Schedule{{ID: "1", Name: "test"}}}
	_ = mcp.Connection{}
	_ = settings.Settings{}
)
