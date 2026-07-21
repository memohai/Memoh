package rpc

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const InternalTokenMetadataKey = "x-memoh-internal-token"

func UnaryClientAuth(secret string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(withToken(ctx, secret), method, req, reply, cc, opts...)
	}
}

func StreamClientAuth(secret string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(withToken(ctx, secret), desc, cc, method, opts...)
	}
}

func UnaryServerAuth(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !validToken(ctx, secret) {
			return nil, status.Error(codes.Unauthenticated, "internal service authentication failed")
		}
		return handler(ctx, req)
	}
}

func StreamServerAuth(secret string) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !validToken(stream.Context(), secret) {
			return status.Error(codes.Unauthenticated, "internal service authentication failed")
		}
		return handler(srv, stream)
	}
}

func withToken(ctx context.Context, secret string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, InternalTokenMetadataKey, strings.TrimSpace(secret))
}

func validToken(ctx context.Context, secret string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(InternalTokenMetadataKey)
	if len(values) != 1 {
		return false
	}
	want := []byte(strings.TrimSpace(secret))
	got := []byte(strings.TrimSpace(values[0]))
	return len(want) > 0 && len(want) == len(got) && subtle.ConstantTimeCompare(want, got) == 1
}
