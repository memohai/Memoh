package grpctransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/agent/turn/turnpb"
	"github.com/memohai/memoh/internal/userinput"
)

type Client struct {
	client turnpb.TurnServiceClient
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: turnpb.NewTurnServiceClient(conn)}
}

func (c *Client) StartTurn(ctx context.Context, cmd turn.StartTurnCommand) (turn.RunHandle, error) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	stream, err := c.client.Run(ctx)
	if err != nil {
		return nil, mapClientError(err)
	}
	if err := stream.Send(&turnpb.RunRequest{Body: &turnpb.RunRequest_StartJson{StartJson: data}}); err != nil {
		return nil, mapClientError(err)
	}
	first, err := stream.Recv()
	if err != nil {
		return nil, mapClientError(err)
	}
	started := first.GetStarted()
	if started == nil || started.GetRunId() == "" {
		return nil, errors.New("turn rpc: missing started frame")
	}
	h := &runHandle{
		id: started.GetRunId(), stream: stream,
		events: make(chan turn.Event, 16), errs: make(chan error, 1),
	}
	go h.pump()
	return h, nil
}

func (c *Client) RespondToolApproval(ctx context.Context, input turn.ToolApprovalResponse, eventCh chan<- json.RawMessage) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	stream, err := c.client.RespondToolApproval(ctx, &turnpb.JsonRequest{Json: data})
	if err != nil {
		return mapClientError(err)
	}
	return receiveContinuation(stream.Recv, eventCh)
}

func (c *Client) RespondUserInput(ctx context.Context, input turn.UserInputResponse, eventCh chan<- json.RawMessage) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	stream, err := c.client.RespondUserInput(ctx, &turnpb.JsonRequest{Json: data})
	if err != nil {
		return mapClientError(err)
	}
	return receiveContinuation(stream.Recv, eventCh)
}

func (c *Client) AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return userinput.AdvanceTextResult{}, err
	}
	resp, err := c.client.AdvancePlainTextUserInput(ctx, &turnpb.JsonRequest{Json: data})
	if err != nil {
		return userinput.AdvanceTextResult{}, mapClientError(err)
	}
	var result userinput.AdvanceTextResult
	if err := json.Unmarshal(resp.GetJson(), &result); err != nil {
		return userinput.AdvanceTextResult{}, err
	}
	return result, nil
}

type continuationReceiver func() (*turnpb.EventResponse, error)

func receiveContinuation(recv continuationReceiver, eventCh chan<- json.RawMessage) error {
	for {
		event, err := recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return mapClientError(err)
		}
		eventCh <- json.RawMessage(event.GetPayload())
	}
}

type runHandle struct {
	id     string
	stream turnpb.TurnService_RunClient
	events chan turn.Event
	errs   chan error
	mu     sync.Mutex
	once   sync.Once
}

func (h *runHandle) RunID() string             { return h.id }
func (h *runHandle) Events() <-chan turn.Event { return h.events }
func (h *runHandle) Errs() <-chan error        { return h.errs }

func (h *runHandle) Inject(ctx context.Context, msg turn.InjectMessage) error {
	data, err := json.Marshal(injectPayload{
		Text:            msg.Text,
		Attachments:     msg.Attachments,
		HeaderifiedText: msg.HeaderifiedText,
	})
	if err != nil {
		return err
	}
	return h.send(ctx, &turnpb.RunRequest{Body: &turnpb.RunRequest_InjectJson{InjectJson: data}})
}

func (h *runHandle) AddOutboundAssets(refs []turn.OutboundAssetRef) {
	data, err := json.Marshal(refs)
	if err != nil {
		return
	}
	_ = h.send(context.Background(), &turnpb.RunRequest{Body: &turnpb.RunRequest_OutboundAssetsJson{OutboundAssetsJson: data}})
}

func (h *runHandle) Cancel() {
	h.once.Do(func() {
		_ = h.send(context.Background(), &turnpb.RunRequest{Body: &turnpb.RunRequest_Cancel{Cancel: true}})
		_ = h.stream.CloseSend()
	})
}

func (h *runHandle) send(ctx context.Context, frame *turnpb.RunRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return mapClientError(h.stream.Send(frame))
	}
}

func (h *runHandle) pump() {
	defer close(h.events)
	defer close(h.errs)
	for {
		frame, err := h.stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			h.errs <- mapClientError(err)
			return
		}
		if frame.GetCompleted() != nil {
			return
		}
		event := frame.GetEvent()
		if event == nil {
			continue
		}
		h.events <- turn.Event{
			RunID: event.GetRunId(), TeamID: event.GetTeamId(), SessionID: event.GetSessionId(),
			Seq: event.GetSeq(), Kind: event.GetKind(), Payload: json.RawMessage(event.GetPayload()),
		}
	}
}

func mapClientError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.AlreadyExists:
		return turn.ErrDuplicateTurn
	case codes.PermissionDenied:
		return turn.ErrTeamNotServed
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	default:
		return err
	}
}
