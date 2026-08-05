package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/storage/providers/localfs"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const readMediaTestBufSize = 1 << 20

type readMediaTestContainerService struct {
	pb.UnimplementedContainerServiceServer
	files map[string][]byte
}

func (s *readMediaTestContainerService) ReadRaw(req *pb.ReadRawRequest, stream pb.ContainerService_ReadRawServer) error {
	data, ok := s.files[req.GetPath()]
	if !ok {
		return status.Error(codes.NotFound, "not found")
	}
	if len(data) == 0 {
		return nil
	}
	return stream.Send(&pb.DataChunk{Data: data})
}

func newReadMediaTestClient(t *testing.T, files map[string][]byte) *bridge.Client {
	t.Helper()

	lis := bufconn.Listen(readMediaTestBufSize)
	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, &readMediaTestContainerService{files: files})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		<-done
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return bridge.NewClientFromConn(conn)
}

func TestReadImageFromContainerSuccess(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\npayload")
	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.png": pngBytes,
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.png", 0)

	if !result.Public.OK {
		t.Fatalf("expected success result, got %+v", result.Public)
	}
	if result.Public.Path != "/data/images/demo.png" {
		t.Fatalf("unexpected path: %q", result.Public.Path)
	}
	if result.Public.Mime != "image/png" {
		t.Fatalf("unexpected mime: %q", result.Public.Mime)
	}
	if result.Public.Size != len(pngBytes) {
		t.Fatalf("unexpected size: %d", result.Public.Size)
	}

	expectedBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	if result.ImageBase64 != expectedBase64 {
		t.Fatalf("unexpected image payload: %q", result.ImageBase64)
	}
	if result.ImageMediaType != "image/png" {
		t.Fatalf("unexpected image media type: %q", result.ImageMediaType)
	}
}

func TestContainerReadByContentHash(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\npayload")
	service := media.NewService(slog.Default(), localfs.New(t.TempDir()))
	asset, err := service.Ingest(context.Background(), media.IngestInput{
		BotID:  "bot-1",
		Mime:   "image/png",
		Reader: bytes.NewReader(pngBytes),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	provider := NewContainerProvider(nil, nil, nil, "")
	provider.SetMediaService(service)
	result, err := provider.execRead(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	}, map[string]any{"content_hash": strings.ToUpper(asset.ContentHash)})
	if err != nil {
		t.Fatalf("execRead() error = %v", err)
	}
	output, ok := result.(ReadMediaToolOutput)
	if !ok {
		t.Fatalf("execRead() result = %T, want ReadMediaToolOutput", result)
	}
	if !output.Public.OK || output.Public.ContentHash != asset.ContentHash {
		t.Fatalf("execRead() public result = %+v", output.Public)
	}
	if output.Public.Path != "" {
		t.Fatalf("execRead() path = %q, want empty", output.Public.Path)
	}
	if got := output.ImageBase64; got != base64.StdEncoding.EncodeToString(pngBytes) {
		t.Fatalf("execRead() image = %q", got)
	}
}

func TestContainerReadByContentHashRejectsPathAndCrossBotLookup(t *testing.T) {
	t.Parallel()

	service := media.NewService(slog.Default(), localfs.New(t.TempDir()))
	provider := NewContainerProvider(nil, nil, nil, "")
	provider.SetMediaService(service)
	hash := strings.Repeat("a", 64)

	if _, err := provider.execRead(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	}, map[string]any{"path": "/data/a.png", "content_hash": hash}); err == nil {
		t.Fatal("execRead() with path and content_hash unexpectedly succeeded")
	}

	result, err := provider.execRead(context.Background(), SessionContext{
		BotID:              "bot-2",
		SupportsImageInput: true,
	}, map[string]any{"content_hash": hash})
	if err != nil {
		t.Fatalf("execRead() missing asset error = %v, want public result", err)
	}
	output, ok := result.(ReadMediaToolOutput)
	if !ok || output.Public.OK {
		t.Fatalf("execRead() missing asset result = %#v", result)
	}
}

func TestReadImageFromContainerRejectsUnsupportedMime(t *testing.T) {
	t.Parallel()

	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.svg": svgBytes,
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.svg", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if !strings.Contains(result.Public.Error, "PNG, JPEG, GIF, or WebP") {
		t.Fatalf("unexpected error: %q", result.Public.Error)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}

func TestReadImageFromContainerRejectsCorruptedBytes(t *testing.T) {
	t.Parallel()

	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.png": []byte("definitely not a png"),
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.png", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if !strings.Contains(result.Public.Error, "PNG, JPEG, GIF, or WebP") {
		t.Fatalf("unexpected error: %q", result.Public.Error)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}

func TestReadImageFromContainerNotFound(t *testing.T) {
	t.Parallel()

	client := newReadMediaTestClient(t, map[string][]byte{})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/missing.png", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}
