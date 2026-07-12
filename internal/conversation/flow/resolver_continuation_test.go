package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messageconv"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/userinput"
)

type continuationHistoryService struct {
	recordingMessageService
	history []messagepkg.Message
}

func (s *continuationHistoryService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.history...), nil
}

func TestPrepareContinuationRunConfigReplacesStaleContextAndSetsCapabilities(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{userInput: &userinput.Service{}}
	eventCh := make(chan WSStreamEvent)
	base := agent.RunConfig{
		Query:    "stale query",
		Messages: []sdk.Message{sdk.UserMessage("stale context")},
		ContextFrags: []contextfrag.ContextFrag{{
			ID:   "stale-fragment",
			Kind: contextfrag.KindConversationEvent,
		}},
	}

	got, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{},
		base,
		historyfrag.ScopeFallback{},
		contextfrag.Scope{},
		eventCh,
		"",
		0,
	)
	if err != nil {
		t.Fatalf("prepareContinuationRunConfig() error = %v", err)
	}
	cfg := got.runConfig
	if cfg.Query != "" || len(cfg.Messages) != 0 {
		t.Fatalf("continuation retained stale context: %#v", got)
	}
	for _, frag := range cfg.ContextFrags {
		if frag.ID == "stale-fragment" {
			t.Fatalf("continuation retained stale fragment: %#v", frag)
		}
	}
	if !cfg.LiveToolStream || !cfg.CanRequestUserInput {
		t.Fatalf("continuation capabilities = live:%v input:%v, want true/true", cfg.LiveToolStream, cfg.CanRequestUserInput)
	}
	if cfg.InitialPromptMaterializer == nil || got.promptState == nil || !got.compactableTokensKnown {
		t.Fatalf("continuation materialization receipt was not installed: %#v", got)
	}
}

func TestPrepareContinuationRunConfigDefersFinalBudgetAndReportsRawPressure(t *testing.T) {
	t.Parallel()

	oldest := strings.Repeat("old ", 20000)
	resolver := &Resolver{
		logger: slog.New(slog.DiscardHandler),
		messageService: &continuationHistoryService{history: []messagepkg.Message{
			persistedHistoryMessage(t, "old", "user", oldest),
			persistedHistoryMessage(t, "new", "assistant", "recent answer"),
		}},
	}
	rc, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{},
		agent.RunConfig{},
		historyfrag.ScopeFallback{},
		contextfrag.Scope{SessionID: "session-1"},
		nil,
		"",
		4096,
	)
	if err != nil {
		t.Fatalf("prepareContinuationRunConfig() error = %v", err)
	}
	baselineTexts := messageTexts(rc.runConfig.Messages)
	if len(baselineTexts) != 2 || baselineTexts[0] != oldest {
		t.Fatalf("continuation baseline was budgeted early: %#v", baselineTexts)
	}
	materialized, err := rc.runConfig.InitialPromptMaterializer(context.Background(), rc.runConfig, nil)
	if err != nil {
		t.Fatalf("InitialPromptMaterializer() error = %v", err)
	}
	if got := messageTexts(materialized.Messages); len(got) != 2 || !strings.HasPrefix(got[0], "[System Notice]") || got[1] != "recent answer" {
		t.Fatalf("final continuation messages = %#v, want notice and recent answer", got)
	}
	finalPressure, known := rc.compactionPressure()
	if rc.compactableTokens <= 0 || !known || finalPressure != rc.compactableTokens {
		t.Fatalf("continuation raw pressure = %d final=%d known=%v", rc.compactableTokens, finalPressure, known)
	}
	if outcome, ok := rc.promptState.Snapshot(); !ok || !outcome.AccountingReady || outcome.Allocation.CompactableTokens != rc.compactableTokens {
		t.Fatalf("continuation prompt outcome = %#v, set=%v", outcome, ok)
	}
}

func TestPrepareContinuationRunConfigRequiresLatestToolResultOccurrence(t *testing.T) {
	t.Parallel()

	history := make([]messagepkg.Message, 0, 12)
	for index := range 10 {
		history = append(history, persistedHistoryMessage(t, "history-"+string(rune('a'+index)), "user", strings.Repeat("older ", 2000)))
	}
	history = append(history,
		persistedSDKHistoryMessage(t, "call", sdk.Message{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "call-current",
				ToolName:   "request_user_input",
				Input:      map[string]any{"question": "continue?"},
			}},
		}),
		persistedSDKHistoryMessage(t, "result", sdk.ToolMessage(sdk.ToolResultPart{
			ToolCallID: "call-current",
			ToolName:   "request_user_input",
			Result:     "yes",
		})),
	)
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler), messageService: &continuationHistoryService{history: history}}

	rc, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{},
		agent.RunConfig{},
		historyfrag.ScopeFallback{},
		contextfrag.Scope{SessionID: "session-1"},
		nil,
		"call-current",
		4096,
	)
	if err != nil {
		t.Fatalf("prepareContinuationRunConfig() error = %v", err)
	}
	materialized, err := rc.runConfig.InitialPromptMaterializer(context.Background(), rc.runConfig, nil)
	if err != nil {
		t.Fatalf("InitialPromptMaterializer() error = %v", err)
	}
	analysis := messageconv.AnalyzeSDKToolOccurrences(materialized.Messages)
	if len(analysis.Matches) != 1 || analysis.Matches[0].CallID != "call-current" {
		t.Fatalf("latest continuation occurrence was stripped: messages=%#v analysis=%#v", materialized.Messages, analysis)
	}
	if len(materialized.Messages) >= len(rc.runConfig.Messages) {
		t.Fatalf("tight final budget did not trim older continuation history: before=%d after=%d", len(rc.runConfig.Messages), len(materialized.Messages))
	}
}

func TestPrepareContinuationRunConfigDrainsAndRepinsToolOccurrence(t *testing.T) {
	t.Parallel()

	old := persistedHistoryMessage(t, "old", "user", strings.Repeat("old raw context ", 1000))
	call := persistedSDKHistoryMessage(t, "call", sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-current",
			ToolName:   "request_user_input",
			Input:      map[string]any{"question": "continue?"},
		}},
	})
	result := persistedSDKHistoryMessage(t, "result", sdk.ToolMessage(sdk.ToolResultPart{
		ToolCallID: "call-current",
		ToolName:   "request_user_input",
		Result:     "yes",
	}))
	history := &continuationHistoryService{history: []messagepkg.Message{old, call, result}}
	attempts := 0
	var compactReq conversation.ChatRequest
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: history,
		syncCompactionFn: func(_ context.Context, req conversation.ChatRequest, _, _ int) (compaction.Result, error) {
			attempts++
			compactReq = req
			history.history = []messagepkg.Message{call, result}
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
	}

	rc, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session-1", UserID: "user"},
		agent.RunConfig{},
		historyfrag.ScopeFallback{},
		contextfrag.Scope{BotID: "bot", SessionID: "session-1"},
		nil,
		"call-current",
		2000,
	)
	if err != nil {
		t.Fatalf("prepareContinuationRunConfig() error = %v", err)
	}
	if attempts != 1 || rc.compactableTokens >= preSendCompactionThreshold(2000) {
		t.Fatalf("continuation drain = attempts:%d pressure:%d", attempts, rc.compactableTokens)
	}
	if compactReq.BotID != "bot" || compactReq.SessionID != "session-1" || compactReq.UserID != "user" {
		t.Fatalf("continuation compaction request = %#v", compactReq)
	}
	if rc.promptState.ClaimCompaction() {
		t.Fatal("post-send compaction claim remained available after pre-send attempt")
	}
	materialized, err := rc.runConfig.InitialPromptMaterializer(context.Background(), rc.runConfig, nil)
	if err != nil {
		t.Fatalf("InitialPromptMaterializer() error = %v", err)
	}
	analysis := messageconv.AnalyzeSDKToolOccurrences(materialized.Messages)
	if len(analysis.Matches) != 1 || analysis.Matches[0].CallID != "call-current" {
		t.Fatalf("rebuilt continuation occurrence was not pinned: %#v", analysis)
	}
	if strings.Contains(strings.Join(messageTexts(materialized.Messages), "\n"), "old raw context") {
		t.Fatalf("materialized continuation retained pre-compaction history: %#v", materialized.Messages)
	}
}

func TestPrepareContinuationRunConfigRejectsCompactionThatRemovedRequiredOccurrence(t *testing.T) {
	t.Parallel()

	old := persistedHistoryMessage(t, "old", "user", strings.Repeat("old raw context ", 1000))
	call := persistedSDKHistoryMessage(t, "call", sdk.Message{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: "call-current",
			ToolName:   "request_user_input",
			Input:      map[string]any{"question": "continue?"},
		}},
	})
	result := persistedSDKHistoryMessage(t, "result", sdk.ToolMessage(sdk.ToolResultPart{
		ToolCallID: "call-current",
		ToolName:   "request_user_input",
		Result:     "yes",
	}))
	history := &continuationHistoryService{history: []messagepkg.Message{old, call, result}}
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		messageService: history,
		syncCompactionFn: func(context.Context, conversation.ChatRequest, int, int) (compaction.Result, error) {
			history.history = nil
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
	}

	_, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session-1", UserID: "user"},
		agent.RunConfig{},
		historyfrag.ScopeFallback{},
		contextfrag.Scope{BotID: "bot", SessionID: "session-1"},
		nil,
		"call-current",
		2000,
	)
	if err == nil || !strings.Contains(err.Error(), `continuation tool occurrence "call-current" was not found`) {
		t.Fatalf("prepareContinuationRunConfig() error = %v", err)
	}
}

func TestRequireContinuationToolOccurrenceSelectsLatestRepeatedRawID(t *testing.T) {
	t.Parallel()

	messages := messageconv.SDKMessagesToModelMessages([]sdk.Message{
		{
			Role:    sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{ToolCallID: "reused", ToolName: "first", Input: map[string]any{}}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "reused", ToolName: "first", Result: "first result"}),
		{
			Role:    sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{ToolCallID: "reused", ToolName: "second", Input: map[string]any{}}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{ToolCallID: "reused", ToolName: "second", Result: "second result"}),
	})
	projection := budgetSourcesForHistoryRecords([]historyfrag.HistoryRecord{
		{ModelMessage: messages[0]},
		{ModelMessage: messages[1]},
		{ModelMessage: messages[2]},
		{ModelMessage: messages[3]},
	})

	anchored, err := requireContinuationToolOccurrence(projection, "reused")
	if err != nil {
		t.Fatalf("requireContinuationToolOccurrence() error = %v", err)
	}
	if anchored.sources[0].Retention != contextbudget.RetentionCandidate || anchored.sources[1].Retention != contextbudget.RetentionCandidate {
		t.Fatalf("older reused occurrence was promoted: %#v", anchored.sources[:2])
	}
	if anchored.sources[2].Retention != contextbudget.RetentionRequired || anchored.sources[3].Retention != contextbudget.RetentionRequired {
		t.Fatalf("latest reused occurrence was not promoted: %#v", anchored.sources[2:])
	}
}

func TestResolvedContextClaimsCompactionPressureOnce(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 31},
	}, nil)
	rc := resolvedContext{promptState: state}
	if pressure, known, claimed := rc.claimCompactionPressure(); !claimed || !known || pressure != 31 {
		t.Fatalf("first claim = pressure:%d known:%v claimed:%v, want 31/true/true", pressure, known, claimed)
	}
	if pressure, known, claimed := rc.claimCompactionPressure(); claimed || known || pressure != 0 {
		t.Fatalf("second claim = pressure:%d known:%v claimed:%v, want 0/false/false", pressure, known, claimed)
	}
}

func TestConsumeContinuationStreamReturnsTypedMaterializationError(t *testing.T) {
	t.Parallel()

	overflow := &PromptEnvelopeOverflowError{ContextBudget: 10, TotalTokens: 11}
	state := &initialPromptState{}
	state.Store(initialPromptResult{}, overflow)
	rc := resolvedContext{
		compactableTokensKnown: true,
		promptState:            state,
	}
	stream := make(chan agent.StreamEvent, 1)
	stream <- agent.StreamEvent{Type: agent.EventError, Error: overflow.Error()}
	close(stream)
	forwarded := make(chan WSStreamEvent, 1)
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}

	err := resolver.consumeContinuationStream(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session"},
		rc,
		forwarded,
		stream,
	)
	var got *PromptEnvelopeOverflowError
	if !errors.As(err, &got) || got != overflow {
		t.Fatalf("consumeContinuationStream() error = %v, want typed overflow", err)
	}
	if len(forwarded) != 0 {
		t.Fatalf("materialization error was also forwarded as a string event")
	}
	if _, _, claimed := rc.claimCompactionPressure(); claimed {
		t.Fatal("no-terminal continuation did not consume its compaction claim")
	}
}

func TestConsumeContinuationStreamPersistsTerminalOnce(t *testing.T) {
	t.Parallel()

	terminalMessages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("continued")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	stream := make(chan agent.StreamEvent, 1)
	stream <- agent.StreamEvent{Type: agent.EventAgentEnd, Messages: terminalMessages}
	close(stream)
	state := &initialPromptState{}
	state.Store(initialPromptResult{AccountingReady: true}, nil)
	rc := resolvedContext{
		compactableTokensKnown: true,
		promptState:            state,
	}
	messages := &recordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	forwarded := make(chan WSStreamEvent, 1)

	err = resolver.consumeContinuationStream(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session", UserMessagePersisted: true},
		rc,
		forwarded,
		stream,
	)
	if err != nil {
		t.Fatalf("consumeContinuationStream() error = %v", err)
	}
	if len(messages.persisted) != 1 || messages.persisted[0].Role != "assistant" {
		t.Fatalf("persisted messages = %#v, want one terminal assistant", messages.persisted)
	}
	if len(forwarded) != 1 {
		t.Fatalf("forwarded events = %d, want one terminal event", len(forwarded))
	}
	if _, _, claimed := rc.claimCompactionPressure(); claimed {
		t.Fatal("terminal continuation did not consume its compaction claim")
	}
}

func TestModelContextTokenBudget(t *testing.T) {
	t.Parallel()

	positive := 4096
	negative := -1
	if got := modelContextTokenBudget(models.GetResponse{}); got != 0 {
		t.Fatalf("nil context window budget = %d, want 0", got)
	}
	if got := modelContextTokenBudget(models.GetResponse{Model: models.Model{Config: models.ModelConfig{ContextWindow: &negative}}}); got != 0 {
		t.Fatalf("negative context window budget = %d, want 0", got)
	}
	if got := modelContextTokenBudget(models.GetResponse{Model: models.Model{Config: models.ModelConfig{ContextWindow: &positive}}}); got != positive {
		t.Fatalf("positive context window budget = %d, want %d", got, positive)
	}
}

func TestPrepareContinuationRunConfigPropagatesArtifactProjectionFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("artifact projection unavailable")
	resolver := &Resolver{queries: &recordingCompactionLogQueries{listErr: sentinel}}
	got, err := resolver.prepareContinuationRunConfig(
		context.Background(),
		conversation.ChatRequest{},
		agent.RunConfig{Query: "must not survive"},
		historyfrag.ScopeFallback{},
		contextfrag.Scope{SessionID: "00000000-0000-0000-0000-00000000f401"},
		nil,
		"",
		0,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("prepareContinuationRunConfig() error = %v, want %v", err, sentinel)
	}
	if got.runConfig.Query != "" || len(got.runConfig.Messages) != 0 || len(got.runConfig.ContextFrags) != 0 {
		t.Fatalf("failed continuation returned partial config: %#v", got)
	}
}

func persistedSDKHistoryMessage(t *testing.T, id string, message sdk.Message) messagepkg.Message {
	t.Helper()
	model := sdkModelMessage(t, message)
	content, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal SDK history message: %v", err)
	}
	return messagepkg.Message{ID: id, Role: model.Role, Content: content}
}
