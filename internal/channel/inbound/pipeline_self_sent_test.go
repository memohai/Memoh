package inbound

import (
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

func TestPipelineInboundMessageMarksConfiguredSelfIdentity(t *testing.T) {
	t.Parallel()

	for _, cfg := range []channel.ChannelConfig{
		{ExternalIdentity: "open_id:bot-open-id"},
		{SelfIdentity: map[string]any{"user_id": "bot-open-id"}},
	} {
		msg := channel.InboundMessage{
			Sender: channel.Identity{SubjectID: "bot-open-id"},
		}
		got := pipelineInboundMessage(cfg, msg)
		if !got.IsSelfSent {
			t.Fatalf("self-sent marker missing for config %#v", cfg)
		}
	}
}

func TestPipelineInboundMessageLeavesExternalSenderUnmarked(t *testing.T) {
	t.Parallel()

	msg := channel.InboundMessage{
		Sender: channel.Identity{SubjectID: "human-user"},
	}
	got := pipelineInboundMessage(channel.ChannelConfig{
		ExternalIdentity: "bot-user",
		SelfIdentity:     map[string]any{"user_id": "bot-user"},
	}, msg)
	if got.IsSelfSent {
		t.Fatal("external sender was marked as self-sent")
	}
}

func TestPipelineInboundMessageDoesNotTreatChatIdentityAsSelf(t *testing.T) {
	t.Parallel()

	got := pipelineInboundMessage(channel.ChannelConfig{ExternalIdentity: "chat_id:shared-id"}, channel.InboundMessage{
		Sender: channel.Identity{SubjectID: "shared-id"},
	})
	if got.IsSelfSent {
		t.Fatal("chat identity was treated as the bot's sender identity")
	}
}

func TestSelfSentDirectMessageDoesNotTriggerOrCountAsDirected(t *testing.T) {
	t.Parallel()

	msg := pipelineInboundMessage(channel.ChannelConfig{ExternalIdentity: "bot-id"}, channel.InboundMessage{
		Sender:       channel.Identity{SubjectID: "bot-id"},
		Conversation: channel.Conversation{Type: channel.ConversationTypePrivate},
	})

	if shouldTriggerAssistantResponse(msg) {
		t.Fatal("self-sent direct message triggered an assistant response")
	}
	if isDirectedAtBot(msg) {
		t.Fatal("self-sent direct message was treated as directed at the bot")
	}
}

func TestPipelineSessionLockSerializesTheSameSession(t *testing.T) {
	t.Parallel()

	processor := &ChannelInboundProcessor{}
	unlockFirst := processor.lockPipelineSession("session")
	started := make(chan struct{})
	acquired := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		unlockSecond := processor.lockPipelineSession("session")
		close(acquired)
		unlockSecond()
		close(done)
	}()
	<-started
	select {
	case <-acquired:
		t.Fatal("second inbound acquired the same session lock before the first released it")
	default:
	}

	unlockFirst()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second inbound did not acquire the session lock after release")
	}
	<-done
	if got := processor.pipelineSessionLocks.len(); got != 0 {
		t.Fatalf("retained pipeline session locks = %d, want 0", got)
	}
}
