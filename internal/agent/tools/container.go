package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const defaultContainerExecWorkDir = "/data"

// containerOpTimeout is the maximum time allowed for individual file
// operations (read, write, list, edit). Exec has its own timeout.
const containerOpTimeout = 30 * time.Second

// largeFileThreshold defines the size above which file operations use
// streaming (async chunked I/O) instead of loading fully into memory.
// Files <= this threshold use the simpler synchronous gRPC calls.
const largeFileThreshold = 512 * 1024 // 512 KB

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

	readDesc := "Read file content inside the bot container. Reads the full file by default; use line_offset and n_lines for pagination. Files up to ~16 MB are supported."
	if sess.SupportsImageInput {
		readDesc += " Also supports reading image files (PNG, JPEG, GIF, WebP) — binary images are loaded into model context automatically."
	}

	return []sdk.Tool{
		{
			Name:        "read",
			Description: readDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute inside container)", wd)},
					"line_offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-indexed). Default: 1.", "minimum": 1, "default": 1},
					"n_lines":     map[string]any{"type": "integer", "description": "Number of lines to read. Default: read entire file.", "minimum": 1},
				},
				"required": []string{"path"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execRead(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "write",
			Description: "Write file content inside the bot container. Creates parent directories automatically. Handles files of any size.",
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
	opCtx, opCancel := context.WithTimeout(ctx, containerOpTimeout)
	defer opCancel()

	client, err := p.getClient(opCtx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	if filePath == "" {
		return nil, errors.New("path is required")
	}

	lineOffset := 1
	if offset, ok, err := IntArg(args, "line_offset"); err != nil {
		return nil, fmt.Errorf("invalid line_offset: %w", err)
	} else if ok {
		if offset < 1 {
			return nil, errors.New("line_offset must be >= 1")
		}
		lineOffset = offset
	}
	nLines := 0 // 0 = read entire file
	if n, ok, err := IntArg(args, "n_lines"); err != nil {
		return nil, fmt.Errorf("invalid n_lines: %w", err)
	} else if ok && n > 0 {
		nLines = n
	}

	// Pre-check file size to avoid loading excessively large files into
	// memory. The gRPC transport is capped at 16 MB, so anything larger
	// would fail anyway; reject early with a clear message.
	const maxReadBytes = 16 * 1024 * 1024 // 16 MB
	if stat, err := client.Stat(opCtx, filePath); err == nil && stat != nil {
		if stat.GetSize() > maxReadBytes {
			return nil, fmt.Errorf("file is too large (%d bytes, limit %d bytes). Use exec with head/tail/sed for partial reads", stat.GetSize(), maxReadBytes)
		}
	}

	// Stream-read the full file content.
	reader, err := client.ReadRaw(opCtx, filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	// Probe for binary content.
	probe := make([]byte, 8*1024)
	probeN, probeErr := reader.Read(probe)
	if probeErr != nil && probeErr != io.EOF {
		return nil, fmt.Errorf("read probe: %w", probeErr)
	}
	if bytes.IndexByte(probe[:probeN], 0) >= 0 {
		if !session.SupportsImageInput {
			return nil, errors.New("file appears to be binary. Read tool only supports text files (image reading not available for this model)")
		}
		return ReadImageFromContainer(opCtx, client, filePath, defaultReadMediaMaxBytes), nil
	}

	// Read remaining content after probe.
	var buf strings.Builder
	buf.Write(probe[:probeN])
	if probeErr != io.EOF {
		remaining, readErr := io.ReadAll(reader)
		if readErr != nil {
			return nil, fmt.Errorf("read file: %w", readErr)
		}
		buf.Write(remaining)
	}

	fullContent := buf.String()
	lines := strings.Split(fullContent, "\n")
	totalLines := len(lines)

	// Apply line_offset and n_lines.
	start := lineOffset - 1 // convert to 0-based
	if start > totalLines {
		start = totalLines
	}
	end := totalLines
	if nLines > 0 && start+nLines < end {
		end = start + nLines
	}

	selectedLines := lines[start:end]
	content := strings.Join(selectedLines, "\n")
	if !strings.HasSuffix(content, "\n") && end < totalLines {
		content += "\n"
	}

	content = addLineNumbers(content, int32(lineOffset))
	return map[string]any{"content": content, "total_lines": totalLines}, nil
}

func (p *ContainerProvider) execWrite(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	opCtx, opCancel := context.WithTimeout(ctx, containerOpTimeout)
	defer opCancel()

	client, err := p.getClient(opCtx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	content := StringArg(args, "content")
	if filePath == "" {
		return nil, errors.New("path is required")
	}

	data := []byte(content)
	if len(data) > largeFileThreshold {
		// Large content: use streaming WriteRaw to avoid loading everything
		// into a single gRPC message and to allow incremental transfer.
		if _, err := client.WriteRaw(opCtx, filePath, strings.NewReader(content)); err != nil {
			return nil, err
		}
	} else {
		// Small content: simple synchronous write.
		if err := client.WriteFile(opCtx, filePath, data); err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true}, nil
}

func (p *ContainerProvider) execList(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	opCtx, opCancel := context.WithTimeout(ctx, containerOpTimeout)
	defer opCancel()

	client, err := p.getClient(opCtx, session.BotID)
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

	result, err := client.ListDir(opCtx, dirPath, recursive, offset, limit, collapseThreshold)
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
	opCtx, opCancel := context.WithTimeout(ctx, containerOpTimeout)
	defer opCancel()

	client, err := p.getClient(opCtx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"))
	oldText := StringArg(args, "old_text")
	newText := StringArg(args, "new_text")
	if filePath == "" || oldText == "" {
		return nil, errors.New("path, old_text and new_text are required")
	}

	// Read file content via streaming RPC.
	reader, err := client.ReadRaw(opCtx, filePath)
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

	updatedBytes := []byte(updated)
	if len(updatedBytes) > largeFileThreshold {
		// Large result: stream-write to avoid gRPC message size issues.
		if _, err := client.WriteRaw(opCtx, filePath, strings.NewReader(updated)); err != nil {
			return nil, err
		}
	} else {
		if err := client.WriteFile(opCtx, filePath, updatedBytes); err != nil {
			return nil, err
		}
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
