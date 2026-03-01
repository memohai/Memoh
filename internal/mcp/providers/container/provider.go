package container

import (
	"context"
	"fmt"
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

// CallTool dispatches to the appropriate container-exec backed implementation.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	switch toolName {
	case toolRead:
		filePath := p.normalizePath(mcpgw.StringArg(arguments, "path"))
		if filePath == "" {
			return mcpgw.BuildToolErrorResult("path is required"), nil
		}

		// Parse optional pagination params.
		lineOffset := 1
		offset, ok, err := mcpgw.IntArg(arguments, "line_offset")
		if err != nil {
			return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid line_offset: %v", err)), nil
		}
		if ok {
			if offset < 1 {
				return mcpgw.BuildToolErrorResult("line_offset must be >= 1"), nil
			}
			lineOffset = offset
		}

		nLines := readMaxLines
		n, ok, err := mcpgw.IntArg(arguments, "n_lines")
		if err != nil {
			return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid n_lines: %v", err)), nil
		}
		if ok {
			if n < 1 {
				return mcpgw.BuildToolErrorResult("n_lines must be >= 1"), nil
			}
			if n > readMaxLines {
				n = readMaxLines
			}
			nLines = n
		}

		result, err := ReadFile(ctx, p.execRunner, botID, p.execWorkDir, filePath, lineOffset, nLines)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}

		output := FormatReadResult(result)

		return mcpgw.BuildToolSuccessResult(map[string]any{
			"content": output,
		}), nil

	case toolWrite:
		filePath := p.normalizePath(mcpgw.StringArg(arguments, "path"))
		content := mcpgw.StringArg(arguments, "content")
		if filePath == "" {
			return mcpgw.BuildToolErrorResult("path is required"), nil
		}
		if err := ExecWrite(ctx, p.execRunner, botID, p.execWorkDir, filePath, content); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"ok": true}), nil

	case toolList:
		dirPath := p.normalizePath(mcpgw.StringArg(arguments, "path"))
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
		filePath := p.normalizePath(mcpgw.StringArg(arguments, "path"))
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
		wrappedCmd := command
		if workDir != "" {
			wrappedCmd = "cd " + ShellQuote(workDir) + " && " + command
		}
		result, err := p.execRunner.ExecWithCapture(ctx, mcpgw.ExecRequest{
			BotID:   botID,
			Command: []string{shellCommandName, shellCommandFlag, wrappedCmd},
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
		stdout := pruneToolOutputText(result.Stdout, "tool result (exec stdout)")
		stderr = pruneToolOutputText(stderr, "tool result (exec stderr)")
		return mcpgw.BuildToolSuccessResult(map[string]any{
			"stdout":    stdout,
			"stderr":    stderr,
			"exit_code": result.ExitCode,
		}), nil

	default:
		return nil, mcpgw.ErrToolNotFound
	}
}
