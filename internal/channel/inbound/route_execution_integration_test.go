package inbound

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/session"
	skillset "github.com/memohai/memoh/internal/skills"
)

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
		Channel: channel.ChannelTypeTelegram, ReplyTarget: "telegram:user-1",
		Conversation: channel.Conversation{ID: "user-1", Type: channel.ConversationTypePrivate},
	}
	if err := processor.handleStopCommand(context.Background(), channel.ChannelConfig{}, message, &fakeReplySender{}, InboundIdentity{BotID: "bot-1"}); err != nil {
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
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{ID: "session-old", Type: session.TypeChat, Runtime: session.RuntimeModel}}
	requests := make(chan conversation.ChatRequest, 3)
	releaseOld := make(chan struct{})
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("done")}}},
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
		BotID: "bot-1", Channel: channel.ChannelTypeTelegram, ReplyTarget: "telegram:user-1",
		Sender:       channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
		Conversation: channel.Conversation{ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat"},
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
		resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("done")}}},
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
	processor.SetRequestedSkillResolver(&fakeRequestedSkillResolver{items: []skillset.ResolvedSkill{{Name: "alpha", Content: "alpha skill content"}}})
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}
	baseMessage := channel.InboundMessage{
		BotID: "bot-1", Channel: channel.ChannelTypeTelegram, ReplyTarget: "telegram:user-1",
		Sender:       channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
		Conversation: channel.Conversation{ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat"},
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
	go func() { secondDone <- processor.HandleInbound(context.Background(), cfg, second, sender) }()
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
