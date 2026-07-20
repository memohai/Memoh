package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/memohai/memoh/internal/rpc/runtimepb"
)

var ErrUnavailable = errors.New("internal runtime unavailable")

type Handler func(context.Context, json.RawMessage) (any, error)

type Server struct {
	runtimepb.UnimplementedRuntimeServiceServer
	logger   *slog.Logger
	handlers map[string]Handler
}

func NewServer(log *slog.Logger, handlers map[string]Handler) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{logger: log.With(slog.String("component", "runtime_rpc")), handlers: handlers}
}

func (s *Server) Call(ctx context.Context, req *runtimepb.CallRequest) (*runtimepb.CallResponse, error) {
	method := strings.TrimSpace(req.GetMethod())
	handler := s.handlers[method]
	if handler == nil {
		return nil, status.Error(codes.Unimplemented, "runtime method is not implemented")
	}
	result, err := handler(ctx, json.RawMessage(req.GetPayload()))
	if err != nil {
		s.logger.Error("runtime rpc call failed", slog.String("method", method), slog.Any("error", err))
		if status.Code(err) != codes.Unknown {
			return nil, err
		}
		return nil, status.Error(codes.Internal, "internal runtime operation failed")
	}
	if result == nil {
		return &runtimepb.CallResponse{}, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("runtime rpc result encoding failed", slog.String("method", method), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "internal runtime result encoding failed")
	}
	return &runtimepb.CallResponse{Payload: data}, nil
}

type Client struct {
	client runtimepb.RuntimeServiceClient
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: runtimepb.NewRuntimeServiceClient(conn)}
}

func (c *Client) Call(ctx context.Context, method string, input, output any) error {
	var payload []byte
	var err error
	if input != nil {
		payload, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	resp, err := c.client.Call(ctx, &runtimepb.CallRequest{Method: method, Payload: payload})
	if err != nil {
		switch status.Code(err) {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Unauthenticated:
			return errors.Join(ErrUnavailable, err)
		default:
			return err
		}
	}
	if output == nil || len(resp.GetPayload()) == 0 {
		return nil
	}
	return json.Unmarshal(resp.GetPayload(), output)
}
