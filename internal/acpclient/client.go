package acpclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	DefaultRunTimeout          = 20 * time.Minute
	maxWriteToolContentPreview = 64 * 1024
)

type Workspace interface {
	bridge.Provider
	bridge.WorkspaceInfoProvider
}

type Runner struct {
	logger    *slog.Logger
	workspace Workspace
	command   string
	args      []string
	timeout   time.Duration
}

type RunRequest struct {
	AgentID      string
	BotID        string
	Task         string
	ProjectPath  string
	Command      string
	Args         []string
	LocalCommand string
	LocalArgs    []string
	Env          []string
	SetupMode    SetupMode
	Timeout      time.Duration
}

type RunResult struct {
	SessionID   string        `json:"session_id,omitempty"`
	ProjectPath string        `json:"project_path,omitempty"`
	Text        string        `json:"text,omitempty"`
	StopReason  string        `json:"stop_reason,omitempty"`
	Events      []StreamEvent `json:"events,omitempty"`
}

func NewRunner(log *slog.Logger, workspace Workspace) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{
		logger:    log.With(slog.String("component", "acpclient")),
		workspace: workspace,
		timeout:   DefaultRunTimeout,
	}
}

func (r *Runner) WorkspaceInfo(ctx context.Context, botID string) (bridge.WorkspaceInfo, error) {
	if r == nil || r.workspace == nil {
		return bridge.WorkspaceInfo{}, errors.New("ACP workspace provider is not configured")
	}
	return r.workspace.WorkspaceInfo(ctx, botID)
}

// Run is a convenience wrapper that performs a single-shot ACP exchange:
// start a session, send one prompt, then close. Production code that needs a
// persistent session should use StartSession + (*Session).Prompt directly.
//
// (*Session).Close uses its own short-lived background context so cleanup
// always runs even if the caller's ctx was cancelled; that disconnect trips
// contextcheck, so we silence it here.
//
//nolint:contextcheck // lifecycle close intentionally uses background ctx.
func (r *Runner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if strings.TrimSpace(req.Task) == "" {
		return RunResult{}, errors.New("task is required")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = r.timeout
	}
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sess, err := r.StartSession(runCtx, StartRequest{
		AgentID:      req.AgentID,
		BotID:        req.BotID,
		ProjectPath:  req.ProjectPath,
		Command:      req.Command,
		Args:         req.Args,
		LocalCommand: req.LocalCommand,
		LocalArgs:    req.LocalArgs,
		Env:          req.Env,
		SetupMode:    req.SetupMode,
		Timeout:      timeout,
	}, nil)
	if err != nil {
		return RunResult{}, err
	}
	defer func() { _ = sess.Close() }()

	prompt, err := sess.Prompt(runCtx, req.Task)
	result := RunResult{
		SessionID:   sess.ID(),
		ProjectPath: sess.ProjectPath(),
		Text:        prompt.Text,
		StopReason:  prompt.StopReason,
		Events:      prompt.Events,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func resolveWorkspacePaths(info bridge.WorkspaceInfo, rawProjectPath string) (string, string, WorkspaceBackend, error) {
	backend := WorkspaceBackendContainer
	if strings.EqualFold(info.Backend, bridge.WorkspaceBackendLocal) {
		backend = WorkspaceBackendLocal
	}
	root := strings.TrimSpace(info.DefaultWorkDir)
	if root == "" {
		root = dataMountPath
	}
	if backend == WorkspaceBackendLocal {
		resolvedRoot, err := resolveRoot(root)
		if err != nil {
			return "", "", backend, err
		}
		projectPath, err := ResolvePathUnderRoot(resolvedRoot, rawProjectPath)
		return resolvedRoot, projectPath, backend, err
	}
	root = dataMountPath
	projectPath, err := ResolvePathUnderVirtualRoot(root, rawProjectPath)
	return root, projectPath, backend, err
}

type clientCallbacks struct {
	client      *bridge.Client
	root        string
	cwd         string
	virtualRoot bool
	mu          sync.RWMutex
	collector   *eventCollector
	sink        EventSink
	events      *toolEventEmitter
	toolMapper  *acpToolEventMapper
	terminals   *terminalManager
}

func newClientCallbacks(ctx context.Context, client *bridge.Client, root, cwd string, timeout time.Duration, sink EventSink, env []string, virtualRoot bool) *clientCallbacks {
	timeoutSeconds := int32(timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTerminalTimeout
	}
	events := &toolEventEmitter{}
	return &clientCallbacks{
		client:      client,
		root:        root,
		cwd:         cwd,
		virtualRoot: virtualRoot,
		sink:        sink,
		events:      events,
		toolMapper:  newACPToolEventMapper(),
		terminals:   newTerminalManager(ctx, client, root, cwd, timeoutSeconds, env, virtualRoot, events),
	}
}

func (c *clientCallbacks) close() {
	if c != nil && c.terminals != nil {
		c.terminals.killAll()
	}
}

func (c *clientCallbacks) setPromptState(collector *eventCollector, sink EventSink) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.collector = collector
	c.sink = sink
	c.mu.Unlock()
	if c.events != nil {
		c.events.setPromptState(collector, sink)
	}
}

func (c *clientCallbacks) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	toolID := "read-" + uuid.NewString()
	input := map[string]any{"path": p.Path}
	if p.Line != nil && *p.Line > 0 {
		input["line"] = *p.Line
	}
	if p.Limit != nil && *p.Limit > 0 {
		input["limit"] = *p.Limit
	}
	c.emitToolCallStart(toolID, "read", input)
	var toolErr error
	defer func() {
		result := map[string]any{}
		if toolErr != nil {
			result = toolErrorResult(toolErr)
		}
		c.emitToolCallEnd(toolID, "read", input, result, toolErr)
	}()

	path, err := c.resolvePath(p.Path)
	if err != nil {
		toolErr = err
		return acp.ReadTextFileResponse{}, err
	}
	line := int32(1)
	if p.Line != nil && *p.Line > 0 {
		line = boundedPositiveInt32(*p.Line)
	}
	limit := int32(0)
	if p.Limit != nil && *p.Limit > 0 {
		limit = boundedPositiveInt32(*p.Limit)
	}
	resp, err := c.client.ReadFile(ctx, path, line, limit)
	if err != nil {
		toolErr = err
		return acp.ReadTextFileResponse{}, err
	}
	if resp.GetBinary() {
		toolErr = fmt.Errorf("path %q is binary; ACP text file reads only support text", p.Path)
		return acp.ReadTextFileResponse{}, toolErr
	}
	content := resp.GetContent()
	if content == "" {
		content = "\n"
	}
	return acp.ReadTextFileResponse{Content: content}, nil
}

func (c *clientCallbacks) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	toolID := "write-" + uuid.NewString()
	input := writeToolInput(p.Path, p.Content)
	c.emitToolCallStart(toolID, "write", input)
	var toolErr error
	defer func() {
		result := map[string]any{}
		if toolErr != nil {
			result = toolErrorResult(toolErr)
		}
		c.emitToolCallEnd(toolID, "write", input, result, toolErr)
	}()

	path, err := c.resolvePath(p.Path)
	if err != nil {
		toolErr = err
		return acp.WriteTextFileResponse{}, err
	}
	if err := c.client.WriteFile(ctx, path, []byte(p.Content)); err != nil {
		toolErr = err
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

func writeToolInput(path, content string) map[string]any {
	contentBytes := len(content)
	input := map[string]any{
		"path":               path,
		"content":            content,
		"content_bytes":      contentBytes,
		"content_line_count": lineCount(content),
	}
	if contentBytes <= maxWriteToolContentPreview {
		return input
	}
	preview := strings.ToValidUTF8(content[:maxWriteToolContentPreview], "")
	input["content"] = preview
	input["content_truncated"] = true
	return input
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

func (c *clientCallbacks) emitToolCallStart(id, name string, input map[string]any) {
	if c == nil || c.events == nil {
		return
	}
	c.events.emit(StreamEvent{
		Type:       StreamEventToolCallStart,
		ToolCallID: id,
		ToolName:   name,
		Input:      input,
	})
}

func (c *clientCallbacks) emitToolCallEnd(id, name string, input map[string]any, result any, err error) {
	if c == nil || c.events == nil {
		return
	}
	event := StreamEvent{
		Type:       StreamEventToolCallEnd,
		ToolCallID: id,
		ToolName:   name,
		Input:      input,
		Result:     result,
	}
	if err != nil {
		event.Error = err.Error()
	}
	c.events.emit(event)
}

func toolErrorResult(err error) map[string]any {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return map[string]any{
		"isError": true,
		"content": []map[string]any{{
			"type": "text",
			"text": message,
		}},
	}
}

func (c *clientCallbacks) RequestPermission(_ context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	// ACP permissions stay scoped to the current request. Persistent grants are
	// intentionally not stored because Memoh's approval table models native
	// tool approvals, not ACP client callback grants.
	if err := c.validatePermissionScope(p); err != nil {
		return cancelledPermission(), nil
	}
	for _, opt := range p.Options {
		if opt.Kind == acp.PermissionOptionKindAllowOnce {
			return selectedPermission(opt.OptionId), nil
		}
	}
	return cancelledPermission(), nil
}

func (c *clientCallbacks) SessionUpdate(_ context.Context, p acp.SessionNotification) error {
	c.mu.RLock()
	collector := c.collector
	sink := c.sink
	c.mu.RUnlock()
	var events []StreamEvent
	if c.toolMapper != nil {
		events = c.toolMapper.eventsFromNotification(p)
	}
	if collector != nil {
		collector.apply(p, events)
	}
	if sink != nil {
		for _, event := range events {
			sink.EmitACPEvent(event)
		}
	}
	return nil
}

func (c *clientCallbacks) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return c.terminals.CreateTerminal(ctx, p)
}

func (c *clientCallbacks) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return c.terminals.KillTerminal(ctx, p)
}

func (c *clientCallbacks) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return c.terminals.TerminalOutput(ctx, p)
}

func (c *clientCallbacks) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return c.terminals.ReleaseTerminal(ctx, p)
}

func (c *clientCallbacks) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return c.terminals.WaitForTerminalExit(ctx, p)
}

func (c *clientCallbacks) resolvePath(path string) (string, error) {
	if c.virtualRoot {
		return ResolvePathUnderVirtualRoot(c.root, path)
	}
	return ResolvePathUnderRoot(c.root, path)
}

func (c *clientCallbacks) validatePermissionScope(p acp.RequestPermissionRequest) error {
	for _, loc := range p.ToolCall.Locations {
		if strings.TrimSpace(loc.Path) == "" {
			continue
		}
		if _, err := c.resolvePath(loc.Path); err != nil {
			return err
		}
	}
	if raw, ok := p.ToolCall.RawInput.(map[string]any); ok {
		for _, key := range []string{"cwd", "work_dir", "path", "old_path", "new_path"} {
			value, ok := raw[key].(string)
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			if _, err := c.resolvePath(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func selectedPermission(id acp.PermissionOptionId) acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id},
		},
	}
}

func cancelledPermission() acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Cancelled: &acp.RequestPermissionOutcomeCancelled{},
		},
	}
}

func boundedPositiveInt32(v int) int32 {
	const maxInt32 = int(^uint32(0) >> 1)
	if v <= 0 {
		return 0
	}
	if v > maxInt32 {
		return int32(maxInt32) //nolint:gosec // maxInt32 is exactly the largest int32 value.
	}
	return int32(v) //nolint:gosec // v is bounded to the int32 range above.
}
