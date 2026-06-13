package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/userinput"
)

type fakeUserInputService struct {
	target   userinput.Request
	resolved userinput.Request

	submitCalls   int
	cancelCalls   int
	submitted     userinput.SubmitInput
	canceled      userinput.CancelInput
	submitErr     error
	cancelErr     error
	submitHook    func()
	canRespond    bool
	canRespondSet bool
}

func (*fakeUserInputService) CreatePending(context.Context, userinput.CreatePendingInput) (userinput.Request, error) {
	return userinput.Request{}, errors.New("unexpected CreatePending")
}

func (f *fakeUserInputService) ResolveTarget(context.Context, userinput.ResolveInput) (userinput.Request, error) {
	return f.target, nil
}

func (f *fakeUserInputService) Submit(_ context.Context, input userinput.SubmitInput) (userinput.Request, error) {
	f.submitCalls++
	f.submitted = input
	if f.submitHook != nil {
		f.submitHook()
	}
	if f.submitErr != nil {
		return userinput.Request{}, f.submitErr
	}
	return f.resolved, nil
}

func (f *fakeUserInputService) Cancel(_ context.Context, input userinput.CancelInput) (userinput.Request, error) {
	f.cancelCalls++
	f.canceled = input
	if f.cancelErr != nil {
		return userinput.Request{}, f.cancelErr
	}
	return f.resolved, nil
}

func (f *fakeUserInputService) CanRespond(req userinput.Request) bool {
	if f.canRespondSet {
		return f.canRespond
	}
	if userinput.IsACPMCPRequest(req) {
		return false
	}
	return req.Status == userinput.StatusPending
}

func chatResolvedRequest() userinput.Request {
	return userinput.Request{
		ID:         "input-1",
		SessionID:  "session-1",
		ToolCallID: "call-1",
		ToolName:   userinput.ToolNameAskUser,
		Status:     userinput.StatusSubmitted,
		Result: map[string]any{
			"status": userinput.StatusSubmitted,
			"answers": []any{
				map[string]any{"question_id": "q1", "selected": []any{map[string]any{"id": "q1.o1", "label": "Plan A"}}},
			},
		},
	}
}

func collectAgentStreamEvents(t *testing.T, ch <-chan WSStreamEvent, count int) []agentpkg.StreamEvent {
	t.Helper()
	events := make([]agentpkg.StreamEvent, 0, count)
	timeout := time.After(2 * time.Second)
	for len(events) < count {
		select {
		case raw := <-ch:
			var ev agentpkg.StreamEvent
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal stream event: %v", err)
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatalf("timed out waiting for stream event %d/%d", len(events)+1, count)
		}
	}
	return events
}

func TestRespondUserInputContinuesChatSession(t *testing.T) {
	t.Parallel()

	fake := &fakeUserInputService{
		target:   userinput.Request{ID: "input-1", Status: userinput.StatusPending},
		resolved: chatResolvedRequest(),
	}
	var continued *sdk.ToolResultPart
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(_ context.Context, req userinput.Request, _ UserInputResponseInput, result sdk.ToolResultPart, _ chan<- WSStreamEvent) error {
			if req.ID != "input-1" {
				t.Errorf("continued request = %#v", req)
			}
			continued = &result
			return nil
		},
	}

	eventCh := make(chan WSStreamEvent, 4)
	answers := []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}}
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Answers:   answers,
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}

	if fake.submitCalls != 1 || fake.cancelCalls != 0 {
		t.Fatalf("submit/cancel calls = %d/%d", fake.submitCalls, fake.cancelCalls)
	}
	if fake.submitted.RequestID != "input-1" || len(fake.submitted.Answers) != 1 || fake.submitted.Answers[0].QuestionID != "q1" {
		t.Fatalf("submitted input = %#v", fake.submitted)
	}
	if continued == nil {
		t.Fatal("chat request must continue the session")
	}
	if continued.ToolCallID != "call-1" || continued.ToolName != userinput.ToolNameAskUser {
		t.Fatalf("continued tool result = %#v", continued)
	}
	if len(eventCh) != 0 {
		t.Fatalf("chat continuation must not emit ack events, got %d", len(eventCh))
	}
}

func TestRespondUserInputOnlyAcksACPRequests(t *testing.T) {
	t.Parallel()

	resolved := chatResolvedRequest()
	resolved.ProviderMetadata = map[string]any{"source": userinput.ProviderSourceACPMCP}
	fake := &fakeUserInputService{
		target:        userinput.Request{ID: "input-1", Status: userinput.StatusPending, ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP}},
		resolved:      resolved,
		canRespond:    true,
		canRespondSet: true,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("ACP request must not continue the chat session; the blocked waiter resumes it")
			return nil
		},
	}

	eventCh := make(chan WSStreamEvent, 4)
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Answers:   []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.submitCalls != 1 {
		t.Fatalf("submit calls = %d", fake.submitCalls)
	}
	// emitApprovalAck sends agent start + end so the client stream settles.
	if len(eventCh) != 2 {
		t.Fatalf("ack events = %d, want 2", len(eventCh))
	}
}

func TestRespondUserInputAcksAlreadyDecidedACPRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		submitErr:     userinput.ErrAlreadyDecided,
		canRespond:    true,
		canRespondSet: true,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("already-decided ACP request must not continue the chat session")
			return nil
		},
	}

	eventCh := make(chan WSStreamEvent, 4)
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Answers:   []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.submitCalls != 1 || fake.cancelCalls != 0 {
		t.Fatalf("submit/cancel calls = %d/%d", fake.submitCalls, fake.cancelCalls)
	}
	if len(eventCh) != 2 {
		t.Fatalf("ack events = %d, want 2", len(eventCh))
	}
}

func TestRespondUserInputACPRequestSubmitsWithLiveWaiter(t *testing.T) {
	t.Parallel()

	resolved := chatResolvedRequest()
	resolved.ProviderMetadata = map[string]any{"source": userinput.ProviderSourceACPMCP}
	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		resolved:      resolved,
		canRespond:    true,
		canRespondSet: true,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("ACP request must not continue the session in this response handler")
			return nil
		},
	}

	eventCh := make(chan WSStreamEvent, 4)
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Answers:   []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", fake.submitCalls)
	}
	if fake.cancelCalls != 0 {
		t.Fatalf("cancel calls = %d, want 0", fake.cancelCalls)
	}
	if len(eventCh) != 2 {
		t.Fatalf("ack events = %d, want 2", len(eventCh))
	}
}

func TestRespondUserInputACPRequestReattachesActivePrompt(t *testing.T) {
	t.Parallel()

	resolved := chatResolvedRequest()
	resolved.ProviderMetadata = map[string]any{"source": userinput.ProviderSourceACPMCP}
	submitted := make(chan struct{})
	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			SessionID:        "session-1",
			ToolCallID:       "call-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		resolved:      resolved,
		submitHook:    func() { close(submitted) },
		canRespond:    true,
		canRespondSet: true,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("ACP request must resume through the active ACP prompt")
			return nil
		},
	}
	hub := resolver.registerACPActivePrompt("bot-1", "session-1")
	if hub == nil {
		t.Fatal("expected active ACP prompt hub")
	}
	defer resolver.unregisterACPActivePrompt("bot-1", "session-1", hub)

	eventCh := make(chan WSStreamEvent, 8)
	done := make(chan error, 1)
	go func() {
		done <- resolver.RespondUserInput(context.Background(), UserInputResponseInput{
			BotID:     "bot-1",
			SessionID: "session-1",
			Answers:   []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
		}, eventCh)
	}()

	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for submit")
	}
	hub.emit(agentpkg.StreamEvent{
		Type:        agentpkg.EventUserInputRequest,
		ToolCallID:  "call-1",
		ToolName:    userinput.ToolNameAskUser,
		UserInputID: "input-1",
		Status:      userinput.StatusSubmitted,
	})
	hub.emit(agentpkg.StreamEvent{
		Type:       agentpkg.EventToolCallEnd,
		ToolCallID: "call-1",
		ToolName:   userinput.ToolNameAskUser,
		Result:     map[string]any{"status": userinput.StatusSubmitted},
	})
	hub.emit(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "continuing"})
	hub.emit(agentpkg.StreamEvent{Type: agentpkg.EventEnd})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("respond user input: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ACP prompt reattach")
	}

	events := collectAgentStreamEvents(t, eventCh, 3)
	if events[0].Type != agentpkg.EventStart {
		t.Fatalf("first event = %q, want %q", events[0].Type, agentpkg.EventStart)
	}
	if events[1].Type != agentpkg.EventTextDelta || events[1].Delta != "continuing" {
		t.Fatalf("second event = %#v, want text delta", events[1])
	}
	if events[2].Type != agentpkg.EventEnd {
		t.Fatalf("third event = %q, want %q", events[2].Type, agentpkg.EventEnd)
	}
	if fake.submitCalls != 1 || fake.cancelCalls != 0 {
		t.Fatalf("submit/cancel calls = %d/%d", fake.submitCalls, fake.cancelCalls)
	}
}

func TestRespondUserInputACPRequestCanSuppressActivePromptReattach(t *testing.T) {
	t.Parallel()

	resolved := chatResolvedRequest()
	resolved.ProviderMetadata = map[string]any{"source": userinput.ProviderSourceACPMCP}
	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			SessionID:        "session-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		resolved:      resolved,
		canRespond:    true,
		canRespondSet: true,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("ACP request must not continue the chat session")
			return nil
		},
	}
	hub := resolver.registerACPActivePrompt("bot-1", "session-1")
	if hub == nil {
		t.Fatal("expected active ACP prompt hub")
	}
	defer resolver.unregisterACPActivePrompt("bot-1", "session-1", hub)

	eventCh := make(chan WSStreamEvent, 4)
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:                      "bot-1",
		SessionID:                  "session-1",
		Answers:                    []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
		SuppressActivePromptAttach: true,
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.submitCalls != 1 || fake.cancelCalls != 0 {
		t.Fatalf("submit/cancel calls = %d/%d", fake.submitCalls, fake.cancelCalls)
	}
	if len(eventCh) != 2 {
		t.Fatalf("ack events = %d, want 2", len(eventCh))
	}
}

func TestRespondUserInputACPRequestWithoutWaiterCancelsInsteadOfSubmitting(t *testing.T) {
	t.Parallel()

	resolved := chatResolvedRequest()
	resolved.ProviderMetadata = map[string]any{"source": userinput.ProviderSourceACPMCP}
	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		resolved: resolved,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("orphaned ACP request must not continue the chat session")
			return nil
		},
	}

	eventCh := make(chan WSStreamEvent, 4)
	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Answers:   []userinput.QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}, eventCh)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.submitCalls != 0 {
		t.Fatalf("submit calls = %d, want 0", fake.submitCalls)
	}
	if fake.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", fake.cancelCalls)
	}
	if fake.canceled.RequestID != "input-1" || fake.canceled.Reason == "" {
		t.Fatalf("canceled input = %#v", fake.canceled)
	}
	if len(eventCh) != 2 {
		t.Fatalf("ack events = %d, want 2", len(eventCh))
	}
}

func TestRespondUserInputCancelRoutesToCancel(t *testing.T) {
	t.Parallel()

	fake := &fakeUserInputService{
		target:   userinput.Request{ID: "input-1", Status: userinput.StatusPending},
		resolved: chatResolvedRequest(),
	}
	continueCalls := 0
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			continueCalls++
			return nil
		},
	}

	err := resolver.RespondUserInput(context.Background(), UserInputResponseInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		Canceled:  true,
		Reason:    "user_canceled",
	}, nil)
	if err != nil {
		t.Fatalf("respond user input: %v", err)
	}
	if fake.cancelCalls != 1 || fake.submitCalls != 0 {
		t.Fatalf("cancel/submit calls = %d/%d", fake.cancelCalls, fake.submitCalls)
	}
	if fake.canceled.RequestID != "input-1" || fake.canceled.Reason != "user_canceled" {
		t.Fatalf("canceled input = %#v", fake.canceled)
	}
	if continueCalls != 1 {
		t.Fatalf("canceled chat request must still continue the session, calls = %d", continueCalls)
	}
}
