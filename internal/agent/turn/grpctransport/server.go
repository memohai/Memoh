package grpctransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/agent/turn/turnpb"
	"github.com/memohai/memoh/internal/userinput"
)

type Server struct {
	turnpb.UnimplementedTurnServiceServer
	service turn.Service
	logger  *slog.Logger
}

// injectPayload is the cross-process portion of turn.InjectMessage. Applied
// is an in-process callback and is intentionally not transported.
type injectPayload struct {
	Text            string
	Attachments     []turn.Attachment
	HeaderifiedText string
}

func NewServer(log *slog.Logger, service turn.Service) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{service: service, logger: log.With(slog.String("component", "turn_rpc"))}
}

func (s *Server) Run(stream turnpb.TurnService_RunServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	startJSON := first.GetStartJson()
	if len(startJSON) == 0 {
		return status.Error(codes.InvalidArgument, "first frame must start a turn")
	}
	var cmd turn.StartTurnCommand
	if err := json.Unmarshal(startJSON, &cmd); err != nil {
		return status.Error(codes.InvalidArgument, "invalid start turn payload")
	}
	handle, err := s.service.StartTurn(stream.Context(), cmd)
	if err != nil {
		return s.mapError("start turn", err)
	}
	defer handle.Cancel()
	if err := stream.Send(&turnpb.RunResponse{Body: &turnpb.RunResponse_Started{Started: &turnpb.Started{RunId: handle.RunID()}}}); err != nil {
		return err
	}

	controlErr := make(chan error, 1)
	go func() {
		for {
			frame, recvErr := stream.Recv()
			if recvErr != nil {
				controlErr <- recvErr
				return
			}
			switch body := frame.Body.(type) {
			case *turnpb.RunRequest_InjectJson:
				var payload injectPayload
				if err := json.Unmarshal(body.InjectJson, &payload); err != nil {
					controlErr <- status.Error(codes.InvalidArgument, "invalid inject payload")
					return
				}
				msg := turn.InjectMessage{
					Text:            payload.Text,
					Attachments:     payload.Attachments,
					HeaderifiedText: payload.HeaderifiedText,
				}
				if err := handle.Inject(stream.Context(), msg); err != nil {
					controlErr <- s.mapError("inject turn", err)
					return
				}
			case *turnpb.RunRequest_OutboundAssetsJson:
				var refs []turn.OutboundAssetRef
				if err := json.Unmarshal(body.OutboundAssetsJson, &refs); err != nil {
					controlErr <- status.Error(codes.InvalidArgument, "invalid outbound assets payload")
					return
				}
				handle.AddOutboundAssets(refs)
			case *turnpb.RunRequest_Cancel:
				if body.Cancel {
					handle.Cancel()
				}
			default:
				controlErr <- status.Error(codes.InvalidArgument, "unsupported turn control frame")
				return
			}
		}
	}()

	events, errs := handle.Events(), handle.Errs()
	for events != nil || errs != nil {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if err := stream.Send(&turnpb.RunResponse{Body: &turnpb.RunResponse_Event{Event: eventToProto(event)}}); err != nil {
				return err
			}
		case runErr, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if runErr != nil {
				return s.mapError("run turn", runErr)
			}
		case recvErr := <-controlErr:
			if errors.Is(recvErr, io.EOF) {
				controlErr = nil
				continue
			}
			return recvErr
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
	return stream.Send(&turnpb.RunResponse{Body: &turnpb.RunResponse_Completed{Completed: &turnpb.Completed{}}})
}

func (s *Server) RespondToolApproval(req *turnpb.JsonRequest, stream turnpb.TurnService_RespondToolApprovalServer) error {
	var input turn.ToolApprovalResponse
	if err := json.Unmarshal(req.GetJson(), &input); err != nil {
		return status.Error(codes.InvalidArgument, "invalid tool approval payload")
	}
	return s.streamContinuation(stream.Context(), stream.Send, func(ch chan<- json.RawMessage) error {
		return s.service.RespondToolApproval(stream.Context(), input, ch)
	})
}

func (s *Server) RespondUserInput(req *turnpb.JsonRequest, stream turnpb.TurnService_RespondUserInputServer) error {
	var input turn.UserInputResponse
	if err := json.Unmarshal(req.GetJson(), &input); err != nil {
		return status.Error(codes.InvalidArgument, "invalid user input payload")
	}
	return s.streamContinuation(stream.Context(), stream.Send, func(ch chan<- json.RawMessage) error {
		return s.service.RespondUserInput(stream.Context(), input, ch)
	})
}

func (s *Server) streamContinuation(ctx context.Context, send func(*turnpb.EventResponse) error, run func(chan<- json.RawMessage) error) error {
	eventCh := make(chan json.RawMessage, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(eventCh)
		close(eventCh)
	}()
	var seq int64
	for eventCh != nil || errCh != nil {
		select {
		case payload, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			seq++
			if err := send(&turnpb.EventResponse{Seq: seq, Payload: payload}); err != nil {
				return err
			}
		case err := <-errCh:
			errCh = nil
			if err != nil {
				return s.mapError("resume turn", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *Server) AdvancePlainTextUserInput(ctx context.Context, req *turnpb.JsonRequest) (*turnpb.JsonResponse, error) {
	var input userinput.AdvanceTextInput
	if err := json.Unmarshal(req.GetJson(), &input); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid plain text user input payload")
	}
	result, err := s.service.AdvancePlainTextUserInput(ctx, input)
	if err != nil {
		return nil, s.mapError("advance plain text user input", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, status.Error(codes.Internal, "encode plain text user input result")
	}
	return &turnpb.JsonResponse{Json: data}, nil
}

func (s *Server) mapError(operation string, err error) error {
	switch {
	case errors.Is(err, turn.ErrDuplicateTurn):
		return status.Error(codes.AlreadyExists, "duplicate turn")
	case errors.Is(err, turn.ErrTeamNotServed):
		return status.Error(codes.PermissionDenied, "team is not served")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "turn canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "turn deadline exceeded")
	default:
		s.logger.Error("internal turn rpc failed", slog.String("operation", operation), slog.Any("error", err))
		return status.Error(codes.Internal, "internal turn operation failed")
	}
}

func eventToProto(event turn.Event) *turnpb.EventResponse {
	return &turnpb.EventResponse{
		RunId: event.RunID, TeamId: event.TeamID, SessionId: event.SessionID,
		Seq: event.Seq, Kind: event.Kind, Payload: event.Payload,
	}
}
