package inbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/session"
	skillset "github.com/memohai/memoh/internal/skills"
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
	ensurer := &orderedCreateSessionEnsurer{
		firstEntered:  firstEntered,
		secondEntered: secondEntered,
		releaseFirst:  releaseFirst,
	}
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
	currentTurn := &deferredTurn{
		identity:  InboundIdentity{BotID: "bot-1"},
		sessionID: "session-new",
	}
	current := processor.admitRouteTurn(context.Background(), "route-1", routeIntentContinue, currentTurn)
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
		done <- processor.streamContinuationCommand(
			ctx,
			channel.InboundMessage{
				Channel:     channel.ChannelType("web"),
				ReplyTarget: "local:user-1",
				Message:     channel.Message{ID: "message-1"},
			},
			sender,
			InboundIdentity{BotID: "bot-1"},
			"route-1",
			"session-1",
			func(context.Context, chan<- flow.WSStreamEvent) error {
				runnerCalled <- struct{}{}
				return nil
			},
		)
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

type cancelAwareOpenStreamSender struct {
	opened chan struct{}
}

func (*cancelAwareOpenStreamSender) Send(context.Context, channel.OutboundMessage) error {
	return nil
}

func (s *cancelAwareOpenStreamSender) OpenStream(ctx context.Context, _ string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	close(s.opened)
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func TestRouteExecutionFallsBackOnlyUncommittedInjections(t *testing.T) {
	for _, test := range []struct {
		name         string
		commit       bool
		wantFallback bool
	}{
		{name: "durable commit", commit: true, wantFallback: false},
		{name: "uncommitted fallback", commit: false, wantFallback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			identityService := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: testChannelIdentityUUID}}
			chatService := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
			requests := make(chan conversation.ChatRequest, 2)
			releasePrimary := make(chan struct{})
			gateway := &fakeChatGateway{
				resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{
					Role: "assistant", Content: conversation.NewTextContent("done"),
				}}},
				onChat: func(req conversation.ChatRequest) {
					requests <- req
					if req.Query == "first" {
						<-releasePrimary
					}
				},
			}
			processor := NewChannelInboundProcessor(
				slog.New(slog.DiscardHandler), nil, chatService, chatService, gateway,
				identityService, &fakePolicyService{}, "", 0,
			)
			processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
				ID: "session-1", Type: session.TypeChat, Runtime: session.RuntimeModel,
			}})
			pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
			processor.SetPipeline(pipeline, nil, nil)
			processor.SetDispatcher(NewRouteDispatcher(slog.New(slog.DiscardHandler)))
			sender := &fakeReplySender{}
			cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}
			baseMessage := channel.InboundMessage{
				BotID:       "bot-1",
				Channel:     channel.ChannelTypeTelegram,
				ReplyTarget: "telegram:user-1",
				Sender:      channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
				Conversation: channel.Conversation{
					ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat",
				},
			}

			primaryDone := make(chan error, 1)
			go func() {
				message := baseMessage
				message.Message = channel.Message{ID: "external-first", Text: "first"}
				primaryDone <- processor.HandleInbound(context.Background(), cfg, message, sender)
			}()
			primaryRequest := awaitRouteRequest(t, requests)
			if primaryRequest.InjectionFeed.Messages == nil || primaryRequest.InjectionFeed.CommitPersisted == nil {
				t.Fatal("primary request did not own an injection feed")
			}

			injectedMessage := baseMessage
			injectedMessage.Message = channel.Message{ID: "external-injected", Text: "/btw injected"}
			if err := processor.HandleInbound(context.Background(), cfg, injectedMessage, sender); err != nil {
				t.Fatalf("inject HandleInbound() error = %v", err)
			}
			var injected conversation.InjectMessage
			select {
			case injected = <-primaryRequest.InjectionFeed.Messages:
			case <-time.After(time.Second):
				t.Fatal("primary feed did not receive injection")
			}
			if test.commit && !primaryRequest.InjectionFeed.CommitPersisted(injected.Receipt.ID) {
				t.Fatal("primary feed rejected durable injection commit")
			}

			close(releasePrimary)
			if err := <-primaryDone; err != nil {
				t.Fatalf("primary HandleInbound() error = %v", err)
			}
			if test.wantFallback {
				fallback := awaitRouteRequest(t, requests)
				if fallback.Query != "injected" {
					t.Fatalf("fallback request = %#v", fallback)
				}
			} else {
				select {
				case fallback := <-requests:
					t.Fatalf("committed injection fell back: %#v", fallback)
				case <-time.After(25 * time.Millisecond):
				}
			}

			ic, loaded := pipeline.GetIC("session-1")
			if !loaded || len(ic.Nodes) != 2 {
				t.Fatalf("pipeline context = loaded:%v nodes:%d", loaded, len(ic.Nodes))
			}
			if identityService.calls != 2 {
				t.Fatalf("identity resolutions = %d, want ingress-only 2", identityService.calls)
			}
		})
	}
}

func awaitRouteRequest(t *testing.T, requests <-chan conversation.ChatRequest) conversation.ChatRequest {
	t.Helper()
	select {
	case req := <-requests:
		return req
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for route request")
		return conversation.ChatRequest{}
	}
}

func TestStopCommandFencesLeaseBeforeStreamBindsCancel(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("active"))
	processor := &ChannelInboundProcessor{
		dispatcher:    dispatcher,
		routeResolver: &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}},
		logger:        slog.New(slog.DiscardHandler),
	}
	message := channel.InboundMessage{
		Channel:     channel.ChannelTypeTelegram,
		ReplyTarget: "telegram:user-1",
		Conversation: channel.Conversation{
			ID: "user-1", Type: channel.ConversationTypePrivate,
		},
	}
	if err := processor.handleStopCommand(
		context.Background(),
		channel.ChannelConfig{},
		message,
		&fakeReplySender{},
		InboundIdentity{BotID: "bot-1"},
	); err != nil {
		t.Fatalf("handleStopCommand() error = %v", err)
	}

	cancelled := make(chan struct{})
	primary.Lease.BindCancel(func() { close(cancelled) })
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("pending stop did not cancel lease when stream bound")
	}
	primary.Lease.Release()
}

func TestRouteExecutionSessionChangeCannotInjectOrPromoteOldWork(t *testing.T) {
	t.Parallel()

	identityService := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: testChannelIdentityUUID}}
	chatService := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{
		ID: "session-old", Type: session.TypeChat, Runtime: session.RuntimeModel,
	}}
	requests := make(chan conversation.ChatRequest, 3)
	releaseOld := make(chan struct{})
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{
			Role: "assistant", Content: conversation.NewTextContent("done"),
		}}},
		onChat: func(req conversation.ChatRequest) {
			requests <- req
			if req.SessionID == "session-old" && req.Query == "old first" {
				<-releaseOld
			}
		},
	}
	processor := NewChannelInboundProcessor(
		slog.New(slog.DiscardHandler), nil, chatService, chatService, gateway,
		identityService, &fakePolicyService{}, "", 0,
	)
	processor.SetSessionEnsurer(ensurer)
	processor.SetPipeline(pipelinepkg.NewPipeline(pipelinepkg.RenderParams{}), nil, nil)
	processor.SetDispatcher(NewRouteDispatcher(slog.New(slog.DiscardHandler)))
	oldSender := &fakeReplySender{}
	newSender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}
	baseMessage := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelTypeTelegram,
		ReplyTarget: "telegram:user-1",
		Sender:      channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
		Conversation: channel.Conversation{
			ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat",
		},
	}

	oldDone := make(chan error, 1)
	go func() {
		message := baseMessage
		message.Message = channel.Message{ID: "external-old", Text: "old first"}
		oldDone <- processor.HandleInbound(context.Background(), cfg, message, oldSender)
	}()
	oldRequest := awaitRouteRequest(t, requests)

	queued := baseMessage
	queued.Message = channel.Message{ID: "external-queued", Text: "/next old queued"}
	if err := processor.HandleInbound(context.Background(), cfg, queued, oldSender); err != nil {
		t.Fatalf("old queue HandleInbound() error = %v", err)
	}
	ensurer.activeSession = SessionResult{ID: "session-new", Type: session.TypeChat, Runtime: session.RuntimeModel}
	newMessage := baseMessage
	newMessage.Message = channel.Message{ID: "external-new", Text: "new message"}
	if err := processor.HandleInbound(context.Background(), cfg, newMessage, newSender); err != nil {
		t.Fatalf("new HandleInbound() error = %v", err)
	}
	newRequest := awaitRouteRequest(t, requests)
	if newRequest.SessionID != "session-new" || newRequest.Query != "new message" {
		t.Fatalf("new generation request = %#v", newRequest)
	}
	select {
	case _, open := <-oldRequest.InjectionFeed.Messages:
		if open {
			t.Fatal("new generation was delivered to old injection feed")
		}
	default:
		t.Fatal("old injection feed remained open after session reset")
	}

	close(releaseOld)
	if err := <-oldDone; err != nil {
		t.Fatalf("old HandleInbound() error = %v", err)
	}
	select {
	case stale := <-requests:
		t.Fatalf("old queued work was promoted after session reset: %#v", stale)
	default:
	}
}

func TestDirectSkillAutoCreatedSessionAdoptsReservedRouteScope(t *testing.T) {
	identityService := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: testChannelIdentityUUID}}
	chatService := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	ensurer := &statefulSessionEnsurer{}
	requests := make(chan conversation.ChatRequest, 1)
	releaseSkill := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseSkill:
		default:
			close(releaseSkill)
		}
	})
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{
			Role: "assistant", Content: conversation.NewTextContent("done"),
		}}},
		onChat: func(req conversation.ChatRequest) {
			requests <- req
			<-releaseSkill
		},
	}
	processor := NewChannelInboundProcessor(
		slog.New(slog.DiscardHandler), nil, chatService, chatService, gateway,
		identityService, &fakePolicyService{}, "", 0,
	)
	processor.SetACLService(&fakeChatACL{allowed: true})
	processor.SetSessionEnsurer(ensurer)
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.AdvanceScope("route-1", "session-old")
	processor.SetDispatcher(dispatcher)
	processor.SetRequestedSkillResolver(&fakeRequestedSkillResolver{items: []skillset.ResolvedSkill{{
		Name: "alpha", Content: "alpha skill content",
	}}})
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}
	baseMessage := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelTypeTelegram,
		ReplyTarget: "telegram:user-1",
		Sender:      channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
		Conversation: channel.Conversation{
			ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat",
		},
	}

	skillDone := make(chan error, 1)
	go func() {
		message := baseMessage
		message.Message = channel.Message{ID: "external-skill", Text: "/alpha first"}
		skillDone <- processor.HandleInbound(context.Background(), cfg, message, sender)
	}()
	skillRequest := awaitRouteRequest(t, requests)
	if skillRequest.SessionID != "created-session" {
		t.Fatalf("skill session = %q, want created-session", skillRequest.SessionID)
	}
	second := baseMessage
	second.Message = channel.Message{ID: "external-second", Text: "second"}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- processor.HandleInbound(context.Background(), cfg, second, sender)
	}()
	select {
	case injected := <-skillRequest.InjectionFeed.Messages:
		if injected.Text != "second" {
			t.Fatalf("injected text = %q, want second", injected.Text)
		}
	case unexpected := <-requests:
		t.Fatalf("same-session message started a separate stream: %#v", unexpected)
	case <-time.After(time.Second):
		t.Fatal("same-session message did not inject into reserved skill stream")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second HandleInbound() error = %v", err)
	}

	close(releaseSkill)
	if err := <-skillDone; err != nil {
		t.Fatalf("skill HandleInbound() error = %v", err)
	}
}

type statefulSessionEnsurer struct {
	mu     sync.Mutex
	active SessionResult
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
	session := SessionResult{ID: fmt.Sprintf("session-%d", call), Type: session.TypeChat, Runtime: session.RuntimeModel}
	e.active = session
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
	return session, nil
}

type misdirectedSessionEnsurer struct {
	active SessionResult
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

func (e *misdirectedSessionEnsurer) EnsureActiveSession(context.Context, string, string, string) (SessionResult, error) {
	return e.active, nil
}

func (e *misdirectedSessionEnsurer) GetActiveSession(context.Context, string) (SessionResult, error) {
	return e.active, nil
}

func (*misdirectedSessionEnsurer) CreateNewSession(context.Context, string, string, string, NewSessionSpec) (SessionResult, error) {
	return SessionResult{ID: "session-not-active", Type: session.TypeChat}, nil
}

func (e *statefulSessionEnsurer) EnsureActiveSession(ctx context.Context, _, routeID, _ string) (SessionResult, error) {
	return e.GetActiveSession(ctx, routeID)
}

func (e *statefulSessionEnsurer) GetActiveSession(_ context.Context, _ string) (SessionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if strings.TrimSpace(e.active.ID) == "" {
		return SessionResult{}, errors.New("no active session")
	}
	return e.active, nil
}

func (e *statefulSessionEnsurer) CreateNewSession(_ context.Context, _, _, _ string, spec NewSessionSpec) (SessionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = SessionResult{
		ID:                    "created-session",
		Type:                  spec.Type,
		Mode:                  spec.Mode,
		Runtime:               spec.Runtime,
		RuntimeOwnerAccountID: spec.RuntimeOwnerAccountID,
	}
	return e.active, nil
}
