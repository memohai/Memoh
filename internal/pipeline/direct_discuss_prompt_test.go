package pipeline

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
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

type capturingDirectDiscussPromptPreparer struct {
	calls    int
	input    DirectDiscussPromptInput
	prepared PreparedDirectDiscussPrompt
	err      error
}

func (p *capturingDirectDiscussPromptPreparer) PrepareDirectDiscussPrompt(
	_ context.Context,
	input DirectDiscussPromptInput,
) (PreparedDirectDiscussPrompt, error) {
	p.calls++
	p.input = input
	return p.prepared, p.err
}

type countingDirectDiscussReceipt struct {
	calls atomic.Int32
	err   error
}

func (r *countingDirectDiscussReceipt) Finish(context.Context) error {
	r.calls.Add(1)
	return r.err
}
