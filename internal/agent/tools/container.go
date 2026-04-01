package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const defaultContainerExecWorkDir = "/data"

type ContainerProvider struct {
	clients     bridge.Provider
	execWorkDir string
	logger      *slog.Logger
}

func NewContainerProvider(log *slog.Logger, clients bridge.Provider, execWorkDir string) *ContainerProvider {
	if log == nil {
		log = slog.Default()
	}
	wd := strings.TrimSpace(execWorkDir)
	if wd == "" {
		wd = defaultContainerExecWorkDir
	}
	return &ContainerProvider{clients: clients, execWorkDir: wd, logger: log.With(slog.String("tool", "container"))}
}

func (p *ContainerProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	wd := p.execWorkDir
	sess := session
	return []sdk.Tool{
		{
			Name:        "read",
			Description: fmt.Sprintf("Read file content inside the bot container. Supports pagination for large files. Max %d lines / %d bytes per call.", readMaxLines, readMaxBytes),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"line_offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-indexed). Default: 1.", "minimum": 1, "default": 1},
					"n_lines":     map[string]any{"type": "integer", "description": fmt.Sprintf("Number of lines to read per call. Default: %d. Max: %d.", readMaxLines, readMaxLines), "minimum": 1, "maximum": readMaxLines, "default": readMaxLines},
				},
				"required": []string{"path"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execRead(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "write",
			Description: "Write file content inside the bot container.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"content": map[string]any{"type": "string", "description": "File content"},
				},
				"required": []string{"path", "content"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execWrite(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "list",
			Description: fmt.Sprintf("List directory entries inside the bot container. Supports pagination. Max %d entries per call. In recursive mode, subdirectories with >%d items are collapsed to a summary.", listMaxEntries, listCollapseThreshold),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": fmt.Sprintf("Directory path (relative to %s or absolute inside container)", wd)},
					"recursive": map[string]any{"type": "boolean", "description": "List recursively"},
					"offset":    map[string]any{"type": "integer", "description": "Entry offset to start from (0-indexed). Default: 0.", "minimum": 0, "default": 0},
					"limit":     map[string]any{"type": "integer", "description": fmt.Sprintf("Max entries to return per call. Default: %d. Max: %d.", listMaxEntries, listMaxEntries), "minimum": 1, "maximum": listMaxEntries, "default": listMaxEntries},
				},
				"required": []string{"path"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execList(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "edit",
			Description: "Replace exact text in a file inside the bot container.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"old_text": map[string]any{"type": "string", "description": "Exact text to find"},
					"new_text": map[string]any{"type": "string", "description": "Replacement text"},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execEdit(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "exec",
			Description: fmt.Sprintf("Execute a command in the bot container. Runs in the bot's data directory (%s) by default.", wd),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":  map[string]any{"type": "string", "description": "Shell command to run (e.g. ls -la, cat file.txt)"},
					"work_dir": map[string]any{"type": "string", "description": fmt.Sprintf("Working directory inside the container (default: %s)", wd)},
				},
				"required": []string{"command"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execExec(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *ContainerProvider) normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	prefix := p.execWorkDir
	if prefix == "" {
		prefix = defaultContainerExecWorkDir
	}
	if path == prefix {
		return "."
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimLeft(strings.TrimPrefix(path, prefix+"/"), "/")
	}
	return path
}

func (p *ContainerProvider) getClient(ctx context.Context, botID string) (*bridge.Client, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	client, err := p.clients.MCPClient(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("container not reachable: %w", err)
	}
	return client, nil
}

func (p *ContainerProvider) execRead(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	client, err := p.getClient(ctx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	if filePath == "" {
		return nil, errors.New("path is required")
	}
	lineOffset := int32(1)
	if offset, ok, err := IntArg(args, "line_offset"); err != nil {
		return nil, fmt.Errorf("invalid line_offset: %w", err)
	} else if ok {
		if offset < 1 {
			return nil, errors.New("line_offset must be >= 1")
		}
		if offset > math.MaxInt32 {
			return nil, errors.New("line_offset exceeds maximum")
		}
		lineOffset = int32(offset)
	}
	nLines := int32(readMaxLines)
	if n, ok, err := IntArg(args, "n_lines"); err != nil {
		return nil, fmt.Errorf("invalid n_lines: %w", err)
	} else if ok {
		if n < 1 {
			return nil, errors.New("n_lines must be >= 1")
		}
		if n > readMaxLines {
			n = readMaxLines
		}
		nLines = int32(n) //nolint:gosec // bounded by readMaxLines
	}
	resp, err := client.ReadFile(ctx, filePath, lineOffset, nLines)
	if err != nil {
		return nil, err
	}
	if resp.GetBinary() {
		return nil, errors.New("file appears to be binary. Read tool only supports text files")
	}
	content := addLineNumbers(resp.GetContent(), lineOffset)
	return map[string]any{"content": content, "total_lines": resp.GetTotalLines()}, nil
}

func (p *ContainerProvider) execWrite(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	client, err := p.getClient(ctx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	content := StringArg(args, "content")
	if filePath == "" {
		return nil, errors.New("path is required")
	}
	if err := client.WriteFile(ctx, filePath, []byte(content)); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (p *ContainerProvider) execList(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	client, err := p.getClient(ctx, session.BotID)
	if err != nil {
		return nil, err
	}
	dirPath := p.normalizePath(StringArg(args, "path"))
	if dirPath == "" {
		dirPath = "."
	}
	recursive, _, _ := BoolArg(args, "recursive")

	offset := int32(0)
	if v, ok, err := IntArg(args, "offset"); err != nil {
		return nil, fmt.Errorf("invalid offset: %w", err)
	} else if ok {
		if v < 0 {
			return nil, errors.New("offset must be >= 0")
		}
		if v > math.MaxInt32 {
			return nil, errors.New("offset exceeds maximum")
		}
		offset = int32(v) //nolint:gosec // bounded above
	}

	limit := int32(listMaxEntries)
	if v, ok, err := IntArg(args, "limit"); err != nil {
		return nil, fmt.Errorf("invalid limit: %w", err)
	} else if ok {
		if v < 1 {
			return nil, errors.New("limit must be >= 1")
		}
		if v > listMaxEntries {
			v = listMaxEntries
		}
		limit = int32(v) //nolint:gosec // bounded by listMaxEntries
	}

	var collapseThreshold int32
	if recursive {
		collapseThreshold = listCollapseThreshold
	}

	result, err := client.ListDir(ctx, dirPath, recursive, offset, limit, collapseThreshold)
	if err != nil {
		return nil, err
	}

	entriesMaps := make([]map[string]any, 0, len(result.Entries))
	for _, e := range result.Entries {
		m := map[string]any{
			"path": e.GetPath(), "is_dir": e.GetIsDir(), "size": e.GetSize(),
			"mode": e.GetMode(), "mod_time": e.GetModTime(),
		}
		if s := e.GetSummary(); s != "" {
			m["summary"] = s
		}
		entriesMaps = append(entriesMaps, m)
	}

	return map[string]any{
		"path":        dirPath,
		"entries":     entriesMaps,
		"total_count": result.TotalCount,
		"truncated":   result.Truncated,
		"offset":      offset,
		"limit":       limit,
	}, nil
}

func (p *ContainerProvider) execEdit(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	client, err := p.getClient(ctx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	oldText := StringArg(args, "old_text")
	newText := StringArg(args, "new_text")
	if filePath == "" || oldText == "" {
		return nil, errors.New("path, old_text and new_text are required")
	}
	reader, err := client.ReadRaw(ctx, filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	updated, err := applyEdit(string(raw), filePath, oldText, newText)
	if err != nil {
		return nil, err
	}
	if err := client.WriteFile(ctx, filePath, []byte(updated)); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (p *ContainerProvider) execExec(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	client, err := p.getClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	command := strings.TrimSpace(StringArg(args, "command"))
	if command == "" {
		return nil, errors.New("command is required")
	}
	workDir := strings.TrimSpace(StringArg(args, "work_dir"))
	if workDir == "" {
		workDir = p.execWorkDir
	}
	result, err := client.Exec(ctx, command, workDir, 30)
	if err != nil {
		return nil, err
	}
	stdout := pruneToolOutputText(result.Stdout, "tool result (exec stdout)")
	stderr := pruneToolOutputText(result.Stderr, "tool result (exec stderr)")
	return map[string]any{"stdout": stdout, "stderr": stderr, "exit_code": result.ExitCode}, nil
}

func addLineNumbers(content string, startLine int32) string {
	if content == "" {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	var out strings.Builder
	out.Grow(len(content) + len(lines)*8)
	for i, line := range lines {
		fmt.Fprintf(&out, "%6d\t%s\n", int(startLine)+i, line)
	}
	return out.String()
}
