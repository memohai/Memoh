package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

// blockedSleepPattern matches standalone `sleep N` where N >= 2.
// Does not match sleep inside pipelines, subshells, or scripts.
var blockedSleepPattern = regexp.MustCompile(`^sleep\s+(\d+(?:\.\d+)?)(?:\s*[;&]|$)`)

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
	bgManager   *background.Manager
	execWorkDir string
	logger      *slog.Logger
}

func NewContainerProvider(log *slog.Logger, clients bridge.Provider, bgManager *background.Manager, execWorkDir string) *ContainerProvider {
	if log == nil {
		log = slog.Default()
	}
	wd := strings.TrimSpace(execWorkDir)
	if wd == "" {
		wd = defaultContainerExecWorkDir
	}
	return &ContainerProvider{clients: clients, bgManager: bgManager, execWorkDir: wd, logger: log.With(slog.String("tool", "container"))}
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
			Name: "exec",
			Description: fmt.Sprintf(`Execute a shell command in the bot container. Runs in the bot's data directory (%s) by default.

# Instructions
- Use this tool to run shell commands for installing packages, running scripts, building code, running tests, and other system operations.
- If your command will take a long time (package installs, builds, test suites), set run_in_background to true. You will be notified when it completes. You do not need to add '&' at the end of the command when using this parameter.
- If waiting for a background task, you will be notified when it completes — do NOT poll or sleep.
- You may specify a custom timeout (up to %d seconds) for commands you know will take longer than the default %d seconds. If a foreground command times out, it will be automatically moved to the background and you will be notified when it completes.
- Avoid unnecessary sleep commands:
  - Do not sleep between commands that can run immediately — just run them.
  - If your command is long running, use run_in_background. No sleep needed.
  - Do not retry failing commands in a sleep loop — diagnose the root cause.
  - If waiting for a background task, you will be notified when it completes automatically.
  - sleep N (N >= 2) in foreground is blocked. If you genuinely need a short delay, keep it under 2 seconds.`, wd, background.MaxExecTimeout, background.DefaultExecTimeout),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":           map[string]any{"type": "string", "description": "Shell command to run (e.g. ls -la, npm install, python script.py)"},
					"work_dir":          map[string]any{"type": "string", "description": fmt.Sprintf("Working directory inside the container (default: %s)", wd)},
					"description":       map[string]any{"type": "string", "description": `Clear, concise description of what this command does in active voice. For simple commands keep it brief (5-10 words): ls -la → "List files with details". For complex commands add enough context: curl -s url | jq '.data[]' → "Fetch JSON and extract data array".`},
					"timeout":           map[string]any{"type": "integer", "description": fmt.Sprintf("Timeout in seconds (default: %d, max: %d). Only applies to foreground execution. Commands that exceed this timeout are automatically moved to background.", background.DefaultExecTimeout, background.MaxExecTimeout), "minimum": 1, "maximum": background.MaxExecTimeout},
					"run_in_background": map[string]any{"type": "boolean", "description": "If true, run the command in the background. Returns immediately with a task ID. You will be notified when it completes. Use for long-running commands (installs, builds, test suites). You do not need to use '&' at the end of the command."},
				},
				"required": []string{"command"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execExec(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "bg_status",
			Description: "Check the status of background tasks or kill a running one.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":  map[string]any{"type": "string", "enum": []string{"list", "status", "kill"}, "description": "Action to perform: list all tasks, get status of one task, or kill a running task"},
					"task_id": map[string]any{"type": "string", "description": "Task ID (required for status and kill actions)"},
				},
				"required": []string{"action"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execBgStatus(ctx.Context, sess, inputAsMap(input))
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
	description := strings.TrimSpace(StringArg(args, "description"))

	// Parse timeout (default 30s, max 600s).
	timeout := background.DefaultExecTimeout
	if t, ok, err := IntArg(args, "timeout"); err != nil {
		return nil, fmt.Errorf("invalid timeout: %w", err)
	} else if ok {
		if t < 1 {
			return nil, errors.New("timeout must be >= 1")
		}
		maxTimeout := int(background.MaxExecTimeout)
		if t > maxTimeout {
			t = maxTimeout
		}
		timeout = int32(t) //nolint:gosec // bounded above
	}

	// Block sleep N (N>=2) in foreground — nudge model toward run_in_background.
	runInBg, _, _ := BoolArg(args, "run_in_background")
	if !runInBg {
		if reason := detectBlockedSleep(command); reason != "" {
			return nil, fmt.Errorf("blocked: %s. Run blocking commands in the background with run_in_background: true — you'll get a completion notification when done. If you genuinely need a delay (rate limiting, deliberate pacing), keep it under 2 seconds", reason)
		}
	}

	// Background execution path.
	if runInBg && p.bgManager != nil {
		return p.execExecBackground(ctx, session, client, command, workDir, description)
	}

	// If we have a background manager, use streaming exec so we can flip
	// to background on timeout without killing the process.
	if p.bgManager != nil {
		return p.execExecWithFlip(ctx, session, client, command, workDir, description, timeout)
	}

	// Fallback: no background manager, plain synchronous exec.
	result, err := client.Exec(ctx, command, workDir, timeout)
	if err != nil {
		return nil, err
	}
	stdout := pruneToolOutputText(result.Stdout, "tool result (exec stdout)")
	stderr := pruneToolOutputText(result.Stderr, "tool result (exec stderr)")
	return map[string]any{"stdout": stdout, "stderr": stderr, "exit_code": result.ExitCode}, nil
}

// execExecWithFlip runs a command via ExecStream with a client-side soft timeout.
// If the command finishes within the timeout, it returns the result normally.
// If the soft timeout fires first, the running stream is handed off to the
// background manager — the process keeps running in the container, and the
// agent gets an immediate "auto_backgrounded" response.
func (p *ContainerProvider) execExecWithFlip(
	ctx context.Context, session SessionContext, client *bridge.Client,
	command, workDir, description string, softTimeout int32,
) (any, error) {
	// Start streaming exec with a large container-side timeout so the process
	// keeps running even after we stop reading in the foreground.
	// Use a fully independent context (not derived from the agent request ctx)
	// so the gRPC stream is never cancelled when the foreground session ends.
	streamCtx, streamCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(background.BackgroundExecTimeout)*time.Second)
	stream, err := client.ExecStream(streamCtx, command, workDir, background.BackgroundExecTimeout)
	if err != nil {
		streamCancel()
		return nil, err
	}

	resultCh := make(chan background.AdoptResult, 1)
	go func() {
		defer streamCancel()
		var stdout, stderr strings.Builder
		var exitCode int32
		for {
			msg, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				p.logger.Warn("flip-to-background: stream recv error",
					slog.String("command", truncateStr(command, 80)),
					slog.Any("error", recvErr),
				)
				resultCh <- background.AdoptResult{Err: recvErr}
				return
			}
			switch msg.GetStream() {
			case pb.ExecOutput_STDOUT:
				stdout.Write(msg.GetData())
			case pb.ExecOutput_STDERR:
				stderr.Write(msg.GetData())
			case pb.ExecOutput_EXIT:
				exitCode = msg.GetExitCode()
			}
		}
		resultCh <- background.AdoptResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}
	}()

	// Wait for either the result or soft timeout.
	timer := time.NewTimer(time.Duration(softTimeout) * time.Second)
	defer timer.Stop()

	select {
	case r := <-resultCh:
		// Command finished within the soft timeout — return normally.
		if r.Err != nil {
			return nil, r.Err
		}
		stdout := pruneToolOutputText(r.Stdout, "tool result (exec stdout)")
		stderr := pruneToolOutputText(r.Stderr, "tool result (exec stderr)")
		return map[string]any{"stdout": stdout, "stderr": stderr, "exit_code": r.ExitCode}, nil

	case <-timer.C:
		// Soft timeout fired — flip the running stream to background.
		// The container process is still alive; we hand off the stream reader
		// goroutine to the background manager.
		return p.flipToBackground(ctx, session, client, resultCh, command, workDir, description, softTimeout)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// flipToBackground registers the already-running stream as a background task.
// The goroutine reading from the stream continues; its result feeds the task.
func (p *ContainerProvider) flipToBackground(
	ctx context.Context,
	session SessionContext, client *bridge.Client,
	resultCh <-chan background.AdoptResult,
	command, workDir, description string, softTimeout int32,
) (any, error) {
	writeFn := func(ctx context.Context, path string, data []byte) error {
		return client.WriteFile(ctx, path, data)
	}

	taskID, outputFile := p.bgManager.SpawnAdopt(
		ctx,
		session.BotID, session.SessionID, command, workDir, description,
		resultCh, writeFn,
	)

	p.logger.Info("foreground exec flipped to background",
		slog.String("task_id", taskID),
		slog.String("command", truncateStr(command, 120)),
		slog.Int("soft_timeout_seconds", int(softTimeout)),
	)

	return map[string]any{
		"status":      "auto_backgrounded",
		"task_id":     taskID,
		"output_file": outputFile,
		"message": fmt.Sprintf(
			"Command exceeded the foreground timeout (%ds) and has been moved to the background with task ID: %s. "+
				"The process is still running — no work was lost. "+
				"You will be notified when it completes. Output is being written to: %s. "+
				"For long-running commands, use run_in_background: true from the start to avoid this delay.",
			softTimeout, taskID, outputFile,
		),
	}, nil
}

// detectBlockedSleep checks if the command starts with `sleep N` where N >= 2.
// Returns a human-readable reason string, or "" if the command is allowed.
func detectBlockedSleep(command string) string {
	cmd := strings.TrimSpace(command)
	m := blockedSleepPattern.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	seconds, err := strconv.ParseFloat(m[1], 64)
	if err != nil || seconds < 2 {
		return ""
	}
	return fmt.Sprintf("sleep %.0f is not allowed in foreground execution", seconds)
}

// spawnBackground is the shared helper that registers a background task and
// returns (taskID, outputFile). Used by both explicit and auto-background paths.
func (p *ContainerProvider) spawnBackground(
	ctx context.Context,
	session SessionContext, client *bridge.Client,
	command, workDir, description string,
) (taskID, outputFile string) {
	execFn := func(ctx context.Context, cmd, wd string, timeout int32) (*bridge.ExecResult, error) {
		return client.Exec(ctx, cmd, wd, timeout)
	}
	writeFn := func(ctx context.Context, path string, data []byte) error {
		return client.WriteFile(ctx, path, data)
	}
	readFn := func(ctx context.Context, path string) ([]byte, error) {
		// Use pool to get a fresh client — the original client may be in a failed
		// state if the streaming exec errored, but the pool will re-dial as needed.
		c, err := p.clients.MCPClient(ctx, session.BotID)
		if err != nil {
			return nil, err
		}
		resp, err := c.ReadFile(ctx, path, 1, 10)
		if err != nil {
			return nil, err
		}
		return []byte(resp.GetContent()), nil
	}

	taskID, outputFile = p.bgManager.Spawn(
		ctx,
		session.BotID, session.SessionID, command, workDir, description,
		execFn, writeFn, readFn,
	)
	return taskID, outputFile
}

// execExecBackground spawns the command as a background task and returns immediately.
func (p *ContainerProvider) execExecBackground(
	ctx context.Context, session SessionContext, client *bridge.Client,
	command, workDir, description string,
) (any, error) {
	taskID, outputFile := p.spawnBackground(ctx, session, client, command, workDir, description)

	return map[string]any{
		"status":      "background_started",
		"task_id":     taskID,
		"output_file": outputFile,
		"message":     fmt.Sprintf("Command started in background with task ID: %s. You will be notified when it completes. Output is being written to: %s. Do NOT poll or sleep — you will receive a notification automatically.", taskID, outputFile),
	}, nil
}

// execBgStatus handles the bg_status tool for listing/checking/killing background tasks.
func (p *ContainerProvider) execBgStatus(_ context.Context, session SessionContext, args map[string]any) (any, error) {
	if p.bgManager == nil {
		return nil, errors.New("background task manager not available")
	}

	action := strings.TrimSpace(StringArg(args, "action"))
	taskID := strings.TrimSpace(StringArg(args, "task_id"))

	switch action {
	case "list":
		tasks := p.bgManager.ListForSession(session.BotID, session.SessionID)
		entries := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			entries = append(entries, map[string]any{
				"task_id":     t.ID,
				"command":     truncateStr(t.Command, 120),
				"description": t.Description,
				"status":      string(t.Status),
				"output_file": t.OutputFile,
				"started_at":  session.FormatTime(t.StartedAt),
			})
		}
		return map[string]any{"tasks": entries, "count": len(entries)}, nil

	case "status":
		if taskID == "" {
			return nil, errors.New("task_id is required for status action")
		}
		task := p.bgManager.GetForSession(session.BotID, session.SessionID, taskID)
		if task == nil {
			return nil, fmt.Errorf("task %s not found", taskID)
		}
		result := map[string]any{
			"task_id":     task.ID,
			"command":     task.Command,
			"description": task.Description,
			"status":      string(task.Status),
			"output_file": task.OutputFile,
			"started_at":  session.FormatTime(task.StartedAt),
		}
		if task.Status != background.TaskRunning {
			result["exit_code"] = task.ExitCode
			result["completed_at"] = session.FormatTime(task.CompletedAt)
			result["output_tail"] = task.OutputTail()
		}
		return result, nil

	case "kill":
		if taskID == "" {
			return nil, errors.New("task_id is required for kill action")
		}
		if err := p.bgManager.KillForSession(session.BotID, session.SessionID, taskID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "message": fmt.Sprintf("Task %s has been killed.", taskID)}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s (expected: list, status, kill)", action)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
