package inbound

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/acpfeedback"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/i18n"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

// TestResolveNewSessionType_BareConfirmFlag guards the hand-typed "/new
// --confirm" edge: extractFlags doesn't recognize --confirm, so it lands as the
// first positional (the mode slot). It must NOT be read as a session type —
// resolveNewSessionType should fall through to context defaults exactly like a
// bare "/new", not error with `unknown session type "--confirm"`.
func TestResolveNewSessionType_BareConfirmFlag(t *testing.T) {
	msg := channel.InboundMessage{Channel: channel.ChannelTypeTelegram}

	bare, errBare := resolveNewSessionType("/new", msg)
	if errBare != nil {
		t.Fatalf("/new returned error: %v", errBare)
	}
	withFlag, err := resolveNewSessionType("/new --confirm", msg)
	if err != nil {
		t.Fatalf("/new --confirm should not error, got: %v", err)
	}
	if withFlag != bare {
		t.Errorf("/new --confirm resolved to %q, want same as bare /new (%q)", withFlag, bare)
	}
	// Explicit modes must still resolve normally.
	if got, err := resolveNewSessionType("/new chat", msg); err != nil || got != sessionpkg.TypeChat {
		t.Errorf("/new chat = (%q, %v), want (%q, nil)", got, err, sessionpkg.TypeChat)
	}
	if got, err := resolveNewSessionType("/new discuss", msg); err != nil || got != sessionpkg.TypeDiscuss {
		t.Errorf("/new discuss = (%q, %v), want (%q, nil)", got, err, sessionpkg.TypeDiscuss)
	}
	// A genuinely unknown mode still errors.
	if _, err := resolveNewSessionType("/new bogus", msg); err == nil {
		t.Errorf("/new bogus should error on unknown session type")
	}
}

func TestResolveNewSessionSpec_ACPAgent(t *testing.T) {
	dm := channel.InboundMessage{Channel: channel.ChannelTypeTelegram, Conversation: channel.Conversation{Type: "private"}}
	group := channel.InboundMessage{Channel: channel.ChannelTypeTelegram, Conversation: channel.Conversation{Type: "group"}}

	cases := []struct {
		name        string
		cmd         string
		msg         channel.InboundMessage
		wantMode    string
		wantRuntime string
		wantType    string
		wantAgent   string
	}{
		{"bare agent in dm", "/new codex", dm, sessionpkg.TypeChat, sessionpkg.RuntimeACPAgent, sessionpkg.TypeACPAgent, "codex"},
		{"chat agent in dm", "/new chat codex", dm, sessionpkg.TypeChat, sessionpkg.RuntimeACPAgent, sessionpkg.TypeACPAgent, "codex"},
		{"discuss agent", "/new discuss codex", group, sessionpkg.TypeDiscuss, sessionpkg.RuntimeACPAgent, sessionpkg.TypeDiscuss, "codex"},
		{"bare agent in group inherits discuss", "/new codex", group, sessionpkg.TypeDiscuss, sessionpkg.RuntimeACPAgent, sessionpkg.TypeDiscuss, "codex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := resolveNewSessionSpec(tc.cmd, tc.msg)
			if err != nil {
				t.Fatalf("resolveNewSessionSpec(%q) error = %v", tc.cmd, err)
			}
			if spec.Mode != tc.wantMode || spec.Runtime != tc.wantRuntime || spec.Type != tc.wantType {
				t.Fatalf("spec = %#v, want mode/runtime/type %q/%q/%q", spec, tc.wantMode, tc.wantRuntime, tc.wantType)
			}
			if got := newSessionMetadataString(spec.Metadata, "acp_agent_id"); got != tc.wantAgent {
				t.Fatalf("agent = %q, want %q", got, tc.wantAgent)
			}
			if got := newSessionMetadataString(spec.Metadata, "project_path"); got != sessionpkg.DefaultACPProjectPath {
				t.Fatalf("project_path = %q, want default", got)
			}
		})
	}
}

func TestResolveNewSessionSpec_GroupChatACPUnsupported(t *testing.T) {
	group := channel.InboundMessage{Channel: channel.ChannelTypeTelegram, Conversation: channel.Conversation{Type: "group"}}
	_, err := resolveNewSessionSpec("/new chat codex", group)
	if err == nil {
		t.Fatal("resolveNewSessionSpec error = nil, want group chat ACP unsupported")
	}
	var feedback *acpfeedback.Error
	if !errors.As(err, &feedback) || feedback.Code != acpfeedback.CodeGroupChatUnsupported {
		t.Fatalf("feedback = %#v, want code %s", feedback, acpfeedback.CodeGroupChatUnsupported)
	}
}

func TestCurrentContextForNewSessionSpecUsesACPDisplayName(t *testing.T) {
	cc := currentContextForNewSessionSpec(command.CurrentContext{ChatModel: "gpt-4.1"}, NewSessionSpec{
		Runtime: sessionpkg.RuntimeACPAgent,
		Metadata: map[string]any{
			"acp_agent_id": acpprofile.AgentCodexID,
		},
	})
	if cc.ChatModel != acpprofile.AgentCodexName+" / ACP" {
		t.Fatalf("ChatModel = %q, want ACP display label", cc.ChatModel)
	}
}

func TestHandleNewSessionCommandCreatesACPChatSpec(t *testing.T) {
	ownerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	channelIdentityID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "22222222-2222-2222-2222-222222222222", Type: sessionpkg.TypeACPAgent}}
	p := &ChannelInboundProcessor{
		routeResolver:     chatSvc,
		sessionEnsurer:    ensurer,
		permissionChecker: &fakeBotPermissionChecker{allowed: true},
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new chat codex"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: channelIdentityID,
		UserID:            ownerID,
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	spec := ensurer.lastSpec
	if spec.Mode != sessionpkg.TypeChat || spec.Runtime != sessionpkg.RuntimeACPAgent || spec.Type != sessionpkg.TypeACPAgent {
		t.Fatalf("spec = %#v, want chat/acp_agent/acp_agent", spec)
	}
	if spec.RuntimeOwnerAccountID != ownerID {
		t.Fatalf("runtime owner = %q, want authenticated channel identity", spec.RuntimeOwnerAccountID)
	}
	if spec.CreatedByUserID != ownerID {
		t.Fatalf("created_by_user_id = %q, want authenticated channel identity", spec.CreatedByUserID)
	}
	if got := newSessionMetadataString(spec.Metadata, "acp_agent_id"); got != "codex" {
		t.Fatalf("agent = %q, want codex", got)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent replies = %d, want 1", len(sender.sent))
	}
}

func TestHandleNewSessionCommandCreatesNativeSessionWithCreator(t *testing.T) {
	creatorID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{}
	p := &ChannelInboundProcessor{
		routeResolver:  chatSvc,
		sessionEnsurer: ensurer,
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new chat"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		UserID:            creatorID,
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	spec := ensurer.lastSpec
	if spec.Mode != sessionpkg.TypeChat || spec.Runtime != sessionpkg.RuntimeModel || spec.Type != sessionpkg.TypeChat {
		t.Fatalf("spec = %#v, want native chat session", spec)
	}
	if spec.CreatedByUserID != creatorID {
		t.Fatalf("created_by_user_id = %q, want authenticated channel identity", spec.CreatedByUserID)
	}
	if spec.RuntimeOwnerAccountID != "" {
		t.Fatalf("runtime owner = %q, want empty for native session", spec.RuntimeOwnerAccountID)
	}
}

func TestHandleNewSessionCommandCancelsActiveStream(t *testing.T) {
	routeID := "11111111-1111-1111-1111-111111111111"
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: routeID}}
	ensurer := &fakeSessionEnsurer{}
	p := &ChannelInboundProcessor{
		routeResolver:  chatSvc,
		sessionEnsurer: ensurer,
	}
	cancelled := make(chan struct{})
	if _, accepted := p.activeStreams.Register("bot-1:"+routeID, "old-session", context.CancelFunc(func() { close(cancelled) })); !accepted {
		t.Fatal("active stream registration was rejected")
	}

	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new --confirm"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID: "bot-1",
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("/new did not cancel the active stream for the route")
	}
	if got := p.activeStreams.Count("bot-1:" + routeID); got != 0 {
		t.Fatal("active stream remained registered after /new")
	}
}

func TestHandleNewSessionCommandBareNewInheritsDefaultACP(t *testing.T) {
	ownerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "22222222-2222-2222-2222-222222222222", Type: sessionpkg.TypeACPAgent}}
	p := &ChannelInboundProcessor{
		routeResolver:     chatSvc,
		sessionEnsurer:    ensurer,
		permissionChecker: &fakeBotPermissionChecker{allowed: true},
		defaultChatRuntime: fakeDefaultChatRuntimeReader{settings: DefaultChatRuntimeSettings{
			Runtime:     sessionpkg.RuntimeACPAgent,
			ACPAgentID:  "codex",
			ProjectPath: "/workspace",
			ProjectMode: sessionpkg.DefaultACPProjectMode,
		}},
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		UserID:            ownerID,
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	spec := ensurer.lastSpec
	if spec.Mode != sessionpkg.TypeChat || spec.Runtime != sessionpkg.RuntimeACPAgent || spec.Type != sessionpkg.TypeACPAgent {
		t.Fatalf("spec = %#v, want default chat ACP", spec)
	}
	if got := newSessionMetadataString(spec.Metadata, "acp_agent_id"); got != "codex" {
		t.Fatalf("agent = %q, want codex", got)
	}
	if got := newSessionMetadataString(spec.Metadata, "project_path"); got != "/workspace" {
		t.Fatalf("project_path = %q, want /workspace", got)
	}
}

func TestHandleNewSessionCommandExplicitACPInheritsDefaultProject(t *testing.T) {
	ownerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "22222222-2222-2222-2222-222222222222", Type: sessionpkg.TypeACPAgent}}
	p := &ChannelInboundProcessor{
		routeResolver:     chatSvc,
		sessionEnsurer:    ensurer,
		permissionChecker: &fakeBotPermissionChecker{allowed: true},
		defaultChatRuntime: fakeDefaultChatRuntimeReader{settings: DefaultChatRuntimeSettings{
			Runtime:     sessionpkg.RuntimeACPAgent,
			ACPAgentID:  "codex",
			ProjectPath: "/workspace/default",
			ProjectMode: sessionpkg.DefaultACPProjectMode,
		}},
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new codex"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		UserID:            ownerID,
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	spec := ensurer.lastSpec
	if got := newSessionMetadataString(spec.Metadata, "project_path"); got != "/workspace/default" {
		t.Fatalf("project_path = %q, want bot default", got)
	}
}

func TestHandleNewSessionCommandPreflightsACPSetup(t *testing.T) {
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "22222222-2222-2222-2222-222222222222", Type: sessionpkg.TypeACPAgent}}
	p := &ChannelInboundProcessor{
		routeResolver:     chatSvc,
		sessionEnsurer:    ensurer,
		permissionChecker: &fakeBotPermissionChecker{allowed: true},
		acpAgentSetup: fakeACPAgentSetupReader{metadata: map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentClaudeCodeID: map[string]any{
						"enabled":    true,
						"setup_mode": "api_key",
						"managed":    map[string]any{},
					},
				},
			},
		}},
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new claude-code"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		UserID:            "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	if ensurer.lastSpec.Runtime != "" {
		t.Fatalf("session should not be created with incomplete setup, got spec %#v", ensurer.lastSpec)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Message.PlainText(), "setup is incomplete") {
		t.Fatalf("expected setup feedback, got %+v", sender.sent)
	}
}

func TestSendACPFeedbackErrorUsesI18nKey(t *testing.T) {
	p := &ChannelInboundProcessor{}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1"},
		ReplyTarget: "target-1",
	}

	err := p.sendACPFeedbackError(context.Background(), sender, msg, InboundIdentity{BotID: "bot-1"}, acpfeedback.New(
		acpfeedback.CodeNoWorkspaceExec,
		"missing_workspace_exec",
		403,
		"chat.acp.noWorkspaceExec",
		"raw backend message",
		nil,
	))
	if err != nil {
		t.Fatalf("sendACPFeedbackError() error = %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent replies = %d, want 1", len(sender.sent))
	}
	got := sender.sent[0].Message.PlainText()
	if got == "raw backend message" || !strings.Contains(got, "workspace commands") {
		t.Fatalf("feedback text = %q, want localized ACP feedback", got)
	}
}

func TestHandleNewSessionCommandACPRequiresWorkspaceExec(t *testing.T) {
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "11111111-1111-1111-1111-111111111111"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "22222222-2222-2222-2222-222222222222", Type: sessionpkg.TypeACPAgent}}
	p := &ChannelInboundProcessor{
		routeResolver:     chatSvc,
		sessionEnsurer:    ensurer,
		permissionChecker: &fakeBotPermissionChecker{allowed: false},
	}
	sender := &fakeReplySender{}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "msg-1", Text: "/new chat codex"},
		ReplyTarget: "target-1",
		Conversation: channel.Conversation{
			ID:   "dm-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := p.handleNewSessionCommand(context.Background(), channel.ChannelConfig{}, msg, sender, InboundIdentity{
		BotID:             "bot-1",
		ChannelIdentityID: "user-no-exec",
		UserID:            "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("handleNewSessionCommand() error = %v", err)
	}
	if ensurer.lastSpec.Runtime != "" {
		t.Fatalf("session should not be created without workspace_exec, got spec %#v", ensurer.lastSpec)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Message.PlainText(), "permission to run workspace commands") {
		t.Fatalf("expected workspace_exec feedback, got %+v", sender.sent)
	}
}

// TestSendNewConfirmation_LocalizesActionLabels guards the
// newSession.action.{confirm,cancel} key rename. /new on a button-capable
// channel posts a Confirm/Cancel gate; the labels must render in the user's
// command_ui_language with the correct callback data carrying through.
func TestSendNewConfirmation_LocalizesActionLabels(t *testing.T) {
	p := &ChannelInboundProcessor{}
	cases := []struct {
		locale      string
		wantConfirm string
		wantCancel  string
	}{
		{"en", "✅ Confirm", "✕ Cancel"},
		{"zh", "✅ 确认", "✕ 取消"},
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			s := &fakeReplySender{}
			err := p.sendNewConfirmation(
				context.Background(),
				channel.InboundMessage{ReplyTarget: "test-target"},
				s,
				i18n.New(tc.locale),
				"chat",
				i18n.New(tc.locale).T("newSession.modeChat"),
				channel.ChannelCapabilities{Buttons: true, Markdown: true, Text: true},
			)
			if err != nil {
				t.Fatalf("sendNewConfirmation: %v", err)
			}
			if len(s.sent) != 1 {
				t.Fatalf("expected 1 sent message, got %d", len(s.sent))
			}
			out := s.sent[0].Message
			if len(out.Actions) != 2 {
				t.Fatalf("expected 2 actions (confirm + cancel), got %d", len(out.Actions))
			}
			var confirm, cancel channel.Action
			for _, a := range out.Actions {
				if a.Value == command.EncodeConfirmNewCallback("chat") {
					confirm = a
				} else if a.Value == command.DismissCallback() {
					cancel = a
				}
			}
			if confirm.Label != tc.wantConfirm {
				t.Errorf("[%s] confirm label = %q, want %q", tc.locale, confirm.Label, tc.wantConfirm)
			}
			if cancel.Label != tc.wantCancel {
				t.Errorf("[%s] cancel label = %q, want %q", tc.locale, cancel.Label, tc.wantCancel)
			}
			// Body must contain the bold confirm title (markup intact on the
			// Markdown-capable channel used in this test).
			if !strings.Contains(out.Text, "Confirm") && !strings.Contains(out.Text, "确认") {
				t.Errorf("[%s] confirmation body missing confirm token, got %q", tc.locale, out.Text)
			}
		})
	}
}

func TestSendNewConfirmationShowsACPRuntimeLabel(t *testing.T) {
	p := &ChannelInboundProcessor{}
	s := &fakeReplySender{}
	loc := i18n.New("en")
	spec := NewSessionSpec{
		Mode:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeACPAgent,
		Metadata: map[string]any{
			"acp_agent_id": acpprofile.AgentCodexID,
		},
	}

	err := p.sendNewConfirmation(
		context.Background(),
		channel.InboundMessage{ReplyTarget: "test-target"},
		s,
		loc,
		newSessionConfirmModeText(spec),
		newSessionDisplayModeLabel(loc, spec),
		channel.ChannelCapabilities{Buttons: true, Markdown: true, Text: true},
	)
	if err != nil {
		t.Fatalf("sendNewConfirmation: %v", err)
	}
	if len(s.sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(s.sent))
	}
	out := s.sent[0].Message
	if !strings.Contains(out.Text, "chat with "+acpprofile.AgentCodexName+" / ACP") {
		t.Fatalf("confirmation text = %q, want ACP runtime label", out.Text)
	}
	if len(out.Actions) != 2 || out.Actions[0].Value != command.EncodeConfirmNewCallback("chat codex") {
		t.Fatalf("actions = %#v, want callback to preserve /new chat codex", out.Actions)
	}
}
