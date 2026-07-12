package tools

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const readImageHint = "Also supports reading image files (PNG, JPEG, GIF, WebP)"

type containerTestBridgeProvider struct {
	client *bridge.Client
	err    error
}

func (p containerTestBridgeProvider) MCPClient(context.Context, string) (*bridge.Client, error) {
	return p.client, p.err
}

type largeReadTestContainerService struct {
	pb.UnimplementedContainerServiceServer
	size int64
}

func (s *largeReadTestContainerService) Stat(_ context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	return &pb.StatResponse{
		Entry: &pb.FileEntry{
			Path: req.GetPath(),
			Size: s.size,
		},
	}, nil
}

func newLargeReadTestClient(t *testing.T, size int64) *bridge.Client {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, &largeReadTestContainerService{size: size})

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

func readToolDescription(t *testing.T, supportsImageInput bool) string {
	t.Helper()
	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: supportsImageInput,
	})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name == "read" {
			return tool.Description
		}
	}
	t.Fatalf("read tool not found in container provider tools")
	return ""
}

func TestContainerReadDescriptionIncludesImageHintWhenImageInputSupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, true)
	if !strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerReadDescriptionOmitsImageHintWhenImageInputUnsupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, false)
	if strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to NOT contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerApplyPatchDescriptionDoesNotReferenceSiblingTools(t *testing.T) {
	t.Parallel()

	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name != ToolApplyPatch().String() {
			continue
		}
		for _, absent := range []string{"Use edit", "Use write", "`edit`", "`write`"} {
			if strings.Contains(tool.Description, absent) {
				t.Fatalf("apply_patch description references sibling tool %q:\n%s", absent, tool.Description)
			}
		}
		return
	}
	t.Fatalf("apply_patch tool not found")
}

func TestContainerProviderPreservesWorkspaceClientError(t *testing.T) {
	t.Parallel()

	cause := errors.New("runtime unavailable")
	provider := NewContainerProvider(nil, containerTestBridgeProvider{err: cause}, nil, "")
	_, err := provider.getClient(context.Background(), "bot-1")
	if !errors.Is(err, cause) {
		t.Fatalf("getClient() error = %v, want wrapped runtime cause", err)
	}
	if !strings.Contains(err.Error(), "workspace is not reachable") {
		t.Fatalf("getClient() error = %v, want Workspace-oriented context", err)
	}
}

func TestContainerReadLargeFileErrorDoesNotReferenceSiblingTools(t *testing.T) {
	t.Parallel()

	client := newLargeReadTestClient(t, 17*1024*1024)
	provider := NewContainerProvider(nil, containerTestBridgeProvider{client: client}, nil, "")
	_, err := provider.execRead(context.Background(), SessionContext{BotID: "bot-1"}, map[string]any{
		"path": "/data/large.log",
	})
	if err == nil {
		t.Fatal("expected large file read to fail")
	}
	for _, absent := range []string{ToolExec().String(), "head/tail/sed", "line_offset", "n_lines"} {
		if strings.Contains(err.Error(), absent) {
			t.Fatalf("large file error should not reference unavailable or ineffective fallback %q: %v", absent, err)
		}
	}
	if !strings.Contains(err.Error(), "read tool cannot read files above this limit") {
		t.Fatalf("large file error should state the read limit directly, got %v", err)
	}
}

func TestDetectBlockedSleep(t *testing.T) {
	tests := []struct {
		command string
		blocked bool
	}{
		// Should block
		{"sleep 5", true},
		{"sleep 10", true},
		{"sleep 30", true},
		{"sleep 5 && echo done", true},
		{"sleep 5; echo done", true},

		// Should allow
		{"sleep 1", false},       // under 2 seconds
		{"sleep 0.5", false},     // under 2 seconds
		{"echo hello", false},    // not sleep
		{"npm install", false},   // not sleep
		{"echo sleep 5", false},  // sleep not at start
		{"cat sleep.txt", false}, // not the sleep command
	}

	for _, tt := range tests {
		result := detectBlockedSleep(tt.command)
		if tt.blocked && result == "" {
			t.Errorf("expected %q to be blocked, but it was allowed", tt.command)
		}
		if !tt.blocked && result != "" {
			t.Errorf("expected %q to be allowed, but got: %s", tt.command, result)
		}
	}
}
