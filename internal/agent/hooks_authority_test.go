package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/hooks"
)

type terminalHookTestProvider struct {
	waitForCancel     bool
	closeWithoutAbort bool
	started           chan struct{}
	startOnce         sync.Once
}

func TestNewDoesNotInstallTypedNilHookService(t *testing.T) {
	agent := New(Deps{})
	if agent.hookService != nil {
		t.Fatal("New installed a typed-nil hook service")
	}
}

func (*terminalHookTestProvider) Name() string { return "terminal-hook-test" }

func (*terminalHookTestProvider) ListModels(context.Context) ([]sdk.Model, error) { return nil, nil }

func (*terminalHookTestProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*terminalHookTestProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (*terminalHookTestProvider) DoGenerate(context.Context, sdk.GenerateParams) (*sdk.GenerateResult, error) {
	return &sdk.GenerateResult{FinishReason: sdk.FinishReasonStop}, nil
}

func (p *terminalHookTestProvider) DoStream(ctx context.Context, _ sdk.GenerateParams) (*sdk.StreamResult, error) {
	parts := make(chan sdk.StreamPart, 2)
	go func() {
		defer close(parts)
		parts <- &sdk.StartPart{}
		if p.started != nil {
			p.startOnce.Do(func() { close(p.started) })
		}
		if p.waitForCancel {
			<-ctx.Done()
			if !p.closeWithoutAbort {
				parts <- &sdk.AbortPart{}
			}
			return
		}
		parts <- &sdk.FinishPart{FinishReason: sdk.FinishReasonStop}
	}()
	return &sdk.StreamResult{Stream: parts}, nil
}

type terminalHookTestRunner struct {
	blockTerminal bool
	started       chan hooks.Request
	completed     chan hooks.Request
	canceled      chan error
}

func newTerminalHookTestRunner(block bool) *terminalHookTestRunner {
	return &terminalHookTestRunner{
		blockTerminal: block,
		started:       make(chan hooks.Request, 1),
		completed:     make(chan hooks.Request, 1),
		canceled:      make(chan error, 1),
	}
}

func (r *terminalHookTestRunner) Run(ctx context.Context, req hooks.Request, _ hooks.ToolRunner) (hooks.Result, error) {
	if req.Event != hooks.EventTurnEnd && req.Event != hooks.EventTurnError {
		return hooks.Result{Decision: hooks.DecisionAllow}, nil
	}
	r.started <- req
	if r.blockTerminal {
		<-ctx.Done()
		r.canceled <- context.Cause(ctx)
		return hooks.Result{}, ctx.Err()
	}
	r.completed <- req
	return hooks.Result{Decision: hooks.DecisionAllow}, nil
}

func terminalHookRunConfig(provider *terminalHookTestProvider, authority TerminalHookAuthority) RunConfig {
	return RunConfig{
		Model:                 &sdk.Model{ID: "terminal-hook-model", Provider: provider},
		Messages:              []sdk.Message{sdk.UserMessage("test terminal hook")},
		Identity:              SessionContext{BotID: "bot-1", SessionID: "session-1"},
		TerminalHookAuthority: authority,
	}
}

func terminalEventFromStream(t *testing.T, events <-chan StreamEvent) StreamEvent {
	t.Helper()
	for event := range events {
		if event.Type == EventAgentEnd || event.Type == EventAgentAbort {
			return event
		}
	}
	t.Fatal("agent stream closed without a terminal event")
	return StreamEvent{}
}

func TestRunStreamExecutesTurnEndHookBeforeTerminalEvent(t *testing.T) {
	runner := newTerminalHookTestRunner(false)
	a := &Agent{hookService: runner, logger: slog.New(slog.DiscardHandler)}
	event := terminalEventFromStream(t, a.Stream(context.Background(), terminalHookRunConfig(&terminalHookTestProvider{}, TerminalHookAuthority{})))
	if event.Type != EventAgentEnd {
		t.Fatalf("terminal event = %q, want %q", event.Type, EventAgentEnd)
	}
	select {
	case req := <-runner.completed:
		if req.Event != hooks.EventTurnEnd {
			t.Fatalf("terminal hook event = %q, want %q", req.Event, hooks.EventTurnEnd)
		}
	default:
		t.Fatal("terminal event was emitted before TurnEnd hook completed")
	}
}

func TestRunStreamExecutesTurnErrorHookBeforeUserAbortEvent(t *testing.T) {
	provider := &terminalHookTestProvider{waitForCancel: true, started: make(chan struct{})}
	runner := newTerminalHookTestRunner(false)
	a := &Agent{hookService: runner, logger: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	events := a.Stream(ctx, terminalHookRunConfig(provider, TerminalHookAuthority{}))
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("agent provider did not start")
	}
	cancel()
	event := terminalEventFromStream(t, events)
	if event.Type != EventAgentAbort {
		t.Fatalf("terminal event = %q, want %q", event.Type, EventAgentAbort)
	}
	select {
	case req := <-runner.completed:
		if req.Event != hooks.EventTurnError || req.Error != "agent run aborted" {
			t.Fatalf("terminal hook request = %#v", req)
		}
	default:
		t.Fatal("abort event was emitted before TurnError hook completed")
	}
}

func TestRunStreamTreatsCanceledContextWithoutTerminalPartAsAbort(t *testing.T) {
	provider := &terminalHookTestProvider{waitForCancel: true, closeWithoutAbort: true, started: make(chan struct{})}
	runner := newTerminalHookTestRunner(false)
	a := &Agent{hookService: runner, logger: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	events := a.Stream(ctx, terminalHookRunConfig(provider, TerminalHookAuthority{}))
	<-provider.started
	cancel()
	if event := terminalEventFromStream(t, events); event.Type != EventAgentAbort {
		t.Fatalf("terminal event = %q, want %q", event.Type, EventAgentAbort)
	}
}

func TestStreamEndedAbortedPreservesObservedFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if streamEndedAborted(ctx, false, true) {
		t.Fatal("context cancellation after an observed finish was classified as abort")
	}
	if !streamEndedAborted(ctx, false, false) {
		t.Fatal("context cancellation without a terminal part was not classified as abort")
	}
}

func TestRunStreamSkipsTerminalHookWhenAuthorityWasRevoked(t *testing.T) {
	runner := newTerminalHookTestRunner(false)
	a := &Agent{hookService: runner, logger: slog.New(slog.DiscardHandler)}
	authorityCtx, revoke := context.WithCancelCause(context.Background())
	revoke(errors.New("runtime ownership lost"))
	event := terminalEventFromStream(t, a.Stream(context.Background(), terminalHookRunConfig(&terminalHookTestProvider{}, TerminalHookAuthority{
		Context: authorityCtx,
		Validate: func(context.Context) error {
			t.Fatal("revoked authority must not be validated")
			return nil
		},
	})))
	if event.Type != EventAgentEnd {
		t.Fatalf("terminal event = %q, want %q", event.Type, EventAgentEnd)
	}
	select {
	case req := <-runner.started:
		t.Fatalf("revoked authority executed terminal hook: %#v", req)
	default:
	}
}

func TestRunStreamCancelsTerminalHookWhenAuthorityIsLostDuringExecution(t *testing.T) {
	runner := newTerminalHookTestRunner(true)
	a := &Agent{hookService: runner, logger: slog.New(slog.DiscardHandler)}
	authorityCtx, revoke := context.WithCancelCause(context.Background())
	events := a.Stream(context.Background(), terminalHookRunConfig(&terminalHookTestProvider{}, TerminalHookAuthority{
		Context:  authorityCtx,
		Validate: func(context.Context) error { return nil },
	}))
	terminal := make(chan StreamEvent, 1)
	go func() {
		for event := range events {
			if event.Type == EventAgentEnd || event.Type == EventAgentAbort {
				terminal <- event
				return
			}
		}
	}()

	select {
	case req := <-runner.started:
		if req.Event != hooks.EventTurnEnd {
			t.Fatalf("terminal hook event = %q, want %q", req.Event, hooks.EventTurnEnd)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal hook did not start")
	}
	ownershipErr := errors.New("runtime ownership lost")
	revoke(ownershipErr)
	select {
	case err := <-runner.canceled:
		if !errors.Is(err, ownershipErr) {
			t.Fatalf("terminal hook cancellation = %v, want ownership loss", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ownership loss did not cancel terminal hook")
	}
	select {
	case event := <-terminal:
		if event.Type != EventAgentEnd {
			t.Fatalf("terminal event = %q, want %q", event.Type, EventAgentEnd)
		}
	case <-time.After(time.Second):
		t.Fatal("agent did not emit terminal event after hook cancellation")
	}
}
