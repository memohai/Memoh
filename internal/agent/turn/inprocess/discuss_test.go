package inprocess

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type fakeAgentStreamer struct {
	lastConfig *agentpkg.RunConfig
}

func (f *fakeAgentStreamer) Stream(_ context.Context, cfg agentpkg.RunConfig) <-chan agentpkg.StreamEvent {
	f.lastConfig = &cfg
	ch := make(chan agentpkg.StreamEvent, 1)
	ch <- agentpkg.StreamEvent{
		Type:     agentpkg.EventAgentEnd,
		Messages: json.RawMessage(`[{"role":"assistant","content":"done"}]`),
	}
	close(ch)
	return ch
}

type fakeDiscussResolver struct {
	resolveResult agentpkg.ResolveRunConfigResult
	inlineFn      func(ctx context.Context, botID string, refs []pipeline.ImageAttachmentRef) []sdk.ImagePart
	storeCalls    int
}

func (f *fakeDiscussResolver) ResolveRunConfig(_ context.Context, _, _, _, _, _, _, _ string) (agentpkg.ResolveRunConfigResult, error) {
	return f.resolveResult, nil
}

func (f *fakeDiscussResolver) InlineImageAttachments(ctx context.Context, botID string, refs []pipeline.ImageAttachmentRef) []sdk.ImagePart {
	if f.inlineFn != nil {
		return f.inlineFn(ctx, botID, refs)
	}
	return nil
}

func (f *fakeDiscussResolver) StoreRound(_ context.Context, _, _, _, _ string, _ []sdk.Message, _ string) error {
	f.storeCalls++
	return nil
}

func drainDiscuss(t *testing.T, h turn.RunHandle) []turn.Event {
	t.Helper()
	var events []turn.Event
	for e := range h.Events() {
		events = append(events, e)
	}
	for range h.Errs() {
	}
	return events
}

func discussCommand() turn.StartTurnCommand {
	return turn.StartTurnCommand{
		SchemaVersion: 1,
		TeamID:        "team-1",
		Mode:          turn.ModeDiscuss,
		BotID:         "bot-1",
		SessionID:     "sess-1",
		DiscussMessages: []turn.DiscussMessage{
			{Role: "user", Content: `<message id="1">photo</message>`},
		},
		DiscussMentioned: true,
		DiscussAddressed: true,
	}
}

func TestDiscussInlinesImages(t *testing.T) {
	agent := &fakeAgentStreamer{}
	resolver := &fakeDiscussResolver{
		resolveResult: agentpkg.ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{SupportsImageInput: true},
			ModelID:   "model-1",
		},
		inlineFn: func(_ context.Context, _ string, refs []pipeline.ImageAttachmentRef) []sdk.ImagePart {
			if len(refs) != 1 || refs[0].ContentHash != "img-hash" {
				t.Fatalf("unexpected refs: %v", refs)
			}
			return []sdk.ImagePart{{Image: "data:image/jpeg;base64,FAKE", MediaType: "image/jpeg"}}
		},
	}
	a := New(&fakeRunner{}, WithDiscuss(agent, resolver))
	cmd := discussCommand()
	cmd.DiscussImageRefs = []turn.DiscussImageRef{{ContentHash: "img-hash", Mime: "image/jpeg"}}

	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	drainDiscuss(t, h)

	if agent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}
	var userMsgs []sdk.Message
	for _, m := range agent.lastConfig.Messages {
		if m.Role == sdk.MessageRoleUser {
			userMsgs = append(userMsgs, m)
		}
	}
	if len(userMsgs) < 2 {
		t.Fatalf("expected at least 2 user messages (rc + late binding), got %d", len(userMsgs))
	}
	hasImage := false
	for _, part := range userMsgs[0].Content {
		if imgPart, ok := part.(sdk.ImagePart); ok {
			hasImage = true
			if !strings.HasPrefix(imgPart.Image, "data:image/jpeg;base64,") {
				t.Fatalf("unexpected image data: %q", imgPart.Image)
			}
		}
	}
	if !hasImage {
		t.Fatal("expected image part in RC user message")
	}
	if resolver.storeCalls != 1 {
		t.Fatalf("store calls = %d, want 1 after terminal agent_end", resolver.storeCalls)
	}
}

func TestDiscussNoInlineWhenNoVision(t *testing.T) {
	agent := &fakeAgentStreamer{}
	resolver := &fakeDiscussResolver{
		resolveResult: agentpkg.ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{SupportsImageInput: false},
			ModelID:   "model-1",
		},
		inlineFn: func(_ context.Context, _ string, _ []pipeline.ImageAttachmentRef) []sdk.ImagePart {
			t.Fatal("should not be called when model doesn't support vision")
			return nil
		},
	}
	a := New(&fakeRunner{}, WithDiscuss(agent, resolver))
	cmd := discussCommand()
	cmd.DiscussImageRefs = []turn.DiscussImageRef{{ContentHash: "img-hash", Mime: "image/jpeg"}}

	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	drainDiscuss(t, h)

	if agent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}
	for _, m := range agent.lastConfig.Messages {
		for _, part := range m.Content {
			if _, ok := part.(sdk.ImagePart); ok {
				t.Fatal("should not have image parts when vision is not supported")
			}
		}
	}
}

func TestDiscussACPUsesChatStreamer(t *testing.T) {
	agent := &fakeAgentStreamer{}
	runner := &fakeRunner{chunks: []string{`{"type":"agent_end"}`}}
	resolver := &fakeDiscussResolver{
		resolveResult: agentpkg.ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent},
	}
	a := New(runner, WithDiscuss(agent, resolver))
	cmd := discussCommand()
	cmd.RouteID = "route-1"
	cmd.SourceChannelIdentityID = "acct-1"
	cmd.CurrentChannel = "telegram"
	cmd.ReplyTarget = "chat-1"
	cmd.ConversationType = "group"
	cmd.SessionToken = "Bearer owner-token"
	cmd.ChatToken = "chat-token"
	cmd.ToolHTTPURL = "http://example.test/bots/bot-1/tools"
	cmd.DiscussMessages = []turn.DiscussMessage{{Role: "user", Content: "please inspect the app"}}

	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	events := drainDiscuss(t, h)

	if agent.lastConfig != nil {
		t.Fatal("ordinary agent should not be invoked for ACP discuss runtime")
	}
	req := runner.gotReq
	if req.BotID != "bot-1" || req.SessionID != "sess-1" || req.SourceChannelIdentityID != "acct-1" {
		t.Fatalf("runtime request = %#v", req)
	}
	if req.RouteID != "route-1" || req.ChatToken != "chat-token" || req.Token != "Bearer owner-token" {
		t.Fatalf("runtime context = route %q chat token %q token %q", req.RouteID, req.ChatToken, req.Token)
	}
	if req.ToolHTTPURL != "http://example.test/bots/bot-1/tools" {
		t.Fatalf("ToolHTTPURL = %q", req.ToolHTTPURL)
	}
	if !strings.Contains(req.Query, "please inspect the app") || !strings.Contains(req.Query, "reset each turn") || !strings.Contains(req.Query, "MUST use the `send` tool") {
		t.Fatalf("runtime query = %q, want full discuss context", req.Query)
	}
	// DiscussAddressed=true must render the addressed-directly nudge for ACP.
	if !strings.Contains(req.Query, "addressed directly") {
		t.Fatalf("runtime query missing addressed-directly nudge: %q", req.Query)
	}
	if !req.UserMessagePersisted {
		t.Fatal("runtime request should avoid duplicating the full-context prompt as a user history message")
	}
	if !req.ForceFreshRuntime {
		t.Fatal("discuss ACP runtime request should force a fresh runtime each turn")
	}
	var sawTerminal bool
	for _, e := range events {
		if e.Kind == "agent_end" {
			sawTerminal = true
		}
	}
	if !sawTerminal {
		t.Fatal("expected terminal agent_end event forwarded from the runtime")
	}
}

func TestDiscussACPSkipsWhenNotAddressed(t *testing.T) {
	agent := &fakeAgentStreamer{}
	runner := &fakeRunner{chunks: []string{`{"type":"agent_end"}`}}
	resolver := &fakeDiscussResolver{
		resolveResult: agentpkg.ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent},
	}
	a := New(runner, WithDiscuss(agent, resolver))
	cmd := discussCommand()
	cmd.DiscussAddressed = false

	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	events := drainDiscuss(t, h)

	if runner.gotReq.BotID != "" {
		t.Fatal("runtime must not start for a passive (unaddressed) message")
	}
	var sawSkip bool
	for _, e := range events {
		if e.Kind == turn.DiscussEventSkipped {
			sawSkip = true
		}
	}
	if !sawSkip {
		t.Fatal("expected skip marker event")
	}
}

func TestDiscussRefreshesContextFragAfterLateBinding(t *testing.T) {
	agent := &fakeAgentStreamer{}
	resolver := &fakeDiscussResolver{
		resolveResult: agentpkg.ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{System: "base system"},
			ModelID:   "model-1",
		},
	}
	a := New(&fakeRunner{}, WithDiscuss(agent, resolver))
	cmd := discussCommand()
	cmd.DiscussMessages = []turn.DiscussMessage{{Role: "user", Content: `<message id="x">hello</message>`}}

	h, err := a.StartTurn(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	drainDiscuss(t, h)

	if agent.lastConfig == nil {
		t.Fatal("expected agent to be invoked")
	}
	cfg := agent.lastConfig
	if cfg.ContextManifest.Counts.Messages != len(cfg.Messages) {
		t.Fatalf("manifest message count = %d, messages = %d", cfg.ContextManifest.Counts.Messages, len(cfg.Messages))
	}
	if !lastMessageFragContains(cfg.ContextFrags, "MUST use the `send` tool") {
		t.Fatalf("context frags do not include late-binding prompt: %#v", cfg.ContextManifest.Items)
	}
}

func TestInjectImagePartsIntoLastUserMessage(t *testing.T) {
	msgs := []sdk.Message{
		sdk.UserMessage("hello"),
		sdk.AssistantMessage("hi"),
		sdk.UserMessage("look at this"),
	}
	parts := []sdk.ImagePart{
		{Image: "data:image/png;base64,abc", MediaType: "image/png"},
	}

	injectImagePartsIntoLastUserMessage(msgs, parts)

	lastUser := msgs[2]
	if len(lastUser.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(lastUser.Content))
	}
	imgPart, ok := lastUser.Content[1].(sdk.ImagePart)
	if !ok {
		t.Fatalf("expected ImagePart, got %T", lastUser.Content[1])
	}
	if imgPart.Image != "data:image/png;base64,abc" {
		t.Fatalf("unexpected image: %q", imgPart.Image)
	}
}

func TestInjectImagePartsIntoLastUserMessage_Empty(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	injectImagePartsIntoLastUserMessage(msgs, nil)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func TestInjectImagePartsIntoLastUserMessage_SkipsEmptyImage(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	parts := []sdk.ImagePart{{Image: "", MediaType: "image/png"}}
	injectImagePartsIntoLastUserMessage(msgs, parts)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func lastMessageFragContains(frags []contextfrag.ContextFrag, needle string) bool {
	for i := len(frags) - 1; i >= 0; i-- {
		frag := frags[i]
		if frag.Kind != contextfrag.KindConversationEvent || len(frag.Parts) == 0 || frag.Parts[0].SDKMessage == nil {
			continue
		}
		for _, part := range frag.Parts[0].SDKMessage.Content {
			if text, ok := part.(sdk.TextPart); ok && strings.Contains(text.Text, needle) {
				return true
			}
		}
		return false
	}
	return false
}
