package agent

import (
	"context"
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

type agentToolPlaceholderProvider struct{}

func (*agentToolPlaceholderProvider) Name() string { return "tool-placeholder-mock" }

func (*agentToolPlaceholderProvider) ListModels(context.Context) ([]sdk.Model, error) {
	return nil, nil
}

func (*agentToolPlaceholderProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK, Message: "ok"}
}

func (*agentToolPlaceholderProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true, Message: "supported"}, nil
}

func (*agentToolPlaceholderProvider) DoGenerate(context.Context, sdk.GenerateParams) (*sdk.GenerateResult, error) {
	return &sdk.GenerateResult{FinishReason: sdk.FinishReasonStop}, nil
}

func (*agentToolPlaceholderProvider) DoStream(_ context.Context, _ sdk.GenerateParams) (*sdk.StreamResult, error) {
	ch := make(chan sdk.StreamPart, 8)
	go func() {
		defer close(ch)
		ch <- &sdk.StartPart{}
		ch <- &sdk.StartStepPart{}
		ch <- &sdk.ToolInputStartPart{ID: "call-1", ToolName: "write"}
		ch <- &sdk.StreamToolCallPart{
			ToolCallID: "call-1",
			ToolName:   "write",
			Input:      map[string]any{"path": "/tmp/long.txt"},
		}
		ch <- &sdk.FinishStepPart{FinishReason: sdk.FinishReasonStop}
		ch <- &sdk.FinishPart{FinishReason: sdk.FinishReasonStop}
	}()
	return &sdk.StreamResult{Stream: ch}, nil
}

func TestAgentStreamEmitsEarlyToolPlaceholderBeforeFullInput(t *testing.T) {
	t.Parallel()

	a := New(Deps{})

	var events []StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model: &sdk.Model{
			ID:       "mock-model",
			Provider: &agentToolPlaceholderProvider{},
		},
		Messages:         []sdk.Message{sdk.UserMessage("write a long file")},
		SupportsToolCall: false,
		Identity:         SessionContext{BotID: "bot-1"},
	}) {
		events = append(events, event)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %#v", len(events), events)
	}
	if events[0].Type != EventAgentStart {
		t.Fatalf("expected first event %q, got %#v", EventAgentStart, events[0])
	}
	if events[1].Type != EventToolCallStart || events[1].ToolCallID != "call-1" || events[1].ToolName != "write" {
		t.Fatalf("unexpected placeholder tool event: %#v", events[1])
	}
	if events[1].Input != nil {
		t.Fatalf("expected placeholder tool event to have nil input, got %#v", events[1].Input)
	}
	if events[2].Type != EventToolCallStart || events[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected full tool event: %#v", events[2])
	}
	expectedInput := map[string]any{"path": "/tmp/long.txt"}
	if !reflect.DeepEqual(events[2].Input, expectedInput) {
		t.Fatalf("expected full tool event input %#v, got %#v", expectedInput, events[2].Input)
	}
	if events[3].Type != EventAgentEnd {
		t.Fatalf("expected terminal event %q, got %#v", EventAgentEnd, events[3])
	}
}
