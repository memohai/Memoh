package flow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestDirectDiscussPromptPreparerOwnsFinalEnvelopeAndReceipt(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		assetLoader: &fakeGatewayAssetLoader{openFn: func(_ context.Context, _, contentHash string) (io.ReadCloser, string, error) {
			if contentHash != "current-image" {
				t.Fatalf("image content hash = %q", contentHash)
			}
			return io.NopCloser(strings.NewReader("image")), "image/png", nil
		}},
	}
	old := sdk.UserMessage(strings.Repeat("old", 400))
	summary := sdk.UserMessage("<summary>durable summary</summary>")
	current := sdk.UserMessage("current input")
	lateBinding := sdk.UserMessage("late binding")
	image := resolver.InlineImageAttachments(context.Background(), "bot", []pipelinepkg.ImageAttachmentRef{{ContentHash: "current-image"}})[0]
	tools := []sdk.Tool{{Name: "send", Description: "send a reply", Parameters: map[string]any{"type": "object"}}}
	base := agent.RunConfig{
		System:             "system prompt",
		SupportsImageInput: true,
		Identity:           agent.SessionContext{BotID: "bot", SessionID: "session"},
	}
	fixedTokens, err := providerFixedPromptTokens(base, tools)
	if err != nil {
		t.Fatalf("providerFixedPromptTokens() error = %v", err)
	}
	currentWithImage := current
	currentWithImage.Content = append(currentWithImage.Content, image)
	notice := historyTruncationNotice().TextContent()
	contextBudget := fixedTokens +
		estimatePromptMessages([]sdk.Message{summary, currentWithImage, lateBinding, sdk.SystemMessage(notice)})

	var compactCalls atomic.Int32
	var compactPressure int
	var compactRequest conversation.ChatRequest
	var compactContext resolvedContext
	preparer := &directDiscussPromptPreparer{
		resolver:           resolver,
		runConfig:          base,
		contextTokenBudget: contextBudget,
		compact: func(_ context.Context, req conversation.ChatRequest, rc resolvedContext, pressure int) {
			compactCalls.Add(1)
			compactRequest = req
			compactContext = rc
			compactPressure = pressure
		},
	}
	summaryFrag := contextfrag.ContextFrag{
		Ref:      contextfrag.ContextRef{ID: "artifact-a"},
		Kind:     contextfrag.KindConversationSummary,
		Coverage: &contextfrag.SummaryCoverage{},
	}
	prepared, err := preparer.PrepareDirectDiscussPrompt(context.Background(), pipelinepkg.DirectDiscussPromptInput{
		ActorUserID:     "account-user",
		CurrentSourceID: "current",
		Sources: []pipelinepkg.DirectDiscussPromptSource{
			{ID: "old", Message: old, Compactable: true},
			{ID: "summary", Message: summary, Required: true, SummaryFrag: &summaryFrag},
			{
				ID:          "current",
				Message:     current,
				Required:    true,
				Compactable: true,
				ImageRefs:   []pipelinepkg.ImageAttachmentRef{{ContentHash: "current-image"}},
			},
			{ID: "late", Message: lateBinding, Required: true},
		},
	})
	if err != nil {
		t.Fatalf("PrepareDirectDiscussPrompt() error = %v", err)
	}
	materialized, err := prepared.RunConfig.InitialPromptMaterializer(
		context.Background(),
		prepared.RunConfig,
		tools,
	)
	if err != nil {
		t.Fatalf("InitialPromptMaterializer() error = %v", err)
	}
	texts := messageTexts(materialized.Messages)
	if len(texts) != 4 || texts[0] != notice || texts[1] != "<summary>durable summary</summary>" || texts[2] != "current input" || texts[3] != "late binding" {
		t.Fatalf("materialized messages = %#v", texts)
	}
	if !messageHasImage(materialized.Messages[2]) || messageHasImage(materialized.Messages[0]) || messageHasImage(materialized.Messages[1]) || messageHasImage(materialized.Messages[3]) {
		t.Fatalf("source-bound image placement = %#v", materialized.Messages)
	}
	if len(materialized.ContextFrags) != 1 || materialized.ContextFrags[0].Ref.ID != "artifact-a" || materialized.ContextFrags[0].Provenance.Index != 1 {
		t.Fatalf("final summary provenance = %#v", materialized.ContextFrags)
	}
	if total := fixedTokens + estimatePromptMessages(materialized.Messages); total > contextBudget {
		t.Fatalf("final envelope = %d, budget = %d", total, contextBudget)
	}

	if err := prepared.Receipt.Finish(context.Background()); err != nil {
		t.Fatalf("Receipt.Finish() error = %v", err)
	}
	wantPressure := messageTokenPressure(old) + messageTokenPressure(current)
	if compactCalls.Load() != 1 || compactPressure != wantPressure {
		t.Fatalf("compaction receipt = calls:%d pressure:%d, want one call with %d", compactCalls.Load(), compactPressure, wantPressure)
	}
	if compactRequest.UserID != "account-user" || compactRequest.BotID != "bot" || compactRequest.SessionID != "session" {
		t.Fatalf("compaction request = %#v", compactRequest)
	}
	if compactContext.contextTokenBudget != contextBudget {
		t.Fatalf("compaction context budget = %d, want %d", compactContext.contextTokenBudget, contextBudget)
	}
}

func TestDirectDiscussPromptReceiptIsConcurrentAndErrorStable(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("materialization failed")
	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 42},
	}, sentinel)
	var compactCalls atomic.Int32
	receipt := &directDiscussPromptReceipt{
		resolved: resolvedContext{
			compactableTokens:      100,
			compactableTokensKnown: true,
			promptState:            state,
		},
		compact: func(context.Context, int) { compactCalls.Add(1) },
	}

	const callers = 16
	errs := make([]error, callers)
	var wait sync.WaitGroup
	for index := range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = receipt.Finish(context.Background())
		}()
	}
	wait.Wait()

	if compactCalls.Load() != 1 {
		t.Fatalf("compaction calls = %d, want 1", compactCalls.Load())
	}
	for index, err := range errs {
		if !errors.Is(err, sentinel) {
			t.Fatalf("Finish() error %d = %v, want sentinel", index, err)
		}
	}
}

func TestDirectDiscussPromptPreparerMaterializesImageOnlyCurrentSource(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		assetLoader: &fakeGatewayAssetLoader{openFn: func(context.Context, string, string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("image")), "image/png", nil
		}},
	}
	preparer := &directDiscussPromptPreparer{
		resolver: resolver,
		runConfig: agent.RunConfig{
			SupportsImageInput: true,
			Identity:           agent.SessionContext{BotID: "bot", SessionID: "session"},
		},
		contextTokenBudget: 100,
	}

	prepared, err := preparer.PrepareDirectDiscussPrompt(context.Background(), pipelinepkg.DirectDiscussPromptInput{
		CurrentSourceID: "current-image",
		Sources: []pipelinepkg.DirectDiscussPromptSource{{
			ID:          "current-image",
			Message:     sdk.UserMessage(""),
			Required:    true,
			Compactable: true,
			ImageRefs:   []pipelinepkg.ImageAttachmentRef{{ContentHash: "image"}},
		}},
	})
	if err != nil {
		t.Fatalf("PrepareDirectDiscussPrompt() error = %v", err)
	}
	if len(prepared.RunConfig.Messages) != 1 || !messageHasImage(prepared.RunConfig.Messages[0]) {
		t.Fatalf("prepared image-only source = %#v", prepared.RunConfig.Messages)
	}
	materialized, err := prepared.RunConfig.InitialPromptMaterializer(context.Background(), prepared.RunConfig, nil)
	if err != nil {
		t.Fatalf("InitialPromptMaterializer() error = %v", err)
	}
	if len(materialized.Messages) != 1 || !messageHasImage(materialized.Messages[0]) {
		t.Fatalf("materialized image-only source = %#v", materialized.Messages)
	}
}

func TestDirectDiscussPromptReceiptKeepsKnownZero(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{AccountingReady: true}, nil)
	var compactCalls atomic.Int32
	receipt := &directDiscussPromptReceipt{
		resolved: resolvedContext{
			compactableTokens:      99,
			compactableTokensKnown: true,
			promptState:            state,
		},
		compact: func(context.Context, int) { compactCalls.Add(1) },
	}

	if err := receipt.Finish(context.Background()); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if compactCalls.Load() != 0 {
		t.Fatalf("known-zero receipt triggered compaction %d times", compactCalls.Load())
	}
}

func messageTokenPressure(message sdk.Message) int {
	return messageconv.EstimateSDKMessageTokens(message)
}
