package inbound

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func newTurnBindingProcessor(sessionID, sessionType string) (*ChannelInboundProcessor, *fakeChatService) {
	chatSvc := &fakeChatService{
		resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"},
		persistedID:   "persisted-user-message",
	}
	processor := NewChannelInboundProcessor(
		slog.Default(),
		nil,
		chatSvc,
		chatSvc,
		&fakeChatGateway{},
		&fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channel-identity"}},
		&fakePolicyService{},
		"",
		0,
	)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      sessionID,
		Type:    sessionType,
		Runtime: sessionpkg.RuntimeModel,
	}})
	return processor, chatSvc
}

func setFixedTranscription(processor *ChannelInboundProcessor, text string) {
	processor.mediaService = &fakeMediaIngestor{}
	processor.sttModelResolver = fixedTranscriptionModelResolver("stt-model")
	processor.transcriber = fixedTranscriber(text)
}

func turnBindingConfig() channel.ChannelConfig {
	return channel.ChannelConfig{ID: "config", BotID: "bot", ChannelType: channel.ChannelType("test")}
}

func turnBindingTextMessage() channel.InboundMessage {
	return channel.InboundMessage{
		BotID:       "bot",
		Channel:     channel.ChannelType("test"),
		Message:     channel.Message{ID: "external-message", Text: "hello"},
		ReplyTarget: "target",
		Sender:      channel.Identity{SubjectID: "sender", DisplayName: "User"},
		Conversation: channel.Conversation{
			ID:   "conversation",
			Type: channel.ConversationTypePrivate,
		},
	}
}

func turnBindingVoiceMessage() channel.InboundMessage {
	msg := turnBindingTextMessage()
	msg.Message.Text = ""
	msg.Message.Attachments = []channel.Attachment{{Type: channel.AttachmentVoice, ContentHash: "voice-hash", Mime: "audio/ogg", Name: "voice.ogg"}}
	return msg
}

func TestDiscussInboundPersistsTriggerBeforeNotification(t *testing.T) {
	processor, chatSvc := newTurnBindingProcessor("discuss-session", sessionpkg.TypeDiscuss)
	processor.SetPipeline(pipelinepkg.NewPipeline(pipelinepkg.RenderParams{}), nil, nil)
	notifier := &recordingDiscussNotifier{messages: chatSvc}
	processor.discussDriver = notifier

	err := processor.HandleInbound(context.Background(), turnBindingConfig(), turnBindingTextMessage(), &fakeReplySender{})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if !notifier.called {
		t.Fatal("discuss driver was not notified")
	}
	if notifier.persistedAtNotify != 1 {
		t.Fatalf("persisted messages at notification = %d, want 1", notifier.persistedAtNotify)
	}
	if got := notifier.config.PersistedUserMessageID; got != "persisted-user-message" {
		t.Fatalf("notification user message id = %q, want persisted message id", got)
	}
	if len(chatSvc.persisted) != 1 {
		t.Fatalf("persisted messages = %d, want exactly 1", len(chatSvc.persisted))
	}
}

func TestDiscussInboundTranscribesVoiceBeforePersistenceAndNotification(t *testing.T) {
	processor, chatSvc := newTurnBindingProcessor("discuss-session", sessionpkg.TypeDiscuss)
	processor.SetPipeline(pipelinepkg.NewPipeline(pipelinepkg.RenderParams{}), nil, nil)
	setFixedTranscription(processor, "ship the fix")
	notifier := &recordingDiscussNotifier{messages: chatSvc}
	processor.discussDriver = notifier

	err := processor.HandleInbound(context.Background(), turnBindingConfig(), turnBindingVoiceMessage(), &fakeReplySender{})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if !strings.Contains(pipelinepkg.RCToXML(notifier.rc), "ship the fix") {
		t.Fatalf("rendered context = %q, want voice transcript", pipelinepkg.RCToXML(notifier.rc))
	}
	if len(chatSvc.persistedIn) != 1 || !strings.Contains(chatSvc.persistedIn[0].DisplayText, "ship the fix") {
		t.Fatalf("persisted input = %#v, want voice transcript", chatSvc.persistedIn)
	}
}

func TestChatInboundTranscribesVoiceBeforePipelineProjection(t *testing.T) {
	processor, _ := newTurnBindingProcessor("chat-session", sessionpkg.TypeChat)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	processor.SetPipeline(pipeline, nil, nil)
	setFixedTranscription(processor, "ship the fix")

	err := processor.HandleInbound(context.Background(), turnBindingConfig(), turnBindingVoiceMessage(), &fakeReplySender{})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if got := pipelinepkg.RCToXML(pipeline.GetRC("chat-session")); !strings.Contains(got, "ship the fix") {
		t.Fatalf("rendered context = %q, want voice transcript", got)
	}
}

type recordingDiscussNotifier struct {
	messages          *fakeChatService
	called            bool
	persistedAtNotify int
	config            pipelinepkg.DiscussSessionConfig
	rc                pipelinepkg.RenderedContext
}

func (n *recordingDiscussNotifier) NotifyRC(
	_ context.Context,
	_ string,
	rc pipelinepkg.RenderedContext,
	config pipelinepkg.DiscussSessionConfig,
) {
	n.called = true
	n.persistedAtNotify = len(n.messages.persisted)
	n.config = config
	n.rc = rc
}

type fixedTranscriptionModelResolver string

func (r fixedTranscriptionModelResolver) ResolveTranscriptionModelID(context.Context, string) (string, error) {
	return string(r), nil
}

type fixedTranscriber string

func (t fixedTranscriber) Transcribe(context.Context, string, []byte, string, string, map[string]any) (TranscriptionResult, error) {
	return fixedTranscriptionResult(t), nil
}

type fixedTranscriptionResult string

func (r fixedTranscriptionResult) GetText() string {
	return string(r)
}
