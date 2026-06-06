package flow

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/userinput"
)

type fakeUserInputService struct {
	target    userinput.Request
	resolved  userinput.Request
	hasWaiter bool

	submitCalls int
	cancelCalls int
	submitted   userinput.SubmitInput
	canceled    userinput.CancelInput
	submitErr   error
	cancelErr   error
}

func (f *fakeUserInputService) HasWaiter(string) bool {
	return f.hasWaiter
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
		target:    userinput.Request{ID: "input-1", Status: userinput.StatusPending, ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP}},
		resolved:  resolved,
		hasWaiter: true,
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
		hasWaiter: true,
		submitErr: userinput.ErrAlreadyDecided,
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

func TestRespondUserInputWithoutWaiterCancelsInsteadOfSubmitting(t *testing.T) {
	t.Parallel()

	fake := &fakeUserInputService{
		target: userinput.Request{
			ID:               "input-1",
			Status:           userinput.StatusPending,
			ProviderMetadata: map[string]any{"source": userinput.ProviderSourceACPMCP},
		},
		resolved:  chatResolvedRequest(),
		hasWaiter: false,
	}
	resolver := &Resolver{
		userInput: fake,
		continueUserInputFn: func(context.Context, userinput.Request, UserInputResponseInput, sdk.ToolResultPart, chan<- WSStreamEvent) error {
			t.Error("orphaned waiter-backed request must not continue the session")
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
	// No live waiter would consume the answer; submitting would look
	// successful while nothing continues. The request must be closed out.
	if fake.submitCalls != 0 {
		t.Fatalf("submit calls = %d, want 0", fake.submitCalls)
	}
	if fake.cancelCalls != 1 || fake.canceled.RequestID != "input-1" {
		t.Fatalf("cancel calls/input = %d/%#v", fake.cancelCalls, fake.canceled)
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
