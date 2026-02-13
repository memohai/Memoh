package container

import (
	"context"
	"log/slog"
	"strings"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const (
	toolRead  = "read"
	toolWrite = "write"
	toolList  = "list"
	toolEdit  = "edit"
	toolExec  = "exec"

	defaultExecWorkDir = "/data"
	shellCommandName   = "/bin/sh"
	shellCommandFlag   = "-c"
)

// ExecRunner runs a command in the bot container and returns stdout, stderr and exit code.
type ExecRunner interface {
	ExecWithCapture(ctx context.Context, req mcpgw.ExecRequest) (*mcpgw.ExecWithCaptureResult, error)
}

// Executor provides filesystem and exec tools (read, write, list, edit, exec) that
// operate inside the bot container via ExecRunner. All I/O goes through the container
// sandbox — no direct host filesystem access.
type Executor struct {
	execRunner  ExecRunner
	execWorkDir string
	logger      *slog.Logger
}

// NewExecutor returns a tool executor. execRunner is required — all tools delegate
// to it for container-side I/O. execWorkDir is the default working directory inside
// the container (e.g. /data).
func NewExecutor(log *slog.Logger, execRunner ExecRunner, execWorkDir string) *Executor {
	if log == nil {
		log = slog.Default()
	}
	wd := strings.TrimSpace(execWorkDir)
	if wd == "" {
		wd = defaultExecWorkDir
	}
	return &Executor{
		execRunner:  execRunner,
		execWorkDir: wd,
		logger:      log.With(slog.String("provider", "container_tool")),
	}
}

// ListTools returns read, write, list, edit, and exec tool descriptors.
func (p *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolRead,
			Description: "Read file content inside the bot container.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "file path (relative to /data or absolute inside container)"},
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
					"path":    map[string]any{"type": "string", "description": "file path (relative to /data or absolute inside container)"},
					"content": map[string]any{"type": "string", "description": "file content"},
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
					"path":      map[string]any{"type": "string", "description": "directory path (relative to /data or absolute inside container)"},
					"recursive": map[string]any{"type": "boolean", "description": "list recursively"},
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
					"path":     map[string]any{"type": "string", "description": "file path (relative to /data or absolute inside container)"},
					"old_text": map[string]any{"type": "string", "description": "exact text to find"},
					"new_text": map[string]any{"type": "string", "description": "replacement text"},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
		},
		{
			Name:        toolExec,
			Description: "Execute a command in the bot container. Runs in the bot's data directory (/data) by default.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to run (e.g. ls -la, cat file.txt)",
					},
					"work_dir": map[string]any{
						"type":        "string",
						"description": "Working directory inside the container (default: /data)",
					},
				},
				"required": []string{"command"},
			},
		},
	}, nil
}

// normalizePath converts paths that the LLM may send as /data/... into relative
// paths under the working directory. e.g. /data/test.txt -> test.txt, /data -> .
func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	const prefix = "/data"
	if path == prefix {
		return "."
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimLeft(strings.TrimPrefix(path, prefix+"/"), "/")
	}
	return path
}

// CallTool dispatches to the appropriate container-exec backed implementation.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	switch toolName {
	case toolRead:
		filePath := normalizePath(mcpgw.StringArg(arguments, "path"))
		if filePath == "" {
			return mcpgw.BuildToolErrorResult("path is required"), nil
		}
		content, err := ExecRead(ctx, p.execRunner, botID, p.execWorkDir, filePath)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"content": content}), nil

	case toolWrite:
		filePath := normalizePath(mcpgw.StringArg(arguments, "path"))
		content := mcpgw.StringArg(arguments, "content")
		if filePath == "" {
			return mcpgw.BuildToolErrorResult("path is required"), nil
		}
		if err := ExecWrite(ctx, p.execRunner, botID, p.execWorkDir, filePath, content); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"ok": true}), nil

	case toolList:
		dirPath := normalizePath(mcpgw.StringArg(arguments, "path"))
		if dirPath == "" {
			dirPath = "."
		}
		recursive, _, _ := mcpgw.BoolArg(arguments, "recursive")
		entries, err := ExecList(ctx, p.execRunner, botID, p.execWorkDir, dirPath, recursive)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		entriesMaps := make([]map[string]any, len(entries))
		for i, e := range entries {
			entriesMaps[i] = map[string]any{
				"path":     e.Path,
				"is_dir":   e.IsDir,
				"size":     e.Size,
				"mode":     e.Mode,
				"mod_time": e.ModTime,
			}
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"path": dirPath, "entries": entriesMaps}), nil

	case toolEdit:
		filePath := normalizePath(mcpgw.StringArg(arguments, "path"))
		oldText := mcpgw.StringArg(arguments, "old_text")
		newText := mcpgw.StringArg(arguments, "new_text")
		if filePath == "" || oldText == "" {
			return mcpgw.BuildToolErrorResult("path, old_text and new_text are required"), nil
		}
		// Step 1: read via exec
		raw, err := ExecRead(ctx, p.execRunner, botID, p.execWorkDir, filePath)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		// Step 2: fuzzy match in Go
		updated, err := applyEdit(raw, filePath, oldText, newText)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		// Step 3: write back via exec
		if err := ExecWrite(ctx, p.execRunner, botID, p.execWorkDir, filePath, updated); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"ok": true}), nil

	case toolExec:
		command := strings.TrimSpace(mcpgw.StringArg(arguments, "command"))
		if command == "" {
			return mcpgw.BuildToolErrorResult("command is required"), nil
		}
		workDir := strings.TrimSpace(mcpgw.StringArg(arguments, "work_dir"))
		if workDir == "" {
			workDir = p.execWorkDir
		}
		result, err := p.execRunner.ExecWithCapture(ctx, mcpgw.ExecRequest{
			BotID:   botID,
			Command: []string{shellCommandName, shellCommandFlag, command},
			WorkDir: workDir,
		})
		if err != nil {
			p.logger.Warn("exec failed", slog.String("bot_id", botID), slog.String("command", command), slog.Any("error", err))
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		stderr := result.Stderr
		if result.ExitCode != 0 && strings.Contains(stderr, "no running task") {
			stderr = strings.TrimSpace(stderr) + "\n\nHint: Container exists but has no running task (main process exited). Start it first: POST /bots/" + botID + "/container/start or use the container start action in the UI."
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{
			"stdout":    result.Stdout,
			"stderr":    stderr,
			"exit_code": result.ExitCode,
		}), nil

	default:
		return nil, mcpgw.ErrToolNotFound
	}
}
