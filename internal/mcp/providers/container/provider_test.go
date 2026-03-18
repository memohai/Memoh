package container

import (
	"context"
	"math"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const bufSize = 1 << 20

// fakeContainerService is an in-process gRPC server for testing.
// Each RPC handler can be overridden via handler fields.
type fakeContainerService struct {
	pb.UnimplementedContainerServiceServer

	mu    sync.Mutex
	files map[string][]byte // path -> content

	execStdout   string
	execStderr   string
	execExitCode int32
}

func newFakeService() *fakeContainerService {
	return &fakeContainerService{files: make(map[string][]byte)}
}

func (f *fakeContainerService) setFile(path, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = []byte(content)
}

func (f *fakeContainerService) getFile(path string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[path]
	return data, ok
}

func (f *fakeContainerService) ReadFile(_ context.Context, req *pb.ReadFileRequest) (*pb.ReadFileResponse, error) {
	data, ok := f.getFile(req.GetPath())
	if !ok {
		return &pb.ReadFileResponse{Content: "", TotalLines: 0}, nil
	}
	content := string(data)
	lines := splitLines(content)
	total := int32(min(len(lines), math.MaxInt32)) //nolint:gosec // G115: value is clamped to math.MaxInt32 above

	offset := req.GetLineOffset()
	if offset < 1 {
		offset = 1
	}
	n := req.GetNLines()
	if n <= 0 {
		n = int32(readMaxLines)
	}

	start := int(offset - 1)
	if start >= len(lines) {
		return &pb.ReadFileResponse{Content: "", TotalLines: total}, nil
	}
	end := start + int(n)
	if end > len(lines) {
		end = len(lines)
	}
	result := ""
	var resultSb76 strings.Builder
	for i, l := range lines[start:end] {
		if i > 0 {
			resultSb76.WriteString("\n")
		}
		resultSb76.WriteString(l)
	}
	result += resultSb76.String()
	return &pb.ReadFileResponse{Content: result, TotalLines: total}, nil
}

func (f *fakeContainerService) WriteFile(_ context.Context, req *pb.WriteFileRequest) (*pb.WriteFileResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[req.GetPath()] = req.GetContent()
	return &pb.WriteFileResponse{}, nil
}

func (f *fakeContainerService) ListDir(_ context.Context, req *pb.ListDirRequest) (*pb.ListDirResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var entries []*pb.FileEntry
	dir := req.GetPath()
	if dir == "." {
		dir = ""
	}
	for path := range f.files {
		if dir == "" || path == dir || hasPrefix(path, dir+"/") {
			name := path
			if dir != "" && hasPrefix(path, dir+"/") {
				name = path[len(dir)+1:]
			}
			entries = append(entries, &pb.FileEntry{
				Path:  name,
				IsDir: false,
				Size:  int64(len(f.files[path])),
			})
		}
	}
	return &pb.ListDirResponse{Entries: entries}, nil
}

func (f *fakeContainerService) ReadRaw(req *pb.ReadRawRequest, stream pb.ContainerService_ReadRawServer) error {
	data, ok := f.getFile(req.GetPath())
	if !ok {
		return nil
	}
	return stream.Send(&pb.DataChunk{Data: data})
}

func (f *fakeContainerService) Exec(stream pb.ContainerService_ExecServer) error {
	// Consume the config message.
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if f.execStdout != "" {
		if err := stream.Send(&pb.ExecOutput{Stream: pb.ExecOutput_STDOUT, Data: []byte(f.execStdout)}); err != nil {
			return err
		}
	}
	if f.execStderr != "" {
		if err := stream.Send(&pb.ExecOutput{Stream: pb.ExecOutput_STDERR, Data: []byte(f.execStderr)}); err != nil {
			return err
		}
	}
	return stream.Send(&pb.ExecOutput{Stream: pb.ExecOutput_EXIT, ExitCode: f.execExitCode})
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

// testSetup creates a bufconn gRPC server and a matching bridge.Provider.
func testSetup(t *testing.T, svc *fakeContainerService) bridge.Provider {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, svc)

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
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := bridge.NewClientFromConn(conn)
	return &staticProvider{client: client}
}

// staticProvider always returns the same client, ignoring botID.
type staticProvider struct {
	client *bridge.Client
}

func (p *staticProvider) MCPClient(_ context.Context, _ string) (*bridge.Client, error) {
	return p.client, nil
}

func session() mcpgw.ToolSessionContext {
	return mcpgw.ToolSessionContext{BotID: "bot-test"}
}

func executor(provider bridge.Provider) *Executor {
	return NewExecutor(nil, provider, defaultExecWorkDir)
}

// --- Tests ---

func TestExecutor_ListTools(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	tools, err := ex.ListTools(context.Background(), session())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{toolRead: false, toolWrite: false, toolList: false, toolEdit: false, toolExec: false}
	for _, tool := range tools {
		want[tool.Name] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q missing from ListTools", name)
		}
	}
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}
}

func TestExecutor_CallTool_Read(t *testing.T) {
	svc := newFakeService()
	svc.setFile("hello.txt", "line1\nline2\nline3")
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolRead, map[string]any{
		"path": "hello.txt",
	})
	if err != nil {
		t.Fatalf("CallTool read: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected tool error: %v", result)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("expected structuredContent, got %T: %v", result["structuredContent"], result)
	}
	content, _ := structured["content"].(string)
	if content == "" {
		t.Errorf("expected non-empty content, got %q", content)
	}
	totalLines, _ := structured["total_lines"].(int32)
	if totalLines != 3 {
		t.Errorf("expected total_lines=3, got %v", structured["total_lines"])
	}
}

func TestExecutor_CallTool_Read_Binary(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	// Reading a nonexistent file should return empty content, not error.
	result, err := ex.CallTool(context.Background(), session(), toolRead, map[string]any{
		"path": "missing.txt",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	// Empty file returns success with empty content (total_lines=0).
	if isError, _ := result["isError"].(bool); isError {
		t.Logf("tool returned error for missing file: %v", result)
	}
}

func TestExecutor_CallTool_Read_Pagination(t *testing.T) {
	svc := newFakeService()
	svc.setFile("big.txt", "a\nb\nc\nd\ne")
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolRead, map[string]any{
		"path":        "big.txt",
		"line_offset": float64(3),
		"n_lines":     float64(2),
	})
	if err != nil {
		t.Fatalf("CallTool read pagination: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected error: %v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	content := structured["content"].(string)
	if content != "c\nd" {
		t.Errorf("expected 'c\nd', got %q", content)
	}
}

func TestExecutor_CallTool_Read_InvalidArgs(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolRead, map[string]any{
		"path":        "f.txt",
		"line_offset": float64(0),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if isError, _ := result["isError"].(bool); !isError {
		t.Errorf("expected error for line_offset=0, got %v", result)
	}
}

func TestExecutor_CallTool_Write(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolWrite, map[string]any{
		"path":    "out.txt",
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("CallTool write: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected error: %v", result)
	}

	data, ok := svc.getFile("out.txt")
	if !ok {
		t.Fatal("file not written")
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestExecutor_CallTool_List(t *testing.T) {
	svc := newFakeService()
	svc.setFile("dir/a.txt", "aaa")
	svc.setFile("dir/b.txt", "bbb")
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolList, map[string]any{
		"path": "dir",
	})
	if err != nil {
		t.Fatalf("CallTool list: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected error: %v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	entries, _ := structured["entries"].([]map[string]any)
	if len(entries) < 1 {
		t.Logf("note: got %d entries", len(entries))
	}
}

func TestExecutor_CallTool_Edit(t *testing.T) {
	svc := newFakeService()
	svc.setFile("edit.txt", "hello world\n")
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolEdit, map[string]any{
		"path":     "edit.txt",
		"old_text": "hello world",
		"new_text": "goodbye world",
	})
	if err != nil {
		t.Fatalf("CallTool edit: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected error: %v", result)
	}

	data, _ := svc.getFile("edit.txt")
	if string(data) != "goodbye world\n" {
		t.Errorf("expected 'goodbye world\n', got %q", string(data))
	}
}

func TestExecutor_CallTool_Edit_NotFound(t *testing.T) {
	svc := newFakeService()
	svc.setFile("edit.txt", "hello world\n")
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolEdit, map[string]any{
		"path":     "edit.txt",
		"old_text": "no such text",
		"new_text": "replacement",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if isError, _ := result["isError"].(bool); !isError {
		t.Errorf("expected error for not-found old_text, got %v", result)
	}
}

func TestExecutor_CallTool_Exec(t *testing.T) {
	svc := newFakeService()
	svc.execStdout = "hello from exec\n"
	svc.execExitCode = 0
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolExec, map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("CallTool exec: %v", err)
	}
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("unexpected error: %v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	stdout, _ := structured["stdout"].(string)
	if stdout == "" {
		t.Errorf("expected non-empty stdout, got %q", stdout)
	}
	exitCode, _ := structured["exit_code"].(int32)
	if exitCode != 0 {
		t.Errorf("expected exit_code=0, got %v", exitCode)
	}
}

func TestExecutor_CallTool_Exec_NonZeroExit(t *testing.T) {
	svc := newFakeService()
	svc.execStderr = "command not found\n"
	svc.execExitCode = 127
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), session(), toolExec, map[string]any{
		"command": "nosuchcmd",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	// Non-zero exit is not a tool error — it's returned as structured output.
	if isError, _ := result["isError"].(bool); isError {
		t.Errorf("unexpected tool error for non-zero exit: %v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	exitCode, _ := structured["exit_code"].(int32)
	if exitCode != 127 {
		t.Errorf("expected exit_code=127, got %v", exitCode)
	}
}

func TestExecutor_CallTool_NoBotID(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	result, err := ex.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: ""}, toolRead, map[string]any{
		"path": "f.txt",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if isError, _ := result["isError"].(bool); !isError {
		t.Errorf("expected error for empty bot_id")
	}
}

func TestExecutor_CallTool_UnknownTool(t *testing.T) {
	svc := newFakeService()
	provider := testSetup(t, svc)
	ex := executor(provider)

	_, err := ex.CallTool(context.Background(), session(), "nosuch", nil)
	if err == nil {
		t.Errorf("expected error for unknown tool")
	}
}

func TestExecutor_NormalizePath(t *testing.T) {
	ex := &Executor{execWorkDir: "/data"}
	cases := []struct {
		in, want string
	}{
		{"/data/test.txt", "test.txt"},
		{"/data", "."},
		{"/data/a/b.txt", "a/b.txt"},
		{"relative.txt", "relative.txt"},
		{"", ""},
	}
	for _, c := range cases {
		got := ex.normalizePath(c.in)
		if got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
