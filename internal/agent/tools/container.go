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
	"sync"
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

func (p *ContainerProvider) Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error) {
	workspace := p.resolveToolWorkspace(ctx, session)
	wd := workspace.defaultWorkDir
	sess := session

	readDesc := fmt.Sprintf("Read file content %s. Reads the full file by default; use line_offset and n_lines for pagination. Files up to ~16 MB are supported.", workspace.locationDescription)
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
					"path":        map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute %s)", wd, workspace.absolutePathDescription)},
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
			Description: fmt.Sprintf("Write file content %s. Creates parent directories automatically. Handles files of any size.", workspace.locationDescription),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute %s)", wd, workspace.absolutePathDescription)},
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
			Description: fmt.Sprintf("List directory entries %s. Supports pagination. Max %d entries per call. In recursive mode, subdirectories with >%d items are collapsed to a summary.", workspace.locationDescription, listMaxEntries, listCollapseThreshold),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": fmt.Sprintf("Directory path (relative to %s or absolute %s)", wd, workspace.absolutePathDescription)},
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
			Description: fmt.Sprintf("Replace exact text in a file %s.", workspace.locationDescription),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": fmt.Sprintf("File path (relative to %s or absolute %s)", wd, workspace.absolutePathDescription)},
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
			Name: "apply_patch",
			Description: fmt.Sprintf(`Apply a structured patch %s. This is a Memoh/Codex-style patch format, not a standard unified diff or git patch.

Use this tool for multi-file edits, structured code changes, file creation, file deletion, or file moves. Use edit for one exact text replacement and write for full-file overwrite.

Patch grammar:
- The patch must start with "*** Begin Patch" and end with "*** End Patch".
- Add a file with "*** Add File: path". Every following content line for that file must start with "+".
- Delete a file with "*** Delete File: path".
- Update a file with "*** Update File: path".
- Move or rename a file by putting "*** Move to: new/path" immediately after an update header, before any changed lines.
- Inside update hunks, use "@@" or "@@ context line" before changed lines. The optional context line helps locate the block in the file.
- Changed lines use a one-character prefix: " " for unchanged context, "-" for removed lines, and "+" for added lines.
- Use "*** End of File" inside an update hunk when the hunk should match the end of the file.
- Paths are relative to the workspace by default. Absolute paths are accepted only when the workspace backend allows them.

Examples:

Add a file:
*** Begin Patch
*** Add File: docs/notes.md
+new line
*** End Patch

Update a file:
*** Begin Patch
*** Update File: path
@@
-old line
+new line
*** End Patch

Move a file:
*** Begin Patch
*** Update File: old/path.txt
*** Move to: new/path.txt
@@
 old content
*** End Patch

Delete a file:
*** Begin Patch
*** Delete File: obsolete.txt
*** End Patch
`, workspace.locationDescription),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patch": map[string]any{"type": "string", "description": "Patch body using the apply_patch format. Paths are relative to the workspace by default, or absolute paths supported by the workspace backend."},
				},
				"required": []string{"patch"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execApplyPatch(ctx.Context, sess, input)
			},
		},
		{
			Name: "exec",
			Description: fmt.Sprintf(`Execute a shell command %s. Runs in %s by default.

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
  - sleep N (N >= 2) in foreground is blocked. If you genuinely need a short delay, keep it under 2 seconds.`, workspace.locationDescription, wd, background.MaxExecTimeout, background.DefaultExecTimeout),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":           map[string]any{"type": "string", "description": "Shell command to run (e.g. ls -la, npm install, python script.py)"},
					"work_dir":          map[string]any{"type": "string", "description": fmt.Sprintf("Working directory (default: %s)", wd)},
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
			Name:        "list_background",
			Description: "List background tasks for the current session.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execListBackground(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "get_background_status",
			Description: "Get the status and details of a background task.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "Task ID"},
				},
				"required": []string{"task_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execGetBackgroundStatus(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "kill_background",
			Description: "Kill a running background task.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "Task ID"},
				},
				"required": []string{"task_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execKillBackground(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

type toolWorkspace struct {
	defaultWorkDir          string
	locationDescription     string
	absolutePathDescription string
}

func (p *ContainerProvider) resolveToolWorkspace(ctx context.Context, session SessionContext) toolWorkspace {
	info := bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendContainer,
		DefaultWorkDir: p.execWorkDir,
	}
	if resolver, ok := p.clients.(bridge.WorkspaceInfoProvider); ok {
		if resolved, err := resolver.WorkspaceInfo(ctx, session.BotID); err == nil {
			info = resolved
		}
	}
	wd := strings.TrimSpace(info.DefaultWorkDir)
	if wd == "" {
		wd = p.execWorkDir
	}
	if strings.EqualFold(info.Backend, bridge.WorkspaceBackendLocal) {
		return toolWorkspace{
			defaultWorkDir:          wd,
			locationDescription:     "on the local machine",
			absolutePathDescription: "host path",
		}
	}
	return toolWorkspace{
		defaultWorkDir:          wd,
		locationDescription:     "inside the bot container",
		absolutePathDescription: "inside container",
	}
}

func (*ContainerProvider) normalizePath(path, workDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	prefix := strings.TrimRight(strings.TrimSpace(workDir), "/")
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
	filePath := p.normalizePath(StringArg(args, "path"), p.resolveToolWorkspace(ctx, session).defaultWorkDir)
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

	content = addLineNumbers(content, lineOffset)
	return map[string]any{"content": content, "total_lines": totalLines}, nil
}

func (p *ContainerProvider) execWrite(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	opCtx, opCancel := context.WithTimeout(ctx, containerOpTimeout)
	defer opCancel()

	client, err := p.getClient(opCtx, session.BotID)
	if err != nil {
		return nil, err
	}
	filePath := p.normalizePath(StringArg(args, "path"), p.resolveToolWorkspace(ctx, session).defaultWorkDir)
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
	dirPath := p.normalizePath(StringArg(args, "path"), p.resolveToolWorkspace(ctx, session).defaultWorkDir)
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
	filePath := p.normalizePath(StringArg(args, "path"), p.resolveToolWorkspace(ctx, session).defaultWorkDir)
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
		workDir = p.resolveToolWorkspace(ctx, session).defaultWorkDir
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

const backgroundReplayBytes = 4096

type backgroundExecStreamReader struct {
	resultCh chan background.AdoptResult
	logger   *slog.Logger
	command  string

	mu             sync.Mutex
	stdout         strings.Builder
	stderr         strings.Builder
	chunkHandler   func(stream, chunk string)
	outputRecorded bool
}

func startBackgroundExecStreamReader(log *slog.Logger, stream *bridge.ExecStream, cancel context.CancelFunc, command string) *backgroundExecStreamReader {
	reader := &backgroundExecStreamReader{
		resultCh: make(chan background.AdoptResult, 1),
		logger:   log,
		command:  command,
	}
	go reader.run(stream, cancel)
	return reader
}

func (r *backgroundExecStreamReader) run(stream *bridge.ExecStream, cancel context.CancelFunc) {
	defer cancel()
	var exitCode int32
	var exitReceived bool
	for {
		msg, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			r.logger.Warn("background exec stream recv error",
				slog.String("command", truncateStr(r.command, 80)),
				slog.Any("error", recvErr),
			)
			stdout, stderr, outputRecorded := r.snapshot()
			r.resultCh <- background.AdoptResult{
				Stdout:         stdout,
				Stderr:         stderr,
				ExitCode:       exitCode,
				ExitReceived:   exitReceived,
				Err:            recvErr,
				OutputRecorded: outputRecorded,
			}
			return
		}
		switch msg.GetStream() {
		case pb.ExecOutput_STDOUT:
			r.appendChunk("stdout", string(msg.GetData()))
		case pb.ExecOutput_STDERR:
			r.appendChunk("stderr", string(msg.GetData()))
		case pb.ExecOutput_EXIT:
			exitCode = msg.GetExitCode()
			exitReceived = true
		}
	}

	stdout, stderr, outputRecorded := r.snapshot()
	r.resultCh <- background.AdoptResult{
		Stdout:         stdout,
		Stderr:         stderr,
		ExitCode:       exitCode,
		ExitReceived:   exitReceived,
		OutputRecorded: outputRecorded,
	}
}

func (r *backgroundExecStreamReader) snapshot() (stdout, stderr string, outputRecorded bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stdout.String(), r.stderr.String(), r.outputRecorded
}

func (r *backgroundExecStreamReader) appendChunk(stream, chunk string) {
	if chunk == "" {
		return
	}
	r.mu.Lock()
	switch stream {
	case "stderr":
		r.stderr.WriteString(chunk)
	default:
		r.stdout.WriteString(chunk)
	}
	handler := r.chunkHandler
	r.mu.Unlock()
	if handler != nil {
		handler(stream, chunk)
	}
}

func (r *backgroundExecStreamReader) SetChunkHandler(handler func(stream, chunk string)) {
	r.mu.Lock()
	r.chunkHandler = handler
	r.outputRecorded = handler != nil
	stdout := tailText(r.stdout.String(), backgroundReplayBytes)
	stderr := tailText(r.stderr.String(), backgroundReplayBytes)
	r.mu.Unlock()

	if handler == nil {
		return
	}
	if stdout != "" {
		handler("stdout", stdout)
	}
	if stderr != "" {
		handler("stderr", stderr)
	}
}

func (r *backgroundExecStreamReader) Result() <-chan background.AdoptResult {
	return r.resultCh
}

func tailText(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[len(value)-maxBytes:]
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
	reader := startBackgroundExecStreamReader(p.logger, stream, streamCancel, command)

	// Wait for either the result or soft timeout.
	timer := time.NewTimer(time.Duration(softTimeout) * time.Second)
	defer timer.Stop()

	select {
	case r := <-reader.Result():
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
		return p.flipToBackground(ctx, session, client, reader, command, workDir, description, softTimeout)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// flipToBackground registers the already-running stream as a background task.
// The goroutine reading from the stream continues; its result feeds the task.
func (p *ContainerProvider) flipToBackground(
	ctx context.Context,
	session SessionContext, client *bridge.Client,
	reader *backgroundExecStreamReader,
	command, workDir, description string, softTimeout int32,
) (any, error) {
	writeFn := func(ctx context.Context, path string, data []byte) error {
		return client.WriteFile(ctx, path, data)
	}

	taskID, outputFile := p.bgManager.SpawnAdopt(
		ctx,
		session.BotID, session.SessionID, command, workDir, description,
		reader.Result(), writeFn,
	)
	reader.SetChunkHandler(func(stream, chunk string) {
		p.bgManager.RecordOutput(taskID, stream, chunk)
	})

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

// execExecBackground spawns the command as a background task and returns immediately.
func (p *ContainerProvider) execExecBackground(
	ctx context.Context, session SessionContext, client *bridge.Client,
	command, workDir, description string,
) (any, error) {
	streamCtx, streamCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(background.BackgroundExecTimeout)*time.Second)
	stream, err := client.ExecStream(streamCtx, command, workDir, background.BackgroundExecTimeout)
	if err != nil {
		streamCancel()
		return nil, err
	}
	reader := startBackgroundExecStreamReader(p.logger, stream, streamCancel, command)

	writeFn := func(ctx context.Context, path string, data []byte) error {
		return client.WriteFile(ctx, path, data)
	}
	taskID, outputFile := p.bgManager.SpawnAdopt(
		ctx,
		session.BotID, session.SessionID, command, workDir, description,
		reader.Result(), writeFn,
	)
	reader.SetChunkHandler(func(stream, chunk string) {
		p.bgManager.RecordOutput(taskID, stream, chunk)
	})

	return map[string]any{
		"status":      "background_started",
		"task_id":     taskID,
		"output_file": outputFile,
		"message":     fmt.Sprintf("Command started in background with task ID: %s. You will be notified when it completes. Output is being written to: %s. Do NOT poll or sleep — you will receive a notification automatically.", taskID, outputFile),
	}, nil
}

func (p *ContainerProvider) execListBackground(_ context.Context, session SessionContext, _ map[string]any) (any, error) {
	if p.bgManager == nil {
		return nil, errors.New("background task manager not available")
	}

	snapshots := p.bgManager.ListSnapshotsForSession(session.BotID, session.SessionID)
	entries := make([]map[string]any, 0, len(snapshots))
	for _, s := range snapshots {
		entry := map[string]any{
			"task_id":     s.TaskID,
			"kind":        string(s.Kind),
			"description": s.Description,
			"status":      string(s.Status),
			"started_at":  session.FormatTime(s.StartedAt),
		}
		if s.Kind == background.KindAgent {
			entry["agent_id"] = s.AgentID
			entry["session_id"] = s.AgentSessionID
		}
		if s.Kind != background.KindSpawn && s.Kind != background.KindAgent {
			entry["command"] = truncateStr(s.Command, 120)
			entry["output_file"] = s.OutputFile
		}
		entries = append(entries, entry)
	}
	return map[string]any{"tasks": entries, "count": len(entries)}, nil
}

func (p *ContainerProvider) execGetBackgroundStatus(_ context.Context, session SessionContext, args map[string]any) (any, error) {
	if p.bgManager == nil {
		return nil, errors.New("background task manager not available")
	}

	taskID := strings.TrimSpace(StringArg(args, "task_id"))
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	task := p.bgManager.GetForSession(session.BotID, session.SessionID, taskID)
	if task == nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	s := task.Snapshot()
	result := map[string]any{
		"task_id":     s.TaskID,
		"kind":        string(s.Kind),
		"description": s.Description,
		"status":      string(s.Status),
		"started_at":  session.FormatTime(s.StartedAt),
	}
	if s.Status != background.TaskRunning && s.Status != background.TaskQueued {
		result["completed_at"] = session.FormatTime(s.CompletedAt)
	}
	if s.Kind == background.KindAgent {
		result["agent_id"] = s.AgentID
		result["session_id"] = s.AgentSessionID
		result["message"] = s.AgentMessage
		if s.AgentReport != "" {
			result["report"] = s.AgentReport
		}
		if s.AgentError != "" {
			result["error"] = s.AgentError
		}
		return result, nil
	}
	if s.Kind == background.KindSpawn {
		if len(s.Branches) > 0 {
			branches := make([]map[string]any, 0, len(s.Branches))
			for _, br := range s.Branches {
				b := map[string]any{"task": br.Task, "status": string(br.Status)}
				if br.ChildSessionID != "" {
					b["session_id"] = br.ChildSessionID
				}
				if br.Report != "" {
					b["report"] = br.Report
				}
				if br.Error != "" {
					b["error"] = br.Error
				}
				branches = append(branches, b)
			}
			result["branches"] = branches
		}
		return result, nil
	}
	result["command"] = s.Command
	result["output_file"] = s.OutputFile
	if s.Status != background.TaskRunning && s.Status != background.TaskQueued {
		result["exit_code"] = s.ExitCode
		result["output_tail"] = s.OutputTail
	}
	return result, nil
}

func (p *ContainerProvider) execKillBackground(_ context.Context, session SessionContext, args map[string]any) (any, error) {
	if p.bgManager == nil {
		return nil, errors.New("background task manager not available")
	}

	taskID := strings.TrimSpace(StringArg(args, "task_id"))
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	if err := p.bgManager.KillForSession(session.BotID, session.SessionID, taskID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "message": fmt.Sprintf("Task %s has been killed.", taskID)}, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func addLineNumbers(content string, startLine int) string {
	if content == "" {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	var out strings.Builder
	out.Grow(len(content) + len(lines)*8)
	for i, line := range lines {
		fmt.Fprintf(&out, "%6d\t%s\n", startLine+i, line)
	}
	return out.String()
}
