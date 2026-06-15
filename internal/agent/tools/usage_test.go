package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

type usageTestResolver struct{}

func (usageTestResolver) ParseChannelType(raw string) (channel.ChannelType, error) {
	return channel.ChannelType(raw), nil
}

type usageTestSender struct{}

func (usageTestSender) Send(context.Context, string, channel.ChannelType, channel.SendRequest) error {
	return nil
}

type usageTestReactor struct{}

func (usageTestReactor) React(context.Context, string, channel.ChannelType, channel.ReactRequest) error {
	return nil
}

func availableToolsForTest(names ...ToolName) AvailableTools {
	available := make(AvailableTools, len(names))
	for _, name := range names {
		available[name.String()] = struct{}{}
	}
	return available
}

func TestMessageProviderUsageGatesRegisteredTools(t *testing.T) {
	t.Parallel()

	provider := NewMessageProvider(nil, usageTestSender{}, usageTestReactor{}, usageTestResolver{}, nil)
	session := SessionContext{SessionType: "chat"}

	if got := provider.Usage(context.Background(), session, nil); got != "" {
		t.Fatalf("Usage without available tools = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), session, availableToolsForTest(ToolSend))
	if !strings.Contains(got, "`send`") {
		t.Fatalf("Usage with send should mention send, got:\n%s", got)
	}
	if strings.Contains(got, "`react`") {
		t.Fatalf("Usage with only send should not mention react, got:\n%s", got)
	}

	got = provider.Usage(context.Background(), session, availableToolsForTest(ToolReact))
	if !strings.Contains(got, "`react`") {
		t.Fatalf("Usage with react should mention react, got:\n%s", got)
	}
	if strings.Contains(got, "`send`") {
		t.Fatalf("Usage with only react should not mention send, got:\n%s", got)
	}
}

func TestMemoryProviderUsageGatesSearchMemory(t *testing.T) {
	t.Parallel()

	provider := NewMemoryProvider(nil, nil, nil)
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without search_memory = %q, want empty", got)
	}
	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolSearchMemory))
	if !strings.Contains(got, "`search_memory`") {
		t.Fatalf("Usage with search_memory should mention it, got:\n%s", got)
	}
}

func TestSkillProviderUsageGatesUseSkill(t *testing.T) {
	t.Parallel()

	provider := NewSkillProvider(nil)
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without use_skill = %q, want empty", got)
	}
	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolUseSkill))
	if !strings.Contains(got, "`use_skill`") {
		t.Fatalf("Usage with use_skill should mention it, got:\n%s", got)
	}
}

func TestHistoryProviderUsageGatesRegisteredTools(t *testing.T) {
	t.Parallel()

	provider := NewHistoryProvider(nil, nil, nil, nil)
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without history tools = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolListSessions))
	if !strings.Contains(got, "`list_sessions`") {
		t.Fatalf("Usage with list_sessions should mention it, got:\n%s", got)
	}
	if strings.Contains(got, "`search_messages`") {
		t.Fatalf("Usage with only list_sessions should not mention search_messages, got:\n%s", got)
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolGetMessages))
	if !strings.Contains(got, "`get_messages`") {
		t.Fatalf("Usage with get_messages should mention it, got:\n%s", got)
	}
	for _, absent := range []string{"`list_sessions`", "`search_messages`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage with only get_messages should not mention %s, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolSearchMessages))
	if !strings.Contains(got, "`search_messages`") {
		t.Fatalf("Usage with search_messages should mention it, got:\n%s", got)
	}
	if strings.Contains(got, "`list_sessions`") {
		t.Fatalf("Usage with only search_messages should not mention list_sessions, got:\n%s", got)
	}
}

func TestContactsProviderUsageGatesGetContacts(t *testing.T) {
	t.Parallel()

	provider := NewContactsProvider(nil, nil)
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without get_contacts = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolGetContacts, ToolSend, ToolSearchMessages))
	for _, want := range []string{"`get_contacts`", "messaging tool", "message-history tools"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Usage with contacts tools should contain %q, got:\n%s", want, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolGetContacts))
	if !strings.Contains(got, "`get_contacts`") {
		t.Fatalf("Usage with get_contacts should mention it, got:\n%s", got)
	}
	for _, absent := range []string{"messaging tool", "message-history tools"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without dependent tools should not contain %q, got:\n%s", absent, got)
		}
	}
}

func TestBrowserProviderUsageGatesRegisteredTools(t *testing.T) {
	t.Parallel()

	provider := NewBrowserProvider(nil, nil, nil, nil, "")
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without browser tools = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolBrowserObserve, ToolBrowserAction))
	if !strings.Contains(got, "`browser_observe`") || !strings.Contains(got, "`browser_action`") {
		t.Fatalf("Usage with browser tools should mention them, got:\n%s", got)
	}
	for _, absent := range []string{"`computer_observe`", "`computer_action`", "`browser_remote_session`", "`read`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without %s should not mention it, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolComputerObserve, ToolComputerAction, ToolBrowserRemoteSession, ToolRead))
	for _, want := range []string{"`computer_observe`", "`computer_action`", "`browser_remote_session`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Usage with %s should mention it, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`read`") || strings.Contains(got, "when you need the image") {
		t.Fatalf("Usage without image input support should not tell the model to read screenshots as images, got:\n%s", got)
	}
	for _, absent := range []string{"`browser_observe`", "`browser_action`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without %s should not mention it, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{SupportsImageInput: true}, availableToolsForTest(ToolComputerObserve, ToolRead))
	if !strings.Contains(got, "`computer_observe`") || !strings.Contains(got, "`read`") || !strings.Contains(got, "when you need the image") {
		t.Fatalf("Usage with observe/read and image input support should mention image read path, got:\n%s", got)
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolBrowserAction))
	if !strings.Contains(got, "`browser_action`") {
		t.Fatalf("Usage with browser_action should mention it, got:\n%s", got)
	}
	for _, absent := range []string{"Observe before acting", "snapshot", "screenshots"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without observe tools should not contain %q, got:\n%s", absent, got)
		}
	}
}

func TestScheduleProviderUsageGatesRegisteredTools(t *testing.T) {
	t.Parallel()

	provider := NewScheduleProvider(nil, nil)
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without schedule tools = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolCreateSchedule, ToolSend))
	if !strings.Contains(got, "`create_schedule`") || !strings.Contains(got, "`send`") {
		t.Fatalf("Usage with create_schedule/send should mention them, got:\n%s", got)
	}
	for _, absent := range []string{"`list_schedule`", "`get_schedule`", "`update_schedule`", "`delete_schedule`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without %s should not mention it, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolCreateSchedule))
	if !strings.Contains(got, "`create_schedule`") {
		t.Fatalf("Usage with create_schedule should mention it, got:\n%s", got)
	}
	if strings.Contains(got, "`send`") || strings.Contains(got, "`speak`") {
		t.Fatalf("Usage without messaging tools should not mention send/speak, got:\n%s", got)
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolCreateSchedule, ToolSpeak))
	if !strings.Contains(got, "`create_schedule`") {
		t.Fatalf("Usage with create_schedule/speak should mention create_schedule, got:\n%s", got)
	}
	if strings.Contains(got, "`send`") || strings.Contains(got, "`speak`") {
		t.Fatalf("Usage with speak should use generic messaging wording instead of naming send/speak, got:\n%s", got)
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolListSchedule, ToolGetSchedule, ToolUpdateSchedule, ToolDeleteSchedule))
	for _, want := range []string{"`list_schedule`", "`get_schedule`", "`update_schedule`", "`delete_schedule`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Usage with %s should mention it, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`create_schedule`") || strings.Contains(got, "`send`") {
		t.Fatalf("Usage without create_schedule/send should not mention them, got:\n%s", got)
	}
}

func TestSpawnProviderUsageGatesRegisteredTools(t *testing.T) {
	t.Parallel()

	provider := &SpawnProvider{}
	if got := provider.Usage(context.Background(), SessionContext{}, nil); got != "" {
		t.Fatalf("Usage without spawn_agent = %q, want empty", got)
	}

	got := provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolSpawnAgent))
	if !strings.Contains(got, "`spawn_agent`") {
		t.Fatalf("Usage with spawn_agent should mention it, got:\n%s", got)
	}
	for _, absent := range []string{"`send_message`", "`wait_agent`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without %s should not mention it, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolSpawnAgent, ToolSendMessage, ToolWaitAgent))
	for _, want := range []string{"`spawn_agent`", "`send_message`", "`wait_agent`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Usage with %s should mention it, got:\n%s", want, got)
		}
	}
	for _, absent := range []string{"`list_agents`", "`list_background`", "`get_background_status`", "`kill_background`", "`search_messages`"} {
		if strings.Contains(got, absent) {
			t.Fatalf("Usage without %s should not mention it, got:\n%s", absent, got)
		}
	}

	got = provider.Usage(context.Background(), SessionContext{}, availableToolsForTest(ToolSpawnAgent, ToolSendMessage, ToolWaitAgent, ToolListAgents, ToolListBackground, ToolGetBackgroundStatus, ToolKillBackground, ToolSearchMessages))
	for _, want := range []string{"`spawn_agent`", "`send_message`", "`wait_agent`", "`list_agents`", "`list_background`", "`get_background_status`", "`kill_background`", "`search_messages`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Usage with %s should mention it, got:\n%s", want, got)
		}
	}
}
