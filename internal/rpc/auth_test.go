package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestValidToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(InternalTokenMetadataKey, "secret"))
	if !validToken(ctx, "secret") {
		t.Fatal("expected matching token to pass")
	}
	if validToken(ctx, "wrong") {
		t.Fatal("expected mismatched token to fail")
	}
	if validToken(context.Background(), "secret") {
		t.Fatal("expected missing token to fail")
	}
}
