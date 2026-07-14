package inbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/session"
)

func TestActiveStreamRegistryFencesOwnerRemoval(t *testing.T) {
	t.Parallel()
	var registry activeStreamRegistry
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	firstOwner, accepted := registry.Register("bot-1:route-1", "session-1", cancelFirst)
	if !accepted {
		t.Fatal("first owner was rejected")
	}
	if _, accepted := registry.Register("bot-1:route-1", "session-1", cancelSecond); !accepted {
		t.Fatal("second owner was rejected")
	}
	registry.Remove("bot-1:route-1", firstOwner)
	if got := registry.Count("bot-1:route-1"); got != 1 {
		t.Fatalf("active owners = %d, want 1 after first owner exits", got)
	}
	if got := registry.CancelAll("bot-1:route-1"); got != 1 {
		t.Fatalf("canceled owners = %d, want 1", got)
	}
	select {
	case <-firstCtx.Done():
		t.Fatal("removed owner was canceled by later route cancellation")
	default:
	}
	select {
	case <-secondCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("remaining owner was not canceled")
	}
}

func TestActiveStreamRegistryScopeAdvancePreservesNewAndRejectsLateOld(t *testing.T) {
	t.Parallel()
	var registry activeStreamRegistry
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer cancelOld()
	if _, accepted := registry.Register("bot-1:route-1", "session-old", cancelOld); !accepted {
		t.Fatal("old owner was rejected")
	}
	if got := registry.AdvanceScope("bot-1:route-1", "session-new"); got != 1 {
		t.Fatalf("canceled old owners = %d, want 1", got)
	}
	select {
	case <-oldCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("old owner was not canceled by scope advance")
	}
	newCtx, cancelNew := context.WithCancel(context.Background())
	defer cancelNew()
	if _, accepted := registry.Register("bot-1:route-1", "session-new", cancelNew); !accepted {
		t.Fatal("new owner was rejected")
	}
	if _, accepted := registry.Register("bot-1:route-1", "session-old", func() {}); accepted {
		t.Fatal("late old owner was accepted after scope advance")
	}
	if got := registry.AdvanceScope("bot-1:route-1", "session-new"); got != 0 {
		t.Fatalf("same-scope advance canceled %d owners, want 0", got)
	}
	select {
	case <-newCtx.Done():
		t.Fatal("same-scope advance canceled new owner")
	default:
	}
}

func TestActiveStreamRegistryNewOwnerCanWinCreateAdvanceWindow(t *testing.T) {
	t.Parallel()
	var registry activeStreamRegistry
	newCtx, cancelNew := context.WithCancel(context.Background())
	defer cancelNew()
	if _, accepted := registry.Register("bot-1:route-1", "session-new", cancelNew); !accepted {
		t.Fatal("new owner was rejected before /new scope advance")
	}
	if got := registry.AdvanceScope("bot-1:route-1", "session-new"); got != 0 {
		t.Fatalf("scope advance canceled %d new owners, want 0", got)
	}
	select {
	case <-newCtx.Done():
		t.Fatal("/new scope advance canceled the already-running new owner")
	default:
	}
}

func TestRouteAdmissionRevalidatesMatchingTombstoneAgainstActiveSession(t *testing.T) {
	t.Parallel()
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.AdvanceScope("route-1", "session-old")
	processor := &ChannelInboundProcessor{
		dispatcher: dispatcher,
		sessionEnsurer: &statefulSessionEnsurer{active: SessionResult{
			ID: "session-new", Type: session.TypeChat, Runtime: session.RuntimeModel,
		}},
	}
	oldTurn := testDeferredTurn("late-old")
	oldTurn.sessionID = "session-old"
	oldTurn.identity.BotID = "bot-1"
	admission := processor.admitRouteTurn(context.Background(), "route-1", routeIntentContinue, oldTurn)
	if admission.Kind != routeAdmissionStale {
		t.Fatalf("old admission = %#v, want stale after DB session switch", admission)
	}
	if scope, known := dispatcher.CurrentScope("route-1"); !known || scope != "session-new" {
		t.Fatalf("dispatcher scope = (%q, %v), want session-new", scope, known)
	}
}

func TestRouteSessionCreationSerializesCommitAndGenerationAdvance(t *testing.T) {
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	ensurer := &orderedCreateSessionEnsurer{firstEntered: firstEntered, secondEntered: secondEntered, releaseFirst: releaseFirst}
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.AdvanceScope("route-1", "session-old")
	processor := &ChannelInboundProcessor{dispatcher: dispatcher, sessionEnsurer: ensurer}
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := processor.createNewRouteSession(context.Background(), "bot-1", "route-1", "telegram", NewSessionSpec{})
		firstDone <- err
	}()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first creation did not enter session service")
	}
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		_, _, err := processor.createNewRouteSession(context.Background(), "bot-1", "route-1", "telegram", NewSessionSpec{})
		secondDone <- err
	}()
	<-secondStarted
	secondEnteredBeforeRelease := false
	select {
	case <-secondEntered:
		secondEnteredBeforeRelease = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first createNewRouteSession() error = %v", err)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second creation did not enter after first transition completed")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second createNewRouteSession() error = %v", err)
	}
	if secondEnteredBeforeRelease {
		t.Fatal("second session creation entered before the first DB transition was advanced")
	}
	if scope, known := dispatcher.CurrentScope("route-1"); !known || scope != "session-2" {
		t.Fatalf("dispatcher scope = (%q, %v), want session-2", scope, known)
	}
	if scope, known := processor.activeStreams.CurrentScope("bot-1:route-1"); !known || scope != "session-2" {
		t.Fatalf("local scope = (%q, %v), want session-2", scope, known)
	}
}

func TestCreatedSessionMustBeActiveBeforeGenerationAdvance(t *testing.T) {
	t.Parallel()
	ensurer := &misdirectedSessionEnsurer{active: SessionResult{ID: "session-old", Type: session.TypeChat}}
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.AdvanceScope("route-1", "session-old")
	processor := &ChannelInboundProcessor{dispatcher: dispatcher, sessionEnsurer: ensurer}
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer cancelOld()
	if _, accepted := processor.activeStreams.Register("bot-1:route-1", "session-old", cancelOld); !accepted {
		t.Fatal("old local owner was rejected")
	}
	if _, _, err := processor.createNewRouteSession(context.Background(), "bot-1", "route-1", "telegram", NewSessionSpec{}); err == nil {
		t.Fatal("createNewRouteSession() succeeded for a session that was not made active")
	}
	if scope, _ := dispatcher.CurrentScope("route-1"); scope != "session-old" {
		t.Fatalf("dispatcher scope = %q, want session-old", scope)
	}
	if scope, _ := processor.activeStreams.CurrentScope("bot-1:route-1"); scope != "session-old" {
		t.Fatalf("local scope = %q, want session-old", scope)
	}
	select {
	case <-oldCtx.Done():
		t.Fatal("failed DB activation canceled the current owner")
	default:
	}
}

func TestCreatedSessionVerificationFailureFencesOldGeneration(t *testing.T) {
	t.Parallel()
	ensurer := &flakyVerificationSessionEnsurer{}
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.AdvanceScope("route-1", "session-old")
	old := dispatcher.Admit("route-1", routeIntentContinue, &deferredTurn{sessionID: "session-old"})
	processor := &ChannelInboundProcessor{dispatcher: dispatcher, sessionEnsurer: ensurer}
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer cancelOld()
	old.Lease.BindCancel(cancelOld)
	localCtx, cancelLocal := context.WithCancel(context.Background())
	defer cancelLocal()
	if _, accepted := processor.activeStreams.Register("bot-1:route-1", "session-old", cancelLocal); !accepted {
		t.Fatal("old local owner was rejected")
	}
	if _, _, err := processor.createNewRouteSession(context.Background(), "bot-1", "route-1", "telegram", NewSessionSpec{}); err == nil {
		t.Fatal("createNewRouteSession() succeeded without verifying the active session")
	}
	for name, done := range map[string]<-chan struct{}{
		"dispatcher lease": oldCtx.Done(),
		"local owner":      localCtx.Done(),
	} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s survived unverified session transition", name)
		}
	}
	if scope, _ := dispatcher.CurrentScope("route-1"); scope != unverifiedRouteScope {
		t.Fatalf("dispatcher scope = %q, want unverified fence", scope)
	}
	if scope, _ := processor.activeStreams.CurrentScope("bot-1:route-1"); scope != unverifiedRouteScope {
		t.Fatalf("local scope = %q, want unverified fence", scope)
	}
	ensurer.AllowVerification()
	current := processor.admitRouteTurn(context.Background(), "route-1", routeIntentContinue, &deferredTurn{
		identity: InboundIdentity{BotID: "bot-1"}, sessionID: "session-new",
	})
	if current.Kind != routeAdmissionStartPrimary || current.Lease == nil {
		t.Fatalf("verified recovery admission = %#v, want primary", current)
	}
	current.Lease.Release()
}

func TestLocalContinuationRegistersCancellationBeforeOpeningStream(t *testing.T) {
	t.Parallel()
	processor := &ChannelInboundProcessor{}
	sender := &cancelAwareOpenStreamSender{opened: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	runnerCalled := make(chan struct{}, 1)
	go func() {
		done <- processor.streamContinuationCommand(ctx, channel.InboundMessage{
			Channel: channel.ChannelType("web"), ReplyTarget: "local:user-1", Message: channel.Message{ID: "message-1"},
		}, sender, InboundIdentity{BotID: "bot-1"}, "route-1", "session-1", func(context.Context, chan<- flow.WSStreamEvent) error {
			runnerCalled <- struct{}{}
			return nil
		})
	}()
	select {
	case <-sender.opened:
	case <-time.After(time.Second):
		t.Fatal("continuation did not attempt to open stream")
	}
	if got := processor.activeStreams.CancelAll("bot-1:route-1"); got != 1 {
		t.Fatalf("canceled owners = %d, want 1 while OpenStream is pending", got)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("streamContinuationCommand() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("continuation did not stop after route cancellation")
	}
	select {
	case <-runnerCalled:
		t.Fatal("continuation runner started after stream cancellation")
	default:
	}
	if got := processor.activeStreams.Count("bot-1:route-1"); got != 0 {
		t.Fatalf("active owners = %d, want 0 after continuation exits", got)
	}
}

type cancelAwareOpenStreamSender struct{ opened chan struct{} }

func (*cancelAwareOpenStreamSender) Send(context.Context, channel.OutboundMessage) error { return nil }

func (s *cancelAwareOpenStreamSender) OpenStream(ctx context.Context, _ string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	close(s.opened)
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

type orderedCreateSessionEnsurer struct {
	mu            sync.Mutex
	createCalls   int
	active        SessionResult
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
}

func (e *orderedCreateSessionEnsurer) EnsureActiveSession(ctx context.Context, _, routeID, _ string) (SessionResult, error) {
	return e.GetActiveSession(ctx, routeID)
}

func (e *orderedCreateSessionEnsurer) GetActiveSession(_ context.Context, _ string) (SessionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active.ID == "" {
		return SessionResult{}, errors.New("no active session")
	}
	return e.active, nil
}

func (e *orderedCreateSessionEnsurer) CreateNewSession(_ context.Context, _, _, _ string, _ NewSessionSpec) (SessionResult, error) {
	e.mu.Lock()
	e.createCalls++
	call := e.createCalls
	sess := SessionResult{ID: fmt.Sprintf("session-%d", call), Type: session.TypeChat, Runtime: session.RuntimeModel}
	e.active = sess
	switch call {
	case 1:
		close(e.firstEntered)
	case 2:
		close(e.secondEntered)
	}
	e.mu.Unlock()
	if call == 1 {
		<-e.releaseFirst
	}
	return sess, nil
}

type misdirectedSessionEnsurer struct{ active SessionResult }

func (e *misdirectedSessionEnsurer) EnsureActiveSession(context.Context, string, string, string) (SessionResult, error) {
	return e.active, nil
}

func (e *misdirectedSessionEnsurer) GetActiveSession(context.Context, string) (SessionResult, error) {
	return e.active, nil
}

func (*misdirectedSessionEnsurer) CreateNewSession(context.Context, string, string, string, NewSessionSpec) (SessionResult, error) {
	return SessionResult{ID: "session-not-active", Type: session.TypeChat}, nil
}

type flakyVerificationSessionEnsurer struct {
	mu                sync.Mutex
	active            SessionResult
	allowVerification bool
}

func (e *flakyVerificationSessionEnsurer) EnsureActiveSession(ctx context.Context, _, routeID, _ string) (SessionResult, error) {
	return e.GetActiveSession(ctx, routeID)
}

func (e *flakyVerificationSessionEnsurer) GetActiveSession(context.Context, string) (SessionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.allowVerification {
		return SessionResult{}, errors.New("verification unavailable")
	}
	return e.active, nil
}

func (e *flakyVerificationSessionEnsurer) CreateNewSession(context.Context, string, string, string, NewSessionSpec) (SessionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = SessionResult{ID: "session-new", Type: session.TypeChat, Runtime: session.RuntimeModel}
	return e.active, nil
}

func (e *flakyVerificationSessionEnsurer) AllowVerification() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.allowVerification = true
}
