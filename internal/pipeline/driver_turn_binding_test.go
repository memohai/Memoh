package pipeline

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

func TestDiscussReplyKeepsTriggerMessageBindingWhenNextTurnArrives(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stored := make(chan string, 1)
	resolver := &turnBindingResolver{
		fakeRunConfigResolver: fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{ModelID: "model-1"}},
		storedRequestID:       stored,
	}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		PersistedUserMessageID: "user-message-b",
	}}
	rc := RenderedContext{{
		EventCursor: 10,
		Content:     []RenderedContentPiece{{Type: "text", Text: "turn a"}},
	}}
	agent := &gatedDiscussStreamer{started: started, release: release}

	done := make(chan struct{})
	go func() {
		driver.handleReplyWithAgentConfig(
			context.Background(),
			sess,
			rc,
			DiscussSessionConfig{
				BotID:                  "bot",
				SessionID:              "session",
				PersistedUserMessageID: "user-message-a",
			},
			driver.logger,
			agent,
		)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first turn to start")
	}
	driver.mu.Lock()
	sess.config.PersistedUserMessageID = "user-message-b"
	driver.mu.Unlock()
	close(release)

	select {
	case got := <-stored:
		if got != "user-message-a" {
			t.Fatalf("stored turn request message id = %q, want first turn trigger", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first turn to persist")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first turn to finish")
	}
}

type turnBindingResolver struct {
	fakeRunConfigResolver
	storedRequestID chan<- string
}

func (r *turnBindingResolver) StoreRound(
	_ context.Context,
	_, _, _, _, persistedUserMessageID string,
	_ []sdk.Message,
	_ string,
) error {
	r.storedRequestID <- persistedUserMessageID
	return nil
}

func (r *turnBindingResolver) StoreRoundWithCursor(
	_ context.Context,
	_, _, _, _, persistedUserMessageID string,
	_ []sdk.Message,
	_ string,
	_ DiscussCursorCommit,
) (bool, error) {
	r.storedRequestID <- persistedUserMessageID
	return true, nil
}

type gatedDiscussStreamer struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (s *gatedDiscussStreamer) Stream(_ context.Context, _ agentpkg.RunConfig) <-chan agentpkg.StreamEvent {
	close(s.started)
	<-s.release
	messages, _ := json.Marshal([]sdk.Message{sdk.AssistantMessage("reply to a")})
	events := make(chan agentpkg.StreamEvent, 1)
	events <- agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, Messages: messages}
	close(events)
	return events
}
