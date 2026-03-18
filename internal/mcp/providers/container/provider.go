package container

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	toolRead  = "read"
	toolWrite = "write"
	toolList  = "list"
	toolEdit  = "edit"
	toolExec  = "exec"

	defaultExecWorkDir = "/data"
)

// Executor provides filesystem and exec tools (read, write, list, edit, exec) that
// operate inside the bot container via gRPC. All I/O goes through the container
// sandbox — no direct host filesystem access.
type Executor struct {
	clients     bridge.Provider
	execWorkDir string
	logger      *slog.Logger
}

// NewExecutor returns a tool executor backed by gRPC container clients.
func NewExecutor(log *slog.Logger, clients bridge.Provider, execWorkDir string) *Executor {
	if log == nil {
		log = slog.Default()
	}
	wd := strings.TrimSpace(execWorkDir)
	if wd == "" {
		wd = defaultExecWorkDir
	}
	return &Executor{
		clients:     clients,
		execWorkDir: wd,
		logger:      log.With(slog.String("provider", "container_tool")),
	}
}

// ListTools returns read, write, list, edit, and exec tool descriptors.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	wd := p.execWorkDir
	if wd == "" {
		wd = defaultExecWorkDir
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolRead,
			Description: fmt.Sprintf("Read file content inside the bot container. Supports pagination for large files. Max %d lines / %d bytes per call.", readMaxLines, readMaxBytes),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd),
					},
					"line_offset": map[string]any{
						"type":        "integer",
						"description": "Line number to start reading from (1-indexed). Default: 1.",
						"minimum":     1,
						"default":     1,
					},
					"n_lines": map[string]any{
						"type":        "integer",
						"description": fmt.Sprintf("Number of lines to read per call. Default: %d (the per-call maximum). Use a smaller value with line_offset for finer pagination. Max: %d.", readMaxLines, readMaxLines),
						"minimum":     1,
						"maximum":     readMaxLines,
						"default":     readMaxLines,
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        toolWrite,
			Description: "Write file content inside the bot container.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"content": map[string]any{"type": "string", "description": "File content"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        toolList,
			Description: "List directory entries inside the bot container.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": fmt.Sprintf("Directory path (relative to %s or absolute inside container)", wd)},
					"recursive": map[string]any{"type": "boolean", "description": "List recursively"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        toolEdit,
			Description: "Replace exact text in a file inside the bot container.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"old_text": map[string]any{"type": "string", "description": "Exact text to find"},
					"new_text": map[string]any{"type": "string", "description": "Replacement text"},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
		},
		{
			Name:        toolExec,
			Description: fmt.Sprintf("Execute a command in the bot container. Runs in the bot's data directory (%s) by default.", wd),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to run (e.g. ls -la, cat file.txt)",
					},
					"work_dir": map[string]any{
						"type":        "string",
						"description": fmt.Sprintf("Working directory inside the container (default: %s)", wd),
					},
				},
				"required": []string{"command"},
			},
		},
	}, nil
}

// normalizePath converts paths that the LLM may send as /data/... into relative
// paths under the working directory. e.g. /data/test.txt -> test.txt, /data -> .
func (p *Executor) normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	prefix := p.execWorkDir
	if prefix == "" {
		prefix = defaultExecWorkDir
	}
	if path == prefix {
		return "."
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimLeft(strings.TrimPrefix(path, prefix+"/"), "/")
	}
	return path
}

// CallTool dispatches to the appropriate gRPC-backed implementation.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	client, err := p.clients.MCPClient(ctx, botID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("container not reachable: %v", err)), nil
	}

	switch toolName {
	case toolRead:
		return p.callRead(ctx, client, arguments)
	case toolWrite:
		return p.callWrite(ctx, client, arguments)
	case toolList:
		return p.callList(ctx, client, arguments)
	case toolEdit:
		return p.callEdit(ctx, client, arguments)
	case toolExec:
		return p.callExec(ctx, client, botID, arguments)
	default:
		return nil, mcpgw.ErrToolNotFound
	}
}

func (p *Executor) callRead(ctx context.Context, client *bridge.Client, args map[string]any) (map[string]any, error) {
	filePath := p.normalizePath(mcpgw.StringArg(args, "path"))
	if filePath == "" {
		return mcpgw.BuildToolErrorResult("path is required"), nil
	}

	lineOffset := int32(1)
	if offset, ok, err := mcpgw.IntArg(args, "line_offset"); err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid line_offset: %v", err)), nil
	} else if ok {
		if offset < 1 {
			return mcpgw.BuildToolErrorResult("line_offset must be >= 1"), nil
		}
		if offset > math.MaxInt32 {
			return mcpgw.BuildToolErrorResult("line_offset exceeds maximum"), nil
		}
		lineOffset = int32(offset)
	}

	nLines := int32(readMaxLines)
	if n, ok, err := mcpgw.IntArg(args, "n_lines"); err != nil {
		return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid n_lines: %v", err)), nil
	} else if ok {
		if n < 1 {
			return mcpgw.BuildToolErrorResult("n_lines must be >= 1"), nil
		}
		if n > readMaxLines {
			n = readMaxLines
		}
		nLines = int32(n) //nolint:gosec // bounded by readMaxLines (200)
	}

	resp, err := client.ReadFile(ctx, filePath, lineOffset, nLines)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	if resp.GetBinary() {
		return mcpgw.BuildToolErrorResult("file appears to be binary. Read tool only supports text files"), nil
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"content":     resp.GetContent(),
		"total_lines": resp.GetTotalLines(),
	}), nil
}

func (p *Executor) callWrite(ctx context.Context, client *bridge.Client, args map[string]any) (map[string]any, error) {
	filePath := p.normalizePath(mcpgw.StringArg(args, "path"))
	content := mcpgw.StringArg(args, "content")
	if filePath == "" {
		return mcpgw.BuildToolErrorResult("path is required"), nil
	}
	if err := client.WriteFile(ctx, filePath, []byte(content)); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{"ok": true}), nil
}

func (p *Executor) callList(ctx context.Context, client *bridge.Client, args map[string]any) (map[string]any, error) {
	dirPath := p.normalizePath(mcpgw.StringArg(args, "path"))
	if dirPath == "" {
		dirPath = "."
	}
	recursive, _, _ := mcpgw.BoolArg(args, "recursive")

	entries, err := client.ListDir(ctx, dirPath, recursive)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	entriesMaps := make([]map[string]any, len(entries))
	for i, e := range entries {
		entriesMaps[i] = map[string]any{
			"path":     e.GetPath(),
			"is_dir":   e.GetIsDir(),
			"size":     e.GetSize(),
			"mode":     e.GetMode(),
			"mod_time": e.GetModTime(),
		}
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{"path": dirPath, "entries": entriesMaps}), nil
}

func (p *Executor) callEdit(ctx context.Context, client *bridge.Client, args map[string]any) (map[string]any, error) {
	filePath := p.normalizePath(mcpgw.StringArg(args, "path"))
	oldText := mcpgw.StringArg(args, "old_text")
	newText := mcpgw.StringArg(args, "new_text")
	if filePath == "" || oldText == "" {
		return mcpgw.BuildToolErrorResult("path, old_text and new_text are required"), nil
	}

	reader, err := client.ReadRaw(ctx, filePath)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	updated, err := applyEdit(string(raw), filePath, oldText, newText)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	if err := client.WriteFile(ctx, filePath, []byte(updated)); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{"ok": true}), nil
}

func (p *Executor) callExec(ctx context.Context, client *bridge.Client, botID string, args map[string]any) (map[string]any, error) {
	command := strings.TrimSpace(mcpgw.StringArg(args, "command"))
	if command == "" {
		return mcpgw.BuildToolErrorResult("command is required"), nil
	}
	workDir := strings.TrimSpace(mcpgw.StringArg(args, "work_dir"))
	if workDir == "" {
		workDir = p.execWorkDir
	}

	result, err := client.Exec(ctx, command, workDir, 30)
	if err != nil {
		p.logger.Warn("exec failed", slog.String("bot_id", botID), slog.String("command", command), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	stdout := pruneToolOutputText(result.Stdout, "tool result (exec stdout)")
	stderr := pruneToolOutputText(result.Stderr, "tool result (exec stderr)")
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"stdout":    stdout,
		"stderr":    stderr,
		"exit_code": result.ExitCode,
	}), nil
}
