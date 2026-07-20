package rpc

import (
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const MaxMessageBytes = 16 << 20

func Dial(target, secret string) (*grpc.ClientConn, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("internal rpc target is required")
	}
	return grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientAuth(secret)),
		grpc.WithStreamInterceptor(StreamClientAuth(secret)),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(MaxMessageBytes),
			grpc.MaxCallSendMsgSize(MaxMessageBytes),
		),
	)
}

func NewServer(secret string, opts ...grpc.ServerOption) *grpc.Server {
	base := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(UnaryServerAuth(secret)),
		grpc.ChainStreamInterceptor(StreamServerAuth(secret)),
		grpc.MaxRecvMsgSize(MaxMessageBytes),
		grpc.MaxSendMsgSize(MaxMessageBytes),
	}
	return grpc.NewServer(append(base, opts...)...)
}
