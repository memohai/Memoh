package tools

import (
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

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

type readMediaStaticProvider struct {
	client *bridge.Client
}

func (p *readMediaStaticProvider) MCPClient(_ context.Context, _ string) (*bridge.Client, error) {
	return p.client, nil
}

func newReadMediaBridgeProvider(t *testing.T, files map[string][]byte) bridge.Provider {
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

	return &readMediaStaticProvider{client: bridge.NewClientFromConn(conn)}
}

func findToolByName(tools []sdk.Tool, name string) (sdk.Tool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return sdk.Tool{}, false
}

func TestReadMediaProviderToolsOnlyWhenImageInputIsSupported(t *testing.T) {
	t.Parallel()

	provider := NewReadMediaProvider(nil, newReadMediaBridgeProvider(t, nil), "/data")

	toolsWithoutImage, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: false,
	})
	if err != nil {
		t.Fatalf("Tools without image input returned error: %v", err)
	}
	if len(toolsWithoutImage) != 0 {
		t.Fatalf("expected no tools without image input support, got %d", len(toolsWithoutImage))
	}

	toolsWithImage, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	})
	if err != nil {
		t.Fatalf("Tools with image input returned error: %v", err)
	}

	tool, ok := findToolByName(toolsWithImage, toolReadMedia)
	if !ok {
		t.Fatalf("expected %q tool to be exposed", toolReadMedia)
	}
	if tool.Execute == nil {
		t.Fatal("expected read_media tool to be executable")
	}
}

func TestReadMediaProviderExecuteReadsImageUnderData(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\npayload")
	provider := NewReadMediaProvider(nil, newReadMediaBridgeProvider(t, map[string][]byte{
		"/data/images/demo.png": pngBytes,
	}), "/data")

	tools, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}

	tool, ok := findToolByName(tools, toolReadMedia)
	if !ok {
		t.Fatalf("expected %q tool", toolReadMedia)
	}

	output, err := tool.Execute(&sdk.ToolExecContext{
		Context:    context.Background(),
		ToolCallID: "call-1",
		ToolName:   toolReadMedia,
	}, map[string]any{"path": "images/demo.png"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result, ok := output.(readMediaToolOutput)
	if !ok {
		t.Fatalf("expected readMediaToolOutput, got %T", output)
	}
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

func TestReadMediaProviderExecuteRejectsPathOutsideData(t *testing.T) {
	t.Parallel()

	provider := NewReadMediaProvider(nil, newReadMediaBridgeProvider(t, nil), "/data")

	tools, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}

	tool, ok := findToolByName(tools, toolReadMedia)
	if !ok {
		t.Fatalf("expected %q tool", toolReadMedia)
	}

	output, err := tool.Execute(&sdk.ToolExecContext{
		Context:    context.Background(),
		ToolCallID: "call-2",
		ToolName:   toolReadMedia,
	}, map[string]any{"path": "/tmp/demo.png"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result, ok := output.(readMediaToolOutput)
	if !ok {
		t.Fatalf("expected readMediaToolOutput, got %T", output)
	}
	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if !strings.Contains(result.Public.Error, "path must be under /data") {
		t.Fatalf("unexpected error: %q", result.Public.Error)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}

func TestReadMediaProviderExecuteRejectsExtensionOnlySVG(t *testing.T) {
	t.Parallel()

	provider := NewReadMediaProvider(nil, newReadMediaBridgeProvider(t, map[string][]byte{
		"/data/images/demo.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
	}), "/data")

	tools, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}

	tool, ok := findToolByName(tools, toolReadMedia)
	if !ok {
		t.Fatalf("expected %q tool", toolReadMedia)
	}

	output, err := tool.Execute(&sdk.ToolExecContext{
		Context:    context.Background(),
		ToolCallID: "call-3",
		ToolName:   toolReadMedia,
	}, map[string]any{"path": "images/demo.svg"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result, ok := output.(readMediaToolOutput)
	if !ok {
		t.Fatalf("expected readMediaToolOutput, got %T", output)
	}
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

func TestReadMediaProviderExecuteRejectsCorruptedRasterBytes(t *testing.T) {
	t.Parallel()

	provider := NewReadMediaProvider(nil, newReadMediaBridgeProvider(t, map[string][]byte{
		"/data/images/demo.png": []byte("definitely not a png"),
	}), "/data")

	tools, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: true,
	})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}

	tool, ok := findToolByName(tools, toolReadMedia)
	if !ok {
		t.Fatalf("expected %q tool", toolReadMedia)
	}

	output, err := tool.Execute(&sdk.ToolExecContext{
		Context:    context.Background(),
		ToolCallID: "call-4",
		ToolName:   toolReadMedia,
	}, map[string]any{"path": "images/demo.png"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result, ok := output.(readMediaToolOutput)
	if !ok {
		t.Fatalf("expected readMediaToolOutput, got %T", output)
	}
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
