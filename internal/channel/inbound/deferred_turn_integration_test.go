package inbound

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/session"
)

func TestQueuedDeferredDiscussTurnActivatesOnceWithoutReenteringIngress(t *testing.T) {
	t.Parallel()

	identityService := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: testChannelIdentityUUID}}
	chatService := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{resp: conversation.ChatResponse{Messages: []conversation.ModelMessage{{
		Role: "assistant", Content: conversation.NewTextContent("done"),
	}}}}
	processor := NewChannelInboundProcessor(
		slog.New(slog.DiscardHandler), nil, chatService, chatService, gateway,
		identityService, &fakePolicyService{}, "", 0,
	)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID: "session-1", Type: session.TypeDiscuss, Runtime: session.RuntimeModel,
	}})
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	processor.SetPipeline(pipeline, nil, nil)
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	dispatcher.MarkActive("route-1")
	processor.SetDispatcher(dispatcher)
	sender := &fakeReplySender{}
	message := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelTypeTelegram,
		Message:     channel.Message{ID: "external-1", Text: "/next queued"},
		ReplyTarget: "telegram:user-1",
		Sender:      channel.Identity{SubjectID: "external-user", DisplayName: "Alice"},
		Conversation: channel.Conversation{
			ID: "user-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat",
		},
		Metadata: map[string]any{"model_id": "model-original"},
	}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}

	if err := processor.HandleInbound(context.Background(), cfg, message, sender); err != nil {
		t.Fatalf("queue HandleInbound() error = %v", err)
	}
	if _, loaded := pipeline.GetIC("session-1"); loaded {
		t.Fatal("queued turn activated pipeline before promotion")
	}
	if identityService.calls != 1 {
		t.Fatalf("identity resolutions before promotion = %d", identityService.calls)
	}
	message.Metadata["model_id"] = "mutated"

	processor.drainQueue(context.Background(), "route-1")
	ic, loaded := pipeline.GetIC("session-1")
	if !loaded || len(ic.Nodes) != 1 {
		t.Fatalf("pipeline after promotion = loaded:%v context:%#v", loaded, ic)
	}
	if identityService.calls != 1 {
		t.Fatalf("queued promotion reentered identity resolution %d times", identityService.calls)
	}
	if gateway.gotReq.Query != "queued" || gateway.gotReq.Model != "model-original" {
		t.Fatalf("promoted request = %#v", gateway.gotReq)
	}
}
