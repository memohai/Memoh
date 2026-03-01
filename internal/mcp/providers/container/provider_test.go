package container

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

// fakeExecRunner records the last request and returns a preset result.
type fakeExecRunner struct {
	result  *mcpgw.ExecWithCaptureResult
	err     error
	lastReq mcpgw.ExecRequest
	handler func(req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error)
}

func (f *fakeExecRunner) ExecWithCapture(ctx context.Context, req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error) {
	f.lastReq = req
	if f.handler != nil {
		return f.handler(req)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestExecutor_ListTools(t *testing.T) {
	runner := &fakeExecRunner{result: &mcpgw.ExecWithCaptureResult{}}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "test-bot"}
	tools, err := exec.ListTools(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"read": true, "write": true, "list": true, "edit": true, "exec": true}
	if len(tools) != len(want) {
		t.Errorf("got %d tools, want %d", len(tools), len(want))
	}
	for _, tool := range tools {
		if !want[tool.Name] {
			t.Errorf("unexpected tool %q", tool.Name)
		}
	}
}

func TestExecutor_CallTool_Read(t *testing.T) {
	callCount := 0
	runner := &fakeExecRunner{
		handler: func(req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error) {
			callCount++
			cmd := strings.Join(req.Command, " ")
			switch callCount {
			case 1:
				if !strings.Contains(cmd, "head -c 8192") {
					t.Errorf("expected bounded binary probe, got %q", cmd)
				}
			case 2:
				if !strings.Contains(cmd, "sed -n") {
					t.Errorf("expected sed command, got %q", cmd)
				}
			default:
				t.Errorf("unexpected extra call #%d: %q", callCount, cmd)
			}
			return &mcpgw.ExecWithCaptureResult{Stdout: "hello world", ExitCode: 0}, nil
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}

	result, err := exec.CallTool(ctx, session, "read", map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content["content"] == "" {
		t.Errorf("content should not be empty, got %v", content["content"])
	}
	if callCount != 2 {
		t.Errorf("expected 2 exec calls, got %d", callCount)
	}
}

func TestExecutor_CallTool_Read_InvalidPaginationArgs(t *testing.T) {
	runner := &fakeExecRunner{
		handler: func(req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error) {
			t.Fatalf("unexpected exec call: %v", req.Command)
			return nil, nil
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "invalid line_offset type",
			args: map[string]any{"path": "test.txt", "line_offset": "abc"},
			want: "invalid line_offset",
		},
		{
			name: "invalid n_lines type",
			args: map[string]any{"path": "test.txt", "n_lines": "abc"},
			want: "invalid n_lines",
		},
		{
			name: "line_offset below minimum",
			args: map[string]any{"path": "test.txt", "line_offset": 0},
			want: "line_offset must be >= 1",
		},
		{
			name: "n_lines below minimum",
			args: map[string]any{"path": "test.txt", "n_lines": 0},
			want: "n_lines must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := exec.CallTool(ctx, session, "read", tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if isErr, _ := result["isError"].(bool); !isErr {
				t.Fatalf("expected tool error containing %q", tt.want)
			}
			msg := ""
			if content, ok := result["content"].([]map[string]any); ok && len(content) > 0 {
				msg, _ = content[0]["text"].(string)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("error = %q, want substring %q", msg, tt.want)
			}
		})
	}
}

func TestExecutor_CallTool_Write(t *testing.T) {
	runner := &fakeExecRunner{
		handler: func(req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error) {
			cmd := strings.Join(req.Command, " ")
			if !strings.Contains(cmd, "base64 -d") {
				return nil, fmt.Errorf("expected base64 write, got %q", cmd)
			}
			return &mcpgw.ExecWithCaptureResult{ExitCode: 0}, nil
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}

	result, err := exec.CallTool(ctx, session, "write", map[string]any{
		"path": "hello.txt", "content": "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
}

func TestExecutor_CallTool_List(t *testing.T) {
	runner := &fakeExecRunner{
		result: &mcpgw.ExecWithCaptureResult{
			Stdout:   "./test.txt|regular file|42|644|1700000000\n./subdir|directory|4096|755|1700000000\n",
			ExitCode: 0,
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}

	result, err := exec.CallTool(ctx, session, "list", map[string]any{"path": "."})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	entries, ok := content["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries type = %T", content["entries"])
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

func TestExecutor_CallTool_Edit(t *testing.T) {
	callCount := 0
	runner := &fakeExecRunner{
		handler: func(req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error) {
			callCount++
			cmd := strings.Join(req.Command, " ")
			if strings.Contains(cmd, "cat") {
				// Read step: return original content.
				return &mcpgw.ExecWithCaptureResult{Stdout: "hello world", ExitCode: 0}, nil
			}
			if strings.Contains(cmd, "base64 -d") {
				// Write step: verify the written content contains the replacement.
				// Extract base64 from: echo '<b64>' | base64 -d > 'path'
				parts := strings.Split(cmd, "'")
				for _, p := range parts {
					decoded, err := base64.StdEncoding.DecodeString(p)
					if err == nil && strings.Contains(string(decoded), "goodbye world") {
						return &mcpgw.ExecWithCaptureResult{ExitCode: 0}, nil
					}
				}
				return &mcpgw.ExecWithCaptureResult{ExitCode: 0}, nil
			}
			return &mcpgw.ExecWithCaptureResult{ExitCode: 0}, nil
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}

	result, err := exec.CallTool(ctx, session, "edit", map[string]any{
		"path": "test.txt", "old_text": "hello", "new_text": "goodbye",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 exec calls (read+write), got %d", callCount)
	}
}

func TestExecutor_CallTool_Exec(t *testing.T) {
	runner := &fakeExecRunner{
		result: &mcpgw.ExecWithCaptureResult{
			Stdout:   "hello\n",
			Stderr:   "",
			ExitCode: 0,
		},
	}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(ctx, session, toolExec, map[string]any{"command": "echo hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		t.Fatal("no structuredContent")
	}
	if content["stdout"] != "hello\n" {
		t.Errorf("stdout = %v", content["stdout"])
	}
	if content["exit_code"].(uint32) != 0 {
		t.Errorf("exit_code = %v", content["exit_code"])
	}
}

func TestExecutor_CallTool_NoBotID(t *testing.T) {
	runner := &fakeExecRunner{result: &mcpgw.ExecWithCaptureResult{}}
	exec := NewExecutor(nil, runner, "/data")
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{}
	result, err := exec.CallTool(ctx, session, "read", map[string]any{"path": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot_id is missing")
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/data/test.txt", "test.txt"},
		{"/data/foo/bar.txt", "foo/bar.txt"},
		{"/data", "."},
		{"test.txt", "test.txt"},
		{"", ""},
		{".", "."},
	}
	exec := &Executor{execWorkDir: "/data"}
	for _, tt := range tests {
		got := exec.normalizePath(tt.in)
		if got != tt.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
