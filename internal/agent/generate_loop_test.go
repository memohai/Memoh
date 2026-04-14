package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/memohai/twilight-ai/sdk"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
)

type staticToolProvider struct {
	tools []sdk.Tool
}

func (p staticToolProvider) Tools(context.Context, agenttools.SessionContext) ([]sdk.Tool, error) {
	return p.tools, nil
}

func TestAgentGenerateStopsOnToolLoopAbort(t *testing.T) {
	t.Parallel()

	modelProvider := &agentReadMediaMockProvider{
		handler: func(_ int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			return &sdk.GenerateResult{
				FinishReason: sdk.FinishReasonToolCalls,
				ToolCalls: []sdk.ToolCall{{
					ToolCallID: "call-same",
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

	_, err := a.Generate(context.Background(), RunConfig{
		Model:            &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:         []sdk.Message{sdk.UserMessage("loop")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	})
	if !errors.Is(err, ErrToolLoopDetected) {
		t.Fatalf("expected ErrToolLoopDetected, got %v", err)
	}
	if modelProvider.calls >= 20 {
		t.Fatalf("expected tool loop to stop generation, got %d provider calls", modelProvider.calls)
	}
}

func TestAgentGenerateStopsOnTextLoopAbort(t *testing.T) {
	t.Parallel()

	repeatedText := "abcdefghijklmnopqrstuvwxyz0123456789 repeated text chunk for loop detection"
	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			return &sdk.GenerateResult{
				Text:         repeatedText,
				FinishReason: sdk.FinishReasonToolCalls,
				ToolCalls: []sdk.ToolCall{{
					ToolCallID: "call-text",
					ToolName:   "noop_tool",
					Input:      map[string]any{"step": call},
				}},
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
		Messages:         []sdk.Message{sdk.UserMessage("loop text")},
		SupportsToolCall: true,
		Identity:         SessionContext{BotID: "bot-1"},
		LoopDetection:    LoopDetectionConfig{Enabled: true},
	})
	if !errors.Is(err, ErrTextLoopDetected) {
		t.Fatalf("expected ErrTextLoopDetected, got %v", err)
	}
	if modelProvider.calls >= 10 {
		t.Fatalf("expected text loop to stop generation, got %d provider calls", modelProvider.calls)
	}
}

func TestAgentGenerateStopsOnTerminalTextLoopAbort(t *testing.T) {
	t.Parallel()

	repeatedText := "abcdefghijklmnopqrstuvwxyz0123456789 repeated text chunk for loop detection"
	modelProvider := &agentReadMediaMockProvider{
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
	if modelProvider.calls != 4 {
		t.Fatalf("expected terminal text loop to abort on final step, got %d provider calls", modelProvider.calls)
	}
}
