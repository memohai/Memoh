package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func TestDiscussDriverIdleRetirementProcessesQueuedNotification(t *testing.T) {
	started := make(chan struct{}, 1)
	idleReached := make(chan struct{}, 1)
	releaseIdle := make(chan struct{})
	logger := slog.New(&sessionLifecycleSignalHandler{
		started:     started,
		idleReached: idleReached,
		releaseIdle: releaseIdle,
	})
	runtime := &recordingDiscussRuntimeStreamer{calls: make(chan conversation.ChatRequest, 1)}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Resolver: &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{
			RuntimeType: sessionpkg.RuntimeACPAgent,
		}},
		RuntimeStreamer: runtime,
		Logger:          logger,
	})
	driver.idleTimeout = 20 * time.Millisecond
	config := DiscussSessionConfig{
		BotID:            "bot-1",
		SessionID:        "session-1",
		ConversationType: "group",
	}
	driver.NotifyRC(context.Background(), config.SessionID, nil, config)
	t.Cleanup(func() { driver.StopSession(config.SessionID) })

	waitForSessionLifecycleSignal(t, started, "session start")
	waitForSessionLifecycleSignal(t, idleReached, "idle timeout")
	driver.NotifyRC(context.Background(), config.SessionID, RenderedContext{{
		LastEventCursor: 10,
		MentionsMe:      true,
		Content: []RenderedContentPiece{{
			Type: "text",
			Text: `<message id="message-1">hello</message>`,
		}},
	}}, config)
	close(releaseIdle)

	select {
	case <-runtime.calls:
	case <-time.After(time.Second):
		t.Fatal("notification queued during idle retirement was not processed")
	}
}

func TestDiscussDriverStopSessionWinsDuringIdleRetirement(t *testing.T) {
	started := make(chan struct{}, 1)
	idleReached := make(chan struct{}, 1)
	stopped := make(chan struct{}, 1)
	releaseIdle := make(chan struct{})
	logger := slog.New(&sessionLifecycleSignalHandler{
		started:     started,
		idleReached: idleReached,
		stopped:     stopped,
		releaseIdle: releaseIdle,
	})
	driver := NewDiscussDriver(DiscussDriverDeps{Logger: logger})
	driver.idleTimeout = 20 * time.Millisecond
	config := DiscussSessionConfig{BotID: "bot-1", SessionID: "session-1"}
	driver.NotifyRC(context.Background(), config.SessionID, nil, config)

	waitForSessionLifecycleSignal(t, started, "session start")
	waitForSessionLifecycleSignal(t, idleReached, "idle timeout")
	driver.StopSession(config.SessionID)
	if driver.HasSession(config.SessionID) {
		close(releaseIdle)
		t.Fatal("StopSession left the idle discuss session registered")
	}
	close(releaseIdle)
	waitForSessionLifecycleSignal(t, stopped, "session stop")
}

func waitForSessionLifecycleSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

type recordingDiscussRuntimeStreamer struct {
	calls chan conversation.ChatRequest
}

func (r *recordingDiscussRuntimeStreamer) StreamChat(
	_ context.Context,
	req conversation.ChatRequest,
) (<-chan conversation.StreamChunk, <-chan error) {
	r.calls <- req
	chunks := make(chan conversation.StreamChunk, 1)
	chunks <- conversation.StreamChunk(`{"type":"agent_end"}`)
	close(chunks)
	errs := make(chan error)
	close(errs)
	return chunks, errs
}

type sessionLifecycleSignalHandler struct {
	started     chan<- struct{}
	idleReached chan<- struct{}
	stopped     chan<- struct{}
	releaseIdle <-chan struct{}
}

func (*sessionLifecycleSignalHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *sessionLifecycleSignalHandler) Handle(_ context.Context, record slog.Record) error {
	switch {
	case record.Message == "discuss session started":
		signalSessionLifecycle(h.started)
	case strings.Contains(record.Message, "discuss session idle timeout"):
		signalSessionLifecycle(h.idleReached)
		if h.releaseIdle != nil {
			<-h.releaseIdle
		}
	case record.Message == "discuss session stopped":
		signalSessionLifecycle(h.stopped)
	}
	return nil
}

func (h *sessionLifecycleSignalHandler) WithAttrs([]slog.Attr) slog.Handler {
	return &sessionLifecycleSignalHandler{
		started:     h.started,
		idleReached: h.idleReached,
		stopped:     h.stopped,
		releaseIdle: h.releaseIdle,
	}
}

func (h *sessionLifecycleSignalHandler) WithGroup(string) slog.Handler {
	return &sessionLifecycleSignalHandler{
		started:     h.started,
		idleReached: h.idleReached,
		stopped:     h.stopped,
		releaseIdle: h.releaseIdle,
	}
}

func signalSessionLifecycle(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
