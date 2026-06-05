package acpclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type ToolSessionContext = mcp.ToolSessionContext

type StartRequest struct {
	AgentID         string
	BotID           string
	ProjectPath     string
	Command         string
	Args            []string
	LocalCommand    string
	LocalArgs       []string
	Env             []string
	SetupMode       SetupMode
	Timeout         time.Duration
	ToolSession     ToolSessionContext
	ToolHTTPURL     string
	ToolHTTPHandler http.Handler
}

type PromptResult struct {
	StopReason string        `json:"stop_reason,omitempty"`
	Text       string        `json:"text,omitempty"`
	Events     []StreamEvent `json:"events,omitempty"`
}

// PromptResource is an embedded text resource sent alongside an ACP prompt.
type PromptResource struct {
	URI      string
	MimeType string
	Text     string
}

type Session struct {
	logger          *slog.Logger
	proc            *bridgeProcess
	callbacks       *clientCallbacks
	conn            *clientConnection
	sessionID       acp.SessionId
	projectPath     string
	modelState      ModelState
	embeddedContext bool
	defaultSink     EventSink
	cancel          context.CancelFunc
	reverseHTTPStop func()

	promptMu     sync.Mutex
	mu           sync.Mutex
	promptCancel context.CancelFunc
	promptDone   <-chan struct{}
	promptToken  *struct{}
	closed       bool
}

func (r *Runner) StartSession(ctx context.Context, req StartRequest, sink EventSink) (*Session, error) {
	if r == nil || r.workspace == nil {
		return nil, errors.New("ACP workspace provider is not configured")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return nil, errors.New("bot_id is required")
	}

	info, err := r.workspace.WorkspaceInfo(ctx, req.BotID)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	root, projectPath, backend, err := resolveWorkspacePaths(info, req.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("invalid project_path: %w", err)
	}

	client, err := r.workspace.MCPClient(ctx, req.BotID)
	if err != nil {
		return nil, fmt.Errorf("connect workspace bridge: %w", err)
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = r.timeout
	}
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}

	lifecycleCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	startupDone := make(chan struct{})
	var startupDoneOnce sync.Once
	finishStartup := func() {
		startupDoneOnce.Do(func() {
			close(startupDone)
		})
	}
	defer finishStartup()
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-startupDone:
		}
	}()
	command := strings.TrimSpace(req.Command)
	args := append([]string(nil), req.Args...)
	if backend == WorkspaceBackendLocal && strings.TrimSpace(req.LocalCommand) != "" {
		command = strings.TrimSpace(req.LocalCommand)
		args = append([]string(nil), req.LocalArgs...)
	}
	if command == "" {
		command = strings.TrimSpace(r.command)
		if len(args) == 0 {
			args = append(args, r.args...)
		}
	}

	toolHTTPURL := strings.TrimSpace(req.ToolHTTPURL)
	toolHTTPHandler := req.ToolHTTPHandler
	var toolHTTPStop func()
	if backend == WorkspaceBackendContainer && toolHTTPHandler != nil &&
		toolHTTPURL != "" &&
		toolHTTPURL == strings.TrimSpace(info.ACPToolsHTTPURL) {
		guardedURL, guardedPath, guardedHandler, err := guardToolHTTPHandler(toolHTTPURL, toolHTTPHandler)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("prepare Memoh tools bridge: %w", err)
		}
		toolHTTPURL = guardedURL
		var stop func()
		client, stop, err = r.startMemohToolsBridge(lifecycleCtx, req.BotID, client, guardedPath, guardedHandler)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("Memoh tools bridge unavailable; starting ACP session without Memoh tools",
					slog.String("agent_id", req.AgentID),
					slog.String("bot_id", req.BotID),
					slog.Any("error", err),
				)
			}
			toolHTTPURL = ""
		} else {
			toolHTTPStop = stop
		}
	} else if backend == WorkspaceBackendLocal && toolHTTPHandler != nil && toolHTTPURL != "" {
		proxyURL, stop, err := startLocalToolHTTPProxy(lifecycleCtx, toolHTTPHandler)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start Memoh tools proxy: %w", err)
		}
		toolHTTPURL = proxyURL
		toolHTTPStop = stop
	}

	proc, err := startBridgeProcess(lifecycleCtx, client, command, args, projectPath, timeout, processOptions{
		Backend:       backend,
		AgentID:       req.AgentID,
		SetupMode:     req.SetupMode,
		Env:           req.Env,
		WorkspaceRoot: root,
		NoTimeout:     true,
	})
	if err != nil {
		if toolHTTPStop != nil {
			toolHTTPStop()
		}
		cancel()
		return nil, fmt.Errorf("start %s: %w", buildShellCommand(command, args), err)
	}

	toolSession := req.ToolSession
	if strings.TrimSpace(toolSession.BotID) == "" {
		toolSession.BotID = req.BotID
	}
	if strings.TrimSpace(toolSession.ChatID) == "" {
		toolSession.ChatID = toolSession.BotID
	}
	callbacks := newClientCallbacks(lifecycleCtx, client, root, projectPath, timeout, sink, proc.env, backend == WorkspaceBackendContainer)
	conn := newClientConnection(callbacks, proc, proc)

	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo:      &acp.Implementation{Name: "memoh", Version: "dev"},
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	})
	if err != nil {
		callbacks.close()
		_ = proc.Close()
		if toolHTTPStop != nil {
			toolHTTPStop()
		}
		cancel()
		return nil, fmt.Errorf("initialize ACP agent: %w", err)
	}

	mcpServers := []acp.McpServer{}
	if initResp.AgentCapabilities.McpCapabilities.Http {
		if server := memohToolsHTTPMCPServer(toolHTTPURL, toolSession); server.Http != nil {
			mcpServers = append(mcpServers, server)
		}
	}
	if r.logger != nil {
		caps := initResp.AgentCapabilities.McpCapabilities
		r.logger.Info("ACP agent initialized",
			slog.String("agent_id", req.AgentID),
			slog.String("bot_id", req.BotID),
			slog.Bool("mcp_acp", caps.Acp),
			slog.Bool("mcp_http", caps.Http),
			slog.Bool("mcp_sse", caps.Sse),
			slog.Bool("memoh_tools_http_configured", toolHTTPURL != ""),
			slog.String("memoh_tools_http_url", redactedToolHTTPURL(toolHTTPURL)),
			slog.Int("mcp_servers", len(mcpServers)),
		)
		if toolHTTPURL != "" && len(mcpServers) == 0 {
			r.logger.Warn("Memoh tools were not exposed to ACP agent because no supported MCP transport was available",
				slog.String("agent_id", req.AgentID),
				slog.String("bot_id", req.BotID),
				slog.Bool("agent_supports_acp_mcp", caps.Acp),
				slog.Bool("agent_supports_http_mcp", caps.Http),
				slog.Bool("agent_supports_sse_mcp", caps.Sse),
				slog.Bool("http_mcp_url_configured", toolHTTPURL != ""),
			)
		}
	}
	sess, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        projectPath,
		McpServers: mcpServers,
	})
	if err != nil {
		callbacks.close()
		_ = proc.Close()
		if toolHTTPStop != nil {
			toolHTTPStop()
		}
		cancel()
		return nil, fmt.Errorf("create ACP session: %w", err)
	}

	finishStartup()
	return &Session{
		logger:          r.logger,
		proc:            proc,
		callbacks:       callbacks,
		conn:            conn,
		sessionID:       sess.SessionId,
		projectPath:     projectPath,
		modelState:      modelStateFromACP(sess.Models),
		embeddedContext: initResp.AgentCapabilities.PromptCapabilities.EmbeddedContext,
		defaultSink:     sink,
		cancel:          cancel,
		reverseHTTPStop: toolHTTPStop,
	}, nil
}

func (r *Runner) startMemohToolsBridge(ctx context.Context, botID string, client *bridge.Client, route string, handler http.Handler) (*bridge.Client, func(), error) {
	if client == nil {
		return nil, nil, errors.New("workspace bridge client is required")
	}
	current := client
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		stop, err := current.ServeReverseHTTPRoute(ctx, route, handler)
		if err == nil {
			return current, stop, nil
		}
		lastErr = err
		if ctx.Err() != nil || !isClosingBridgeClientError(err) || r == nil || r.workspace == nil || strings.TrimSpace(botID) == "" {
			return current, nil, err
		}
		_ = current.Close()
		if err := sleepContext(ctx, time.Duration(attempt+1)*150*time.Millisecond); err != nil {
			return current, nil, err
		}
		next, err := r.workspace.MCPClient(ctx, botID)
		if err != nil {
			return current, nil, fmt.Errorf("%w; reconnect workspace bridge: %w", lastErr, err)
		}
		current = next
	}
	return current, nil, lastErr
}

func isClosingBridgeClientError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "client connection is closing") ||
		strings.Contains(lower, "transport is closing") ||
		strings.Contains(lower, "use of closed network connection")
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func guardToolHTTPHandler(rawURL string, handler http.Handler) (string, string, http.Handler, error) {
	if handler == nil {
		return "", "", nil, errors.New("tool HTTP handler is required")
	}
	guardedURL, guardPath, err := guardedToolHTTPURL(rawURL)
	if err != nil {
		return "", "", nil, err
	}
	return guardedURL, guardPath, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req == nil || req.URL == nil || req.URL.Path != guardPath {
			http.NotFound(w, req)
			return
		}
		handler.ServeHTTP(w, req)
	}), nil
}

func guardedToolHTTPURL(rawURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid Memoh tools URL %q", rawURL)
	}
	basePath := strings.TrimRight(u.Path, "/")
	if basePath == "" {
		basePath = "/mcp"
	}
	u.Path = basePath + "/" + uuid.NewString()
	return u.String(), u.Path, nil
}

func redactedToolHTTPURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	trimmedPath := strings.Trim(u.Path, "/")
	if trimmedPath != "" {
		parts := strings.Split(trimmedPath, "/")
		redacted := false
		for i, part := range parts {
			if i > 0 && strings.EqualFold(parts[i-1], "bots") {
				parts[i] = "redacted"
				redacted = true
				continue
			}
			if _, err := uuid.Parse(part); err == nil {
				parts[i] = "redacted"
				redacted = true
			}
		}
		if !redacted {
			parts[len(parts)-1] = "redacted"
		}
		u.Path = "/" + strings.Join(parts, "/")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func startLocalToolHTTPProxy(ctx context.Context, handler http.Handler) (string, func(), error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	rawURL := "http://" + listener.Addr().String() + "/mcp"
	guardedURL, _, guardedHandler, err := guardToolHTTPHandler(rawURL, handler)
	if err != nil {
		_ = listener.Close()
		return "", nil, err
	}
	server := &http.Server{
		Handler:           guardedHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
			<-done
		})
	}
	go func() {
		<-ctx.Done()
		stop()
	}()
	return guardedURL, stop, nil
}

func (s *Session) ID() string {
	if s == nil {
		return ""
	}
	return string(s.sessionID)
}

func (s *Session) ProjectPath() string {
	if s == nil {
		return ""
	}
	return s.projectPath
}

func (s *Session) Prompt(ctx context.Context, prompt string, sinks ...EventSink) (PromptResult, error) {
	return s.PromptWithResources(ctx, prompt, nil, sinks...)
}

// PromptWithResources sends a user prompt plus optional embedded resources.
func (s *Session) PromptWithResources(ctx context.Context, prompt string, resources []PromptResource, sinks ...EventSink) (PromptResult, error) {
	if s == nil || s.conn == nil {
		return PromptResult{}, ErrSessionNotInitialized
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return PromptResult{}, ErrPromptRequired
	}

	s.promptMu.Lock()
	defer s.promptMu.Unlock()

	promptCtx, cancelPrompt := context.WithCancel(ctx)
	defer cancelPrompt()
	promptToken := &struct{}{}
	promptDone := make(chan struct{})

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(promptDone)
		return PromptResult{}, ErrSessionClosed
	}
	s.promptCancel = cancelPrompt
	s.promptDone = promptDone
	s.promptToken = promptToken
	conn := s.conn
	sessionID := s.sessionID
	callbacks := s.callbacks
	proc := s.proc
	defaultSink := s.defaultSink
	s.mu.Unlock()
	defer func() {
		close(promptDone)
		s.mu.Lock()
		if s.promptToken == promptToken {
			s.promptCancel = nil
			s.promptDone = nil
			s.promptToken = nil
		}
		s.mu.Unlock()
	}()
	if conn == nil {
		return PromptResult{}, ErrSessionNotInitialized
	}

	promptBlocks := s.promptBlocks(prompt, resources)
	collector := newEventCollector()
	sink := defaultSink
	if len(sinks) > 0 {
		sink = sinks[0]
	}
	if callbacks != nil {
		callbacks.setPromptState(collector, sink)
	}
	defer func() {
		if callbacks != nil {
			callbacks.setPromptState(nil, nil)
		}
	}()

	resp, err := conn.Prompt(promptCtx, acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    promptBlocks,
	})
	collected := collector.result()
	result := PromptResult{
		StopReason: string(resp.StopReason),
		Text:       collected.Text,
		Events:     collected.Events,
	}
	if err != nil {
		if proc != nil {
			return result, proc.errorWithStderr(fmt.Errorf("send ACP prompt: %w", err))
		}
		return result, fmt.Errorf("send ACP prompt: %w", err)
	}
	return result, nil
}

func (s *Session) promptBlocks(prompt string, resources []PromptResource) []acp.ContentBlock {
	cleaned := cleanPromptResources(resources)
	if len(cleaned) == 0 {
		return []acp.ContentBlock{acp.TextBlock(prompt)}
	}
	if s != nil && s.embeddedContext {
		blocks := []acp.ContentBlock{acp.TextBlock(prompt)}
		for _, resource := range cleaned {
			mimeType := resource.MimeType
			blocks = append(blocks, acp.ResourceBlock(acp.EmbeddedResourceResource{
				TextResourceContents: &acp.TextResourceContents{
					Uri:      resource.URI,
					MimeType: &mimeType,
					Text:     resource.Text,
				},
			}))
		}
		return blocks
	}

	var sb strings.Builder
	for _, resource := range cleaned {
		sb.WriteString("<context ref=\"")
		sb.WriteString(resource.URI)
		sb.WriteString("\">\n")
		sb.WriteString(resource.Text)
		sb.WriteString("\n</context>\n\n")
	}
	sb.WriteString(prompt)
	return []acp.ContentBlock{acp.TextBlock(strings.TrimSpace(sb.String()))}
}

func cleanPromptResources(resources []PromptResource) []PromptResource {
	out := make([]PromptResource, 0, len(resources))
	for _, resource := range resources {
		text := strings.TrimSpace(resource.Text)
		if text == "" {
			continue
		}
		uri := strings.TrimSpace(resource.URI)
		if uri == "" {
			continue
		}
		mimeType := strings.TrimSpace(resource.MimeType)
		if mimeType == "" {
			mimeType = "text/plain"
		}
		out = append(out, PromptResource{
			URI:      uri,
			MimeType: mimeType,
			Text:     text,
		})
	}
	return out
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conn := s.conn
	sessionID := s.sessionID
	callbacks := s.callbacks
	proc := s.proc
	cancel := s.cancel
	reverseHTTPStop := s.reverseHTTPStop
	promptCancel := s.promptCancel
	promptDone := s.promptDone
	s.mu.Unlock()

	if promptCancel != nil {
		promptCancel()
	}
	if promptDone != nil {
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-promptDone:
		case <-timer.C:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	if conn != nil && sessionID != "" {
		ctx, cancelClose := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = conn.CloseSession(ctx, acp.CloseSessionRequest{SessionId: sessionID})
		cancelClose()
	}
	if callbacks != nil {
		callbacks.close()
	}
	if reverseHTTPStop != nil {
		reverseHTTPStop()
	}
	if cancel != nil {
		cancel()
	}
	if proc != nil {
		return proc.Close()
	}
	return nil
}
