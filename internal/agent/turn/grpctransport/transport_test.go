package grpctransport

import (
	"context"
	"encoding/json"
	"net"
	"runtime"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/agent/turn/turnpb"
	intrpc "github.com/memohai/memoh/internal/rpc"
	"github.com/memohai/memoh/internal/userinput"
)

func TestStartTurnRoundTrip(t *testing.T) {
	fake := &fakeService{}
	client, cleanup := newTestClient(t, fake, "secret")
	defer cleanup()

	handle, err := client.StartTurn(context.Background(), turn.StartTurnCommand{
		SchemaVersion: 1,
		TeamID:        "team-1",
		BotID:         "bot-1",
		SessionID:     "session-1",
		Query:         "hello",
		Attachments:   []turn.Attachment{{Name: "a.txt", ContentHash: "hash-1"}},
	})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	event := <-handle.Events()
	if event.RunID != "run-1" || event.Seq != 1 || string(event.Payload) != `{"type":"text_delta","text":"hi"}` {
		t.Fatalf("event = %#v", event)
	}
	if fake.started.Query != "hello" || fake.started.Attachments[0].ContentHash != "hash-1" {
		t.Fatalf("command = %#v", fake.started)
	}
}

func TestAuthenticationRejectsWrongSecret(t *testing.T) {
	fake := &fakeService{}
	client, cleanup := newTestClient(t, fake, "wrong")
	defer cleanup()
	_, err := client.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "team-1"})
	if err == nil {
		t.Fatal("expected authentication failure")
	}
}

func TestCancelUnblocksFullClientEventBuffer(t *testing.T) {
	fake := &fakeService{eventCount: 40, cancelled: make(chan struct{}, 1)}
	client, cleanup := newTestClient(t, fake, "secret")
	defer cleanup()

	handle, err := client.StartTurn(context.Background(), turn.StartTurnCommand{TeamID: "team-1"})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	h := handle.(*runHandle)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for len(h.events) < cap(h.events) {
		select {
		case <-deadline.C:
			t.Fatal("client event buffer did not fill")
		default:
			runtime.Gosched()
		}
	}

	handle.Cancel()
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatal("client pump remained blocked after Cancel")
	}
	select {
	case <-fake.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("server run was not canceled")
	}
}

func newTestClient(t *testing.T, service turn.Service, clientSecret string) (*Client, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	server := intrpc.NewServer("secret")
	turnpb.RegisterTurnServiceServer(server, NewServer(nil, service))
	go func() { _ = server.Serve(lis) }()
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(intrpc.UnaryClientAuth(clientSecret)),
		grpc.WithStreamInterceptor(intrpc.StreamClientAuth(clientSecret)),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return NewClient(conn), func() { _ = conn.Close(); server.Stop(); _ = lis.Close() }
}

type fakeService struct {
	started    turn.StartTurnCommand
	eventCount int
	cancelled  chan struct{}
}

func (f *fakeService) StartTurn(_ context.Context, cmd turn.StartTurnCommand) (turn.RunHandle, error) {
	f.started = cmd
	eventCount := max(f.eventCount, 1)
	events := make(chan turn.Event, eventCount)
	for i := range eventCount {
		events <- turn.Event{RunID: "run-1", TeamID: cmd.TeamID, SessionID: cmd.SessionID, Seq: int64(i + 1), Kind: "text_delta", Payload: json.RawMessage(`{"type":"text_delta","text":"hi"}`)}
	}
	close(events)
	errs := make(chan error)
	close(errs)
	return &fakeHandle{events: events, errs: errs, cancelled: f.cancelled}, nil
}

func (*fakeService) RespondToolApproval(context.Context, turn.ToolApprovalResponse, chan<- json.RawMessage) error {
	return nil
}

func (*fakeService) RespondUserInput(context.Context, turn.UserInputResponse, chan<- json.RawMessage) error {
	return nil
}

func (*fakeService) AdvancePlainTextUserInput(context.Context, userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	return userinput.AdvanceTextResult{}, nil
}

type fakeHandle struct {
	events    <-chan turn.Event
	errs      <-chan error
	cancelled chan<- struct{}
}

func (*fakeHandle) RunID() string                                    { return "run-1" }
func (h *fakeHandle) Events() <-chan turn.Event                      { return h.events }
func (h *fakeHandle) Errs() <-chan error                             { return h.errs }
func (*fakeHandle) Inject(context.Context, turn.InjectMessage) error { return nil }
func (*fakeHandle) AddOutboundAssets([]turn.OutboundAssetRef)        {}
func (h *fakeHandle) Cancel() {
	if h.cancelled == nil {
		return
	}
	select {
	case h.cancelled <- struct{}{}:
	default:
	}
}
