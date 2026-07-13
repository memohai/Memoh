package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestBuildDirectDiscussPromptInputClassifiesSemanticSources(t *testing.T) {
	t.Parallel()

	scope := contextfrag.Scope{BotID: "bot", SessionID: "session"}
	artifacts := []CompactionArtifact{{
		ID:      "artifact-a",
		Summary: "durable summary",
		Sources: []CompactionSource{{Ref: contextfrag.ContextRef{
			Namespace: "bot_history_message",
			ID:        "covered-row",
			Schema:    contextfrag.SchemaContextRef,
		}}},
	}}
	messages := []ContextMessage{
		{Role: "user", Content: "old rendered", RenderedMessageIDs: []string{"old-a", "old-b"}},
		{Role: "user", Content: "<summary>durable summary</summary>", CompactionArtifactID: "artifact-a"},
		{Role: "assistant", Content: "turn response", SourceMessageID: "history-row"},
		{
			Role:               "user",
			Content:            "current rendered",
			RenderedMessageIDs: []string{"current"},
			ImageRefs:          []ImageAttachmentRef{{ContentHash: "image-hash", Mime: "image/png"}},
			Current:            true,
		},
	}

	input := buildDirectDiscussPromptInput(messages, artifacts, scope, "late binding", "account-user")
	if input.ActorUserID != "account-user" || input.CurrentSourceID != "rendered:current" {
		t.Fatalf("prompt input identity = %#v", input)
	}
	if len(input.Sources) != 5 {
		t.Fatalf("prompt sources = %d, want 5: %#v", len(input.Sources), input.Sources)
	}
	wantIDs := []string{
		"rendered:old-a|old-b",
		"compaction:artifact-a",
		"history:history-row",
		"rendered:current",
		"discuss:late-binding",
	}
	for index, wantID := range wantIDs {
		if got := input.Sources[index].ID; got != wantID {
			t.Fatalf("source %d id = %q, want %q", index, got, wantID)
		}
	}
	if input.Sources[0].Required || !input.Sources[0].Compactable {
		t.Fatalf("old rendered source policy = %#v", input.Sources[0])
	}
	summary := input.Sources[1]
	if !summary.Required || summary.Compactable || summary.SummaryFrag == nil ||
		summary.SummaryFrag.Ref.ID != "artifact-a" || len(summary.SummaryFrag.Coverage.CoveredRefs) != 1 {
		t.Fatalf("summary source policy/provenance = %#v", summary)
	}
	if input.Sources[2].Required || !input.Sources[2].Compactable {
		t.Fatalf("turn response source policy = %#v", input.Sources[2])
	}
	current := input.Sources[3]
	if !current.Required || !current.Compactable || len(current.ImageRefs) != 1 || current.ImageRefs[0].ContentHash != "image-hash" {
		t.Fatalf("current source policy/native refs = %#v", current)
	}
	late := input.Sources[4]
	if !late.Required || late.Compactable || strings.TrimSpace(sdkMessageText([]sdk.Message{late.Message})) != "late binding" {
		t.Fatalf("late-binding source = %#v", late)
	}
}

func TestBuildDirectDiscussPromptInputUsesStableFallbackIdentity(t *testing.T) {
	t.Parallel()

	input := buildDirectDiscussPromptInput(
		[]ContextMessage{{Role: "user", Content: "unidentified", Current: true}},
		nil,
		contextfrag.Scope{},
		"late",
		"",
	)
	if got := input.Sources[0].ID; got != "discuss-source:000" || input.CurrentSourceID != got {
		t.Fatalf("fallback source identity = source:%q current:%q", got, input.CurrentSourceID)
	}
}

func TestDiscussACPFullContextPromptRendersNativeToolMarkers(t *testing.T) {
	t.Parallel()

	prompt := discussACPFullContextPrompt([]sdk.Message{
		sdk.UserMessage("question"),
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{
				sdk.ReasoningPart{Text: "private reasoning"},
				sdk.ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", Input: map[string]any{"q": "memoh"}},
			},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "call-1", ToolName: "lookup", Result: map[string]any{"answer": 42}}),
		sdk.UserMessage("FINAL LATE BINDING"),
	})

	for _, want := range []string{"[user]", "question", "[tool_call: lookup]", `"q":"memoh"`, "[tool_result: lookup]", `"answer":42`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("ACP prompt missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "private reasoning") {
		t.Fatalf("ACP prompt leaked reasoning: %s", prompt)
	}
	if !strings.HasSuffix(prompt, "FINAL LATE BINDING") {
		t.Fatalf("late binding is not the final ACP instruction: %s", prompt)
	}
}

func TestHandleReplyWithAgentConsumesPreparedPromptAndReceipt(t *testing.T) {
	t.Parallel()

	receipt := &countingDirectDiscussReceipt{}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("prepared prompt")}},
		Receipt:   receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RunConfig:                   agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("legacy bypass")}},
		DirectDiscussPromptPreparer: preparer,
	}}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config:          DiscussSessionConfig{BotID: "bot", SessionID: "session", UserID: "account-user"},
		lastProcessedMs: 50,
	}
	rc := RenderedContext{{
		MessageID:    "current-image",
		ReceivedAtMs: 100,
		ImageRefs:    []ImageAttachmentRef{{ContentHash: "image-hash", Mime: "image/png"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if preparer.calls != 1 || preparer.input.ActorUserID != "account-user" || preparer.input.CurrentSourceID != "rendered:current-image" {
		t.Fatalf("preparer input = calls:%d input:%#v", preparer.calls, preparer.input)
	}
	if len(preparer.input.Sources) != 2 || len(preparer.input.Sources[0].ImageRefs) != 1 || !preparer.input.Sources[0].Required {
		t.Fatalf("semantic sources = %#v", preparer.input.Sources)
	}
	if agent.lastConfig == nil || !strings.Contains(sdkMessageText(agent.lastConfig.Messages), "prepared prompt") || strings.Contains(sdkMessageText(agent.lastConfig.Messages), "legacy bypass") {
		t.Fatalf("agent config = %#v", agent.lastConfig)
	}
	if receipt.calls.Load() != 1 {
		t.Fatalf("receipt finish calls = %d, want 1", receipt.calls.Load())
	}
	if resolver.compactionCalls != 0 {
		t.Fatalf("driver bypassed receipt with %d legacy compaction calls", resolver.compactionCalls)
	}
}

func TestHandleReplyWithAgentFinishesReceiptOnceAcrossExitPaths(t *testing.T) {
	t.Parallel()

	terminalMessages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("done")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	tests := []struct {
		name       string
		events     []agentpkg.StreamEvent
		storeErr   error
		wantCursor int64
		wantStore  int
	}{
		{
			name:       "no terminal",
			events:     []agentpkg.StreamEvent{{Type: agentpkg.EventError, Error: "provider failed"}},
			wantCursor: 50,
		},
		{
			name:       "terminal abort",
			events:     []agentpkg.StreamEvent{{Type: agentpkg.EventAgentAbort}},
			wantCursor: 100,
		},
		{
			name:       "store failure",
			events:     []agentpkg.StreamEvent{{Type: agentpkg.EventAgentEnd, Messages: terminalMessages}},
			storeErr:   errors.New("store failed"),
			wantCursor: 100,
			wantStore:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			receipt := &countingDirectDiscussReceipt{}
			preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
				RunConfig: agentpkg.RunConfig{},
				Receipt:   receipt,
			}}
			resolver := &fakeRunConfigResolver{
				resolveResult: ResolveRunConfigResult{DirectDiscussPromptPreparer: preparer},
				storeErr:      test.storeErr,
			}
			driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
			sess := &discussSession{
				config:          DiscussSessionConfig{BotID: "bot", SessionID: "session"},
				lastProcessedMs: 50,
			}
			rc := RenderedContext{renderedText("current", 100, "current")}

			driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{events: test.events})

			if got := receipt.calls.Load(); got != 1 {
				t.Fatalf("receipt finish calls = %d, want 1", got)
			}
			if sess.lastProcessedMs != test.wantCursor {
				t.Fatalf("cursor = %d, want %d", sess.lastProcessedMs, test.wantCursor)
			}
			if resolver.storeCalls != test.wantStore {
				t.Fatalf("store calls = %d, want %d", resolver.storeCalls, test.wantStore)
			}
		})
	}
}

func TestHandleReplyWithAgentStopsWhenPromptPreparationFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preparer *capturingDirectDiscussPromptPreparer
	}{
		{
			name:     "prepare error",
			preparer: &capturingDirectDiscussPromptPreparer{err: errors.New("prepare failed")},
		},
		{
			name: "missing receipt",
			preparer: &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
				RunConfig: agentpkg.RunConfig{},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{DirectDiscussPromptPreparer: test.preparer}}
			agent := &fakeDiscussStreamer{}
			driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
			sess := &discussSession{
				config:          DiscussSessionConfig{BotID: "bot", SessionID: "session"},
				lastProcessedMs: 50,
			}

			driver.handleReplyWithAgent(
				context.Background(),
				sess,
				RenderedContext{renderedText("current", 100, "current")},
				driver.logger,
				agent,
			)

			if agent.lastConfig != nil {
				t.Fatalf("agent received config after %s: %#v", test.name, agent.lastConfig)
			}
			if sess.lastProcessedMs != 50 {
				t.Fatalf("cursor advanced after %s to %d", test.name, sess.lastProcessedMs)
			}
		})
	}
}

func TestHandleReplyWithAgentACPConsumesPreparedPromptAndReceipt(t *testing.T) {
	t.Parallel()

	receipt := &countingDirectDiscussReceipt{}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{
			sdk.UserMessage("prepared summary"),
			sdk.UserMessage("prepared current"),
			sdk.UserMessage("prepared late binding"),
		}},
		Receipt: receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RuntimeType:                 sessionpkg.RuntimeACPAgent,
		DirectDiscussPromptPreparer: preparer,
	}}
	runtime := &fakeDiscussRuntimeStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
		},
		lastProcessedMs: 50,
	}
	rc := RenderedContext{{
		MessageID:    "current",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "raw bypass content"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})

	if preparer.calls != 1 || runtime.calls != 1 {
		t.Fatalf("ACP preparation/runtime = prepare:%d runtime:%d", preparer.calls, runtime.calls)
	}
	if !strings.Contains(runtime.lastReq.Query, "prepared current") || strings.Contains(runtime.lastReq.Query, "raw bypass content") {
		t.Fatalf("ACP query bypassed prepared prompt: %q", runtime.lastReq.Query)
	}
	if receipt.calls.Load() != 1 {
		t.Fatalf("ACP receipt finish calls = %d, want 1", receipt.calls.Load())
	}
	if resolver.compactionCalls != 0 {
		t.Fatalf("ACP driver bypassed receipt with %d legacy compaction calls", resolver.compactionCalls)
	}
	if sess.lastProcessedMs != 100 {
		t.Fatalf("ACP terminal cursor = %d, want 100", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgentACPMaterializesPreparedPrompt(t *testing.T) {
	t.Parallel()

	var materializeCalls atomic.Int32
	receipt := &countingDirectDiscussReceipt{}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{
			Messages: []sdk.Message{sdk.UserMessage("baseline must not escape")},
			InitialPromptMaterializer: func(_ context.Context, cfg agentpkg.RunConfig, tools []sdk.Tool) (agentpkg.RunConfig, error) {
				materializeCalls.Add(1)
				if len(tools) != 0 {
					t.Fatalf("ACP materializer tools = %d, want none", len(tools))
				}
				cfg.Messages = []sdk.Message{sdk.UserMessage("materialized ACP context")}
				return cfg, nil
			},
		},
		Receipt: receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RuntimeType:                 sessionpkg.RuntimeACPAgent,
		DirectDiscussPromptPreparer: preparer,
	}}
	runtime := &fakeDiscussRuntimeStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
		},
		lastProcessedMs: 50,
	}
	rc := RenderedContext{{
		MessageID:    "current",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})

	if materializeCalls.Load() != 1 {
		t.Fatalf("materializer calls = %d, want 1", materializeCalls.Load())
	}
	if runtime.calls != 1 || !strings.Contains(runtime.lastReq.Query, "materialized ACP context") || strings.Contains(runtime.lastReq.Query, "baseline must not escape") {
		t.Fatalf("ACP runtime = calls:%d query:%q", runtime.calls, runtime.lastReq.Query)
	}
	if receipt.calls.Load() != 1 {
		t.Fatalf("receipt calls = %d, want 1", receipt.calls.Load())
	}
}

func TestHandleReplyWithAgentACPFinishesFailedMaterialization(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("materialize failed")
	var materializeCalls atomic.Int32
	receipt := &countingDirectDiscussReceipt{err: sentinel}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{
			InitialPromptMaterializer: func(context.Context, agentpkg.RunConfig, []sdk.Tool) (agentpkg.RunConfig, error) {
				materializeCalls.Add(1)
				return agentpkg.RunConfig{}, sentinel
			},
		},
		Receipt: receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RuntimeType:                 sessionpkg.RuntimeACPAgent,
		DirectDiscussPromptPreparer: preparer,
	}}
	runtime := &fakeDiscussRuntimeStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
		},
		lastProcessedMs: 50,
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{{
		MessageID:    "current",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
	}}, driver.logger, &fakeDiscussStreamer{})

	if materializeCalls.Load() != 1 || receipt.calls.Load() != 1 {
		t.Fatalf("failed materialization = materialize:%d receipt:%d, want 1/1", materializeCalls.Load(), receipt.calls.Load())
	}
	if runtime.calls != 0 || sess.lastProcessedMs != 50 {
		t.Fatalf("failed materialization advanced runtime/cursor = calls:%d cursor:%d", runtime.calls, sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgentACPDoesNotConsumeReceiptWithoutRuntimeAttempt(t *testing.T) {
	t.Parallel()

	receipt := &countingDirectDiscussReceipt{}
	preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
		RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("prepared")}},
		Receipt:   receipt,
	}}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
		RuntimeType:                 sessionpkg.RuntimeACPAgent,
		DirectDiscussPromptPreparer: preparer,
	}}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
		},
		lastProcessedMs: 50,
	}

	driver.handleReplyWithAgent(context.Background(), sess, RenderedContext{{
		MessageID:    "current",
		ReceivedAtMs: 100,
		MentionsMe:   true,
		Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
	}}, driver.logger, &fakeDiscussStreamer{})

	if preparer.calls != 1 {
		t.Fatalf("ACP prepare calls = %d, want 1", preparer.calls)
	}
	if receipt.calls.Load() != 0 {
		t.Fatalf("receipt consumed without runtime attempt %d times", receipt.calls.Load())
	}
	if sess.lastProcessedMs != 50 {
		t.Fatalf("cursor advanced without runtime attempt to %d", sess.lastProcessedMs)
	}
}

func TestHandleReplyWithAgentACPFinishesReceiptAcrossAttemptExits(t *testing.T) {
	t.Parallel()

	abortEvent, err := json.Marshal(agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
	if err != nil {
		t.Fatalf("marshal abort event: %v", err)
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name       string
		ctx        context.Context
		runtime    discussRuntimeStreamer
		wantCursor int64
	}{
		{
			name:       "closed without terminal",
			ctx:        context.Background(),
			runtime:    &scriptedDiscussRuntimeStreamer{},
			wantCursor: 50,
		},
		{
			name:       "terminal abort",
			ctx:        context.Background(),
			runtime:    &scriptedDiscussRuntimeStreamer{chunks: []conversation.StreamChunk{abortEvent}},
			wantCursor: 100,
		},
		{
			name:       "cancelled",
			ctx:        cancelledCtx,
			runtime:    &blockingDiscussRuntimeStreamer{},
			wantCursor: 50,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			receipt := &countingDirectDiscussReceipt{}
			preparer := &capturingDirectDiscussPromptPreparer{prepared: PreparedDirectDiscussPrompt{
				RunConfig: agentpkg.RunConfig{Messages: []sdk.Message{sdk.UserMessage("prepared late binding")}},
				Receipt:   receipt,
			}}
			resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
				RuntimeType:                 sessionpkg.RuntimeACPAgent,
				DirectDiscussPromptPreparer: preparer,
			}}
			driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: test.runtime})
			sess := &discussSession{
				config: DiscussSessionConfig{
					BotID:            "bot",
					SessionID:        "session",
					ConversationType: channel.ConversationTypeGroup,
				},
				lastProcessedMs: 50,
			}

			driver.handleReplyWithAgent(test.ctx, sess, RenderedContext{{
				MessageID:    "current",
				ReceivedAtMs: 100,
				MentionsMe:   true,
				Content:      []RenderedContentPiece{{Type: "text", Text: "current"}},
			}}, driver.logger, &fakeDiscussStreamer{})

			if receipt.calls.Load() != 1 {
				t.Fatalf("receipt finish calls = %d, want 1", receipt.calls.Load())
			}
			if sess.lastProcessedMs != test.wantCursor {
				t.Fatalf("cursor = %d, want %d", sess.lastProcessedMs, test.wantCursor)
			}
		})
	}
}

type capturingDirectDiscussPromptPreparer struct {
	calls    int
	input    DirectDiscussPromptInput
	rebuild  bool
	prepared PreparedDirectDiscussPrompt
	err      error
}

func (p *capturingDirectDiscussPromptPreparer) PrepareDirectDiscussPrompt(
	ctx context.Context,
	recipe DirectDiscussPromptRecipe,
) (PreparedDirectDiscussPrompt, error) {
	p.calls++
	p.input = recipe.Initial
	if p.rebuild {
		var err error
		p.input, err = recipe.Rebuild(ctx)
		if err != nil {
			return PreparedDirectDiscussPrompt{}, err
		}
	}
	return p.prepared, p.err
}

type countingDirectDiscussReceipt struct {
	calls atomic.Int32
	err   error
}

type scriptedDiscussRuntimeStreamer struct {
	chunks []conversation.StreamChunk
}

func (s *scriptedDiscussRuntimeStreamer) StreamChat(context.Context, conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	chunks := make(chan conversation.StreamChunk, len(s.chunks))
	errs := make(chan error)
	for _, chunk := range s.chunks {
		chunks <- chunk
	}
	close(chunks)
	close(errs)
	return chunks, errs
}

type blockingDiscussRuntimeStreamer struct{}

func (*blockingDiscussRuntimeStreamer) StreamChat(context.Context, conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	return make(chan conversation.StreamChunk), make(chan error)
}

func (r *countingDirectDiscussReceipt) Finish(context.Context) error {
	r.calls.Add(1)
	return r.err
}
