package inbound

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestPrepareDeferredTurnOwnsMutableInput(t *testing.T) {
	t.Parallel()

	cfg := channel.ChannelConfig{Credentials: map[string]any{"nested": map[string]any{"value": "original"}}}
	msg := channel.InboundMessage{
		Channel: channel.ChannelTypeTelegram,
		Message: channel.Message{
			Text: "hello",
			Attachments: []channel.Attachment{{
				ContentHash: "asset-1",
				Metadata:    map[string]any{"nested": map[string]any{"value": "original"}},
			}},
		},
		Metadata: map[string]any{"nested": map[string]any{"value": "original"}},
	}
	attachments := []conversation.ChatAttachment{{
		ContentHash: "asset-1",
		Metadata:    map[string]any{"nested": map[string]any{"value": "original"}},
	}}
	activation := &conversation.SkillActivation{
		Prompt: "hello",
		Skills: []conversation.SkillActivationSkill{{Name: "alpha"}},
	}
	receipt := conversation.UserMessageReceipt{
		ID:          "receipt-1",
		Metadata:    map[string]any{"nested": map[string]any{"value": "original"}},
		Attachments: attachments,
	}
	turn, err := prepareDeferredTurn(deferredTurn{
		ctx:             context.Background(),
		cfg:             cfg,
		msg:             msg,
		attachments:     attachments,
		skillActivation: activation,
		receipt:         receipt,
	})
	if err != nil {
		t.Fatalf("prepareDeferredTurn() error = %v", err)
	}

	cfg.Credentials["nested"].(map[string]any)["value"] = "mutated"
	msg.Metadata["nested"].(map[string]any)["value"] = "mutated"
	msg.Message.Attachments[0].Metadata["nested"].(map[string]any)["value"] = "mutated"
	attachments[0].Metadata["nested"].(map[string]any)["value"] = "mutated"
	activation.Skills[0].Name = "mutated"
	receipt.Metadata["nested"].(map[string]any)["value"] = "mutated"

	if turn.cfg.Credentials["nested"].(map[string]any)["value"] != "original" ||
		turn.msg.Metadata["nested"].(map[string]any)["value"] != "original" ||
		turn.resolvedAttachments[0].Metadata["nested"].(map[string]any)["value"] != "original" ||
		turn.attachments[0].Metadata["nested"].(map[string]any)["value"] != "original" ||
		turn.skillActivation.Skills[0].Name != "alpha" ||
		turn.receipt.Metadata["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("deferred turn aliased input: %#v", turn)
	}
}

func TestPrepareDeferredTurnRejectsUncloneableInput(t *testing.T) {
	t.Parallel()

	if _, err := prepareDeferredTurn(deferredTurn{
		ctx: context.Background(),
		msg: channel.InboundMessage{Metadata: map[string]any{
			"invalid": make(chan struct{}),
		}},
	}); err == nil {
		t.Fatal("uncloneable deferred input was accepted")
	}
}

func TestDeferredTurnActivateOncePushesPipelineOnce(t *testing.T) {
	t.Parallel()

	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	processor := &ChannelInboundProcessor{pipeline: pipeline, logger: slog.New(slog.DiscardHandler)}
	turn, err := prepareDeferredTurn(deferredTurn{
		ctx:       context.Background(),
		sessionID: "session-1",
		identity: InboundIdentity{
			BotID: "bot-1", ChannelIdentityID: testChannelIdentityUUID, DisplayName: "Alice",
		},
		msg: channel.InboundMessage{
			Channel: channel.ChannelTypeTelegram,
			Message: channel.Message{ID: "external-1", Text: "hello"},
			Conversation: channel.Conversation{
				ID: "chat-1", Type: channel.ConversationTypePrivate, Name: "Alice Chat",
			},
		},
		receipt: conversation.UserMessageReceipt{ID: "receipt-1"},
	})
	if err != nil {
		t.Fatalf("prepareDeferredTurn() error = %v", err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			turn.ActivateOnce(context.Background(), processor)
		}()
	}
	close(start)
	wait.Wait()

	ic, ok := pipeline.GetIC("session-1")
	if !ok || len(ic.Nodes) != 1 {
		t.Fatalf("pipeline activation = loaded:%v context:%#v", ok, ic)
	}
}
