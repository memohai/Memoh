package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type staticToolProvider struct {
	tools []sdk.Tool
}

func (p staticToolProvider) Tools(context.Context, agenttools.SessionContext) ([]sdk.Tool, error) {
	return p.tools, nil
}

type atomicMockProvider struct {
	calls   atomic.Int32
	handler func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error)
	stream  func(ctx context.Context, params sdk.GenerateParams) (*sdk.StreamResult, error)
}

func (*atomicMockProvider) Name() string {
	return "mock"
}

func (*atomicMockProvider) ListModels(context.Context) ([]sdk.Model, error) {
	return nil, nil
}

func (*atomicMockProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK, Message: "ok"}
}

func (*atomicMockProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true, Message: "supported"}, nil
}

func (m *atomicMockProvider) DoGenerate(_ context.Context, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
	call := int(m.calls.Add(1))
	return m.handler(call, params)
}

func (m *atomicMockProvider) DoStream(ctx context.Context, params sdk.GenerateParams) (*sdk.StreamResult, error) {
	if m.stream != nil {
		return m.stream(ctx, params)
	}

	result, err := m.DoGenerate(ctx, params)
	if err != nil {
		return nil, err
	}
	ch := make(chan sdk.StreamPart, 8)
	go func() {
		defer close(ch)
		ch <- &sdk.StartPart{}
		ch <- &sdk.StartStepPart{}
		if result.Text != "" {
			ch <- &sdk.TextStartPart{ID: "mock"}
			ch <- &sdk.TextDeltaPart{ID: "mock", Text: result.Text}
			ch <- &sdk.TextEndPart{ID: "mock"}
		}
		for _, tc := range result.ToolCalls {
			ch <- &sdk.StreamToolCallPart{
				ToolCallID: tc.ToolCallID,
				ToolName:   tc.ToolName,
				Input:      tc.Input,
			}
		}
		ch <- &sdk.FinishStepPart{
			FinishReason: result.FinishReason,
			Usage:        result.Usage,
			Response:     result.Response,
		}
		ch <- &sdk.FinishPart{
			FinishReason: result.FinishReason,
			TotalUsage:   result.Usage,
		}
	}()
	return &sdk.StreamResult{Stream: ch}, nil
}

func TestAgentGenerateStopsOnTerminalTextLoopAbort(t *testing.T) {
	t.Parallel()

	repeatedText := "abcdefghijklmnopqrstuvwxyz0123456789 repeated text chunk for loop detection"
	modelProvider := &atomicMockProvider{
		handler: func(call int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			finishReason := sdk.FinishReasonToolCalls
			var toolCalls []sdk.ToolCall
			if call < 4 {
				toolCalls = []sdk.ToolCall{{
					ToolCallID: "call-terminal",
					ToolName:   "noop_tool",
					Input:      map[string]any{"step": call},
				}}
			} else {
				finishReason = sdk.FinishReasonStop
			}
			return &sdk.GenerateResult{
				Text:         repeatedText,
				FinishReason: finishReason,
				ToolCalls:    toolCalls,
			}, nil
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		staticToolProvider{
			tools: []sdk.Tool{{
				Name:       "noop_tool",
				Parameters: &jsonschema.Schema{Type: "object"},
				Execute: func(_ *sdk.ToolExecContext, _ any) (any, error) {
					return map[string]any{"ok": true}, nil
				},
			}},
		},
	})

	_, err := a.Generate(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:         []sdk.Message{sdk.UserMessage("loop text terminal")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	})
	if !errors.Is(err, ErrTextLoopDetected) {
		t.Fatalf("expected ErrTextLoopDetected, got %v", err)
	}
	if modelProvider.calls.Load() != 4 {
		t.Fatalf("expected terminal text loop to abort on final step, got %d provider calls", modelProvider.calls.Load())
	}
}

func TestAgentStreamStopsOnToolLoopAbort(t *testing.T) {
	t.Parallel()

	modelProvider := &atomicMockProvider{
		handler: func(call int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			if call >= 20 {
				return &sdk.GenerateResult{
					Text:         "unexpected-final-step",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			}
			return &sdk.GenerateResult{
				FinishReason: sdk.FinishReasonToolCalls,
				ToolCalls: []sdk.ToolCall{{
					ToolCallID: "call-stream",
					ToolName:   "loop_tool",
					Input:      map[string]any{"query": "same"},
				}},
			}, nil
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		staticToolProvider{
			tools: []sdk.Tool{{
				Name:       "loop_tool",
				Parameters: &jsonschema.Schema{Type: "object"},
				Execute: func(_ *sdk.ToolExecContext, _ any) (any, error) {
					return map[string]any{"ok": true}, nil
				},
			}},
		},
	})

	var terminal StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:         []sdk.Message{sdk.UserMessage("loop stream")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	}) {
		if event.IsTerminal() {
			terminal = event
		}
	}

	if terminal.Type != EventAgentAbort {
		t.Fatalf("expected EventAgentAbort, got %q", terminal.Type)
	}
}

func TestAgentStreamMarksTerminalTextLoopAsAbort(t *testing.T) {
	t.Parallel()

	repeatedChunk := strings.Repeat("abcd", 64)
	var observedCancel atomic.Bool
	modelProvider := &atomicMockProvider{
		stream: func(ctx context.Context, _ sdk.GenerateParams) (*sdk.StreamResult, error) {
			ch := make(chan sdk.StreamPart, 16)
			go func() {
				defer close(ch)
				send := func(part sdk.StreamPart) bool {
					select {
					case <-ctx.Done():
						observedCancel.Store(true)
						return false
					case ch <- part:
						return true
					}
				}
				if !send(&sdk.StartPart{}) {
					return
				}
				if !send(&sdk.StartStepPart{}) {
					return
				}
				if !send(&sdk.TextStartPart{ID: "mock"}) {
					return
				}
				for i := 0; i < 4; i++ {
					if !send(&sdk.TextDeltaPart{ID: "mock", Text: repeatedChunk}) {
						return
					}
				}
				select {
				case <-ctx.Done():
					observedCancel.Store(true)
					return
				case <-time.After(50 * time.Millisecond):
				}
				if !send(&sdk.TextEndPart{ID: "mock"}) {
					return
				}
				if !send(&sdk.FinishStepPart{FinishReason: sdk.FinishReasonStop}) {
					return
				}
				_ = send(&sdk.FinishPart{FinishReason: sdk.FinishReasonStop})
			}()
			return &sdk.StreamResult{Stream: ch}, nil
		},
	}

	a := New(Deps{})

	var terminal StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:         []sdk.Message{sdk.UserMessage("loop stream text")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	}) {
		if event.IsTerminal() {
			terminal = event
		}
	}

	if !observedCancel.Load() {
		t.Fatal("expected stream provider to observe context cancellation from text-loop abort")
	}
	if terminal.Type != EventAgentAbort {
		t.Fatalf("expected EventAgentAbort, got %q", terminal.Type)
	}
}

func TestAgentStreamMarksRetryTextLoopAsAbort(t *testing.T) {
	t.Parallel()

	repeatedChunk := strings.Repeat("abcd", 64)
	var streamCalls atomic.Int32
	var observedCancel atomic.Bool
	modelProvider := &atomicMockProvider{
		stream: func(ctx context.Context, _ sdk.GenerateParams) (*sdk.StreamResult, error) {
			call := streamCalls.Add(1)
			ch := make(chan sdk.StreamPart, 16)
			go func() {
				defer close(ch)
				send := func(part sdk.StreamPart) bool {
					select {
					case <-ctx.Done():
						observedCancel.Store(true)
						return false
					case ch <- part:
						return true
					}
				}

				if !send(&sdk.StartPart{}) {
					return
				}
				if !send(&sdk.StartStepPart{}) {
					return
				}

				if call == 1 {
					_ = send(&sdk.ErrorPart{Error: errors.New("api error 500")})
					return
				}

				if !send(&sdk.TextStartPart{ID: "mock-retry"}) {
					return
				}
				for i := 0; i < 4; i++ {
					if !send(&sdk.TextDeltaPart{ID: "mock-retry", Text: repeatedChunk}) {
						return
					}
				}
				select {
				case <-ctx.Done():
					observedCancel.Store(true)
					return
				case <-time.After(50 * time.Millisecond):
				}
				if !send(&sdk.TextEndPart{ID: "mock-retry"}) {
					return
				}
				if !send(&sdk.FinishStepPart{FinishReason: sdk.FinishReasonStop}) {
					return
				}
				_ = send(&sdk.FinishPart{FinishReason: sdk.FinishReasonStop})
			}()
			return &sdk.StreamResult{Stream: ch}, nil
		},
	}

	a := New(Deps{})

	var terminal StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:         []sdk.Message{sdk.UserMessage("loop stream retry text")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	}) {
		if event.IsTerminal() {
			terminal = event
		}
	}

	if streamCalls.Load() != 2 {
		t.Fatalf("expected one retry stream attempt, got %d stream calls", streamCalls.Load())
	}
	if !observedCancel.Load() {
		t.Fatal("expected retry stream provider to observe context cancellation from text-loop abort")
	}
	if terminal.Type != EventAgentAbort {
		t.Fatalf("expected EventAgentAbort after retry text-loop abort, got %q", terminal.Type)
	}
}

func TestRunMidStreamRetryMarksTextLoopCancellationAsAborted(t *testing.T) {
	t.Parallel()

	repeatedChunk := strings.Repeat("abcd", 64)
	var observedCancel atomic.Bool
	var retryParams sdk.GenerateParams
	modelProvider := &atomicMockProvider{
		stream: func(ctx context.Context, params sdk.GenerateParams) (*sdk.StreamResult, error) {
			retryParams = cloneGenerateParams(params)
			ch := make(chan sdk.StreamPart)
			go func() {
				defer close(ch)
				send := func(part sdk.StreamPart) bool {
					select {
					case <-ctx.Done():
						observedCancel.Store(true)
						return false
					case ch <- part:
						return true
					}
				}

				if !send(&sdk.StartPart{}) {
					return
				}
				if !send(&sdk.StartStepPart{}) {
					return
				}
				if !send(&sdk.TextStartPart{ID: "mock-retry-only"}) {
					return
				}
				for i := 0; i < 4; i++ {
					if !send(&sdk.TextDeltaPart{ID: "mock-retry-only", Text: repeatedChunk}) {
						return
					}
				}
				select {
				case <-ctx.Done():
					observedCancel.Store(true)
					return
				case <-time.After(200 * time.Millisecond):
					t.Error("expected text-loop detection to cancel retry stream before any extra part was sent")
					return
				}
			}()
			return &sdk.StreamResult{Stream: ch}, nil
		},
	}

	a := New(Deps{})
	streamCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	textLoopGuard := NewTextLoopGuard(LoopDetectedStreakThreshold, LoopDetectedMinNewGramsPerChunk, SentialOptions{})
	textLoopProbeBuffer := NewTextLoopProbeBuffer(LoopDetectedProbeChars, func(text string) {
		result := textLoopGuard.Inspect(text)
		if result.Abort {
			cancel(ErrTextLoopDetected)
		}
	})

	retryResult, aborted := a.runMidStreamRetry(
		context.Background(),
		streamCtx,
		cancel,
		newToolAbortRegistry(),
		make(chan StreamEvent, 32),
		RunConfig{
			Model:         &sdk.Model{ID: "mock-model", Provider: modelProvider},
			Messages:      []sdk.Message{sdk.UserMessage("retry text loop")},
			Identity:      SessionContext{BotID: "bot-1"},
			LoopDetection: LoopDetectionConfig{Enabled: true},
		},
		nil,
		nil,
		nil,
		&sdk.StreamResult{Messages: []sdk.Message{sdk.UserMessage("previous step")}},
		0,
		"api error 500",
		&strings.Builder{},
		textLoopProbeBuffer,
		nil,
		nil,
	)

	if retryResult == nil {
		t.Fatal("expected retry result")
	}
	if !observedCancel.Load() {
		t.Fatal("expected retry stream provider to observe context cancellation from text-loop abort")
	}
	if !errors.Is(context.Cause(streamCtx), ErrTextLoopDetected) {
		t.Fatalf("expected stream context cause ErrTextLoopDetected, got %v", context.Cause(streamCtx))
	}
	if !aborted {
		t.Fatal("expected runMidStreamRetry to report aborted when retry stream hit text-loop cancellation")
	}
	if len(retryParams.Messages) != 2 {
		t.Fatalf("retry messages = %#v, want materialized prefix plus previous output", retryParams.Messages)
	}
	if got := retryParams.Messages[0].Content[0].(sdk.TextPart).Text; got != "retry text loop" {
		t.Fatalf("retry prefix = %q", got)
	}
	if got := retryParams.Messages[1].Content[0].(sdk.TextPart).Text; got != "previous step" {
		t.Fatalf("retry output = %q", got)
	}
}

func TestStreamMidStreamRetryPreservesPreparedInjection(t *testing.T) {
	t.Parallel()

	retryParams := make(chan sdk.GenerateParams, 1)
	modelProvider := &atomicMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1:
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					Usage:        sdk.Usage{InputTokens: 11, TotalTokens: 11},
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "noop_tool",
						Input:      map[string]any{},
					}},
				}, nil
			case 2:
				return nil, errors.New("api error 500")
			default:
				retryParams <- cloneGenerateParams(params)
				return &sdk.GenerateResult{
					Text:         "done",
					FinishReason: sdk.FinishReasonStop,
					Usage:        sdk.Usage{InputTokens: 13, TotalTokens: 13},
				}, nil
			}
		},
	}
	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{staticToolProvider{tools: []sdk.Tool{{
		Name:       "noop_tool",
		Parameters: &jsonschema.Schema{Type: "object"},
		Execute: func(*sdk.ToolExecContext, any) (any, error) {
			return "ok", nil
		},
	}}}})
	injectCh := make(chan InjectMessage, 1)
	injectCh <- InjectMessage{Text: "keep this injection"}
	close(injectCh)
	var materializerCalls atomic.Int32

	var terminal StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider, Type: sdk.ModelTypeChat},
		Messages:         []sdk.Message{sdk.UserMessage("initial")},
		SupportsToolCall: true,
		InjectCh:         injectCh,
		InitialPromptMaterializer: func(_ context.Context, cfg RunConfig, _ []sdk.Tool) (RunConfig, error) {
			materializerCalls.Add(1)
			return cfg, nil
		},
	}) {
		if event.IsTerminal() {
			terminal = event
		}
	}

	params := <-retryParams
	found := false
	for _, message := range params.Messages {
		for _, part := range message.Content {
			text, ok := part.(sdk.TextPart)
			if ok && text.Text == "keep this injection" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("retry messages lost prepared injection: %#v", params.Messages)
	}
	if got := materializerCalls.Load(); got != 1 {
		t.Fatalf("materializer calls = %d, want 1", got)
	}
	var usage sdk.Usage
	if err := json.Unmarshal(terminal.Usage, &usage); err != nil {
		t.Fatalf("unmarshal terminal usage: %v", err)
	}
	if usage.InputTokens != 24 {
		t.Fatalf("terminal input tokens = %d, want initial and retry steps", usage.InputTokens)
	}
}

func TestStreamMidStreamRetryKeepsBackgroundBaseline(t *testing.T) {
	t.Parallel()

	bgMgr := background.New(nil)
	taskCtx, cancelTask := context.WithCancel(context.Background())
	defer cancelTask()
	started := make(chan struct{})
	bgMgr.Spawn(taskCtx, "bot-1", "session-1", "long task", "/repo", "Long task", func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		close(started)
		<-ctx.Done()
		return &bridge.ExecResult{ExitCode: -1}, ctx.Err()
	}, nil, nil)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("background task did not start")
	}

	finalParams := make(chan sdk.GenerateParams, 1)
	modelProvider := &atomicMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1, 3:
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: fmt.Sprintf("call-%d", call),
						ToolName:   "noop_tool",
						Input:      map[string]any{},
					}},
				}, nil
			case 2:
				return nil, errors.New("api error 500")
			default:
				finalParams <- cloneGenerateParams(params)
				return &sdk.GenerateResult{Text: "done", FinishReason: sdk.FinishReasonStop}, nil
			}
		},
	}
	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{staticToolProvider{tools: []sdk.Tool{{
		Name:       "noop_tool",
		Parameters: &jsonschema.Schema{Type: "object"},
		Execute: func(*sdk.ToolExecContext, any) (any, error) {
			return "ok", nil
		},
	}}}})

	for range a.Stream(context.Background(), RunConfig{
		Model:             &sdk.Model{ID: "mock-model", Provider: modelProvider, Type: sdk.ModelTypeChat},
		System:            "base system",
		Messages:          []sdk.Message{sdk.UserMessage("initial")},
		SupportsToolCall:  true,
		BackgroundManager: bgMgr,
		Identity:          SessionContext{BotID: "bot-1", SessionID: "session-1"},
	}) {
	}

	params := <-finalParams
	if got := strings.Count(params.System, "Currently running background tasks:"); got != 1 {
		t.Fatalf("background summary count = %d, want 1:\n%s", got, params.System)
	}
}

type anthropicAtomicProvider struct {
	*atomicMockProvider
}

func (*anthropicAtomicProvider) Name() string { return "anthropic-messages" }

func TestStreamInjectedRecorderCountsOnlyCompletedOutputMessages(t *testing.T) {
	t.Parallel()

	modelProvider := &anthropicAtomicProvider{atomicMockProvider: &atomicMockProvider{
		handler: func(call int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			if call == 1 {
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "noop_tool",
						Input:      map[string]any{},
					}},
				}, nil
			}
			return &sdk.GenerateResult{Text: "done", FinishReason: sdk.FinishReasonStop}, nil
		},
	}}
	agent := New(Deps{})
	agent.SetToolProviders([]agenttools.ToolProvider{staticToolProvider{tools: []sdk.Tool{{
		Name:       "noop_tool",
		Parameters: &jsonschema.Schema{Type: "object"},
		Execute: func(*sdk.ToolExecContext, any) (any, error) {
			return "ok", nil
		},
	}}}})
	injectCh := make(chan InjectMessage, 1)
	injectCh <- InjectMessage{ReceiptID: "receipt-1", Text: "injected"}
	close(injectCh)
	insertAfter := -1
	recordedReceiptID := ""
	recordedText := ""

	for range agent.Stream(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "claude-test", Provider: modelProvider, Type: sdk.ModelTypeChat},
		System:           "cached system",
		Messages:         []sdk.Message{sdk.UserMessage("initial")},
		SupportsToolCall: true,
		InjectCh:         injectCh,
		InjectedRecorder: func(receipt InjectedReceipt) {
			recordedReceiptID = string(receipt.ID)
			recordedText = receipt.ModelText
			insertAfter = receipt.InsertAfter
		},
		InitialPromptMaterializer: func(_ context.Context, cfg RunConfig, _ []sdk.Tool) (RunConfig, error) {
			cfg.Messages = append([]sdk.Message(nil), cfg.Messages...)
			return cfg, nil
		},
	}) {
	}

	if insertAfter != 2 {
		t.Fatalf("injected insertAfter = %d, want two completed output messages", insertAfter)
	}
	if recordedReceiptID != "receipt-1" || recordedText != "injected" {
		t.Fatalf("recorded injection = %q/%q, want receipt-1/injected", recordedReceiptID, recordedText)
	}
}
