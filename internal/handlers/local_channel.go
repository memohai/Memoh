package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/apperror"
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/decisionruntime"
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/runtimefence"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/sessionruntime"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/slash"
	"github.com/memohai/memoh/internal/userinput"
)

// localSpeechSynthesizer synthesizes text to speech audio.
type localSpeechSynthesizer interface {
	Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error)
}

// localSpeechModelResolver resolves speech model IDs for bots.
type localSpeechModelResolver interface {
	ResolveSpeechModelID(ctx context.Context, botID string) (string, error)
}

// LocalChannelHandler handles local channel routes (WebUI / API) backed by bot history.
type LocalChannelHandler struct {
	channelType         channel.ChannelType
	channelManager      *channel.Manager
	channelStore        *channel.Store
	chatService         *conversation.Service
	routeHub            *local.RouteHub
	botService          *bots.Service
	accountService      *accounts.Service
	sessionService      *sessionpkg.Service
	sessionRuntime      *sessionruntime.Manager
	resolver            localChannelResolver
	commandHandler      *command.Handler
	skillResolver       runtimeSkillResolver
	mediaService        *media.Service
	speechService       localSpeechSynthesizer
	speechModelResolver localSpeechModelResolver
	wsSkillTurnsMu      sync.Mutex
	wsSkillTurns        *wsRequestedSkillTurnRegistry
	runtimeCommandsMu   sync.Mutex
	runtimeCommandSlots chan struct{}
	logger              *slog.Logger
	jwtSecret           string
	tokenTTL            time.Duration
	runtimeAuthInterval time.Duration
	runtimeSetupTimeout time.Duration
}

type runtimeSkillResolver interface {
	ListSafeSkillCatalog(ctx context.Context, botID string) ([]skillset.SafeCatalogItem, error)
	ResolveTextRequestedSkills(ctx context.Context, botID string, names []string) ([]skillset.ResolvedSkill, error)
}

type localChannelResolver interface {
	ACPSessionExecutionInfo(ctx context.Context, sessionID string) (flow.ACPSessionExecutionInfo, error)
	ActivateRuntimePersistenceFenceWithOptions(ctx context.Context, fence runtimefence.Fence, options runtimefence.ActivationOptions) error
	AllocateRuntimePersistenceFence(ctx context.Context, botID, sessionID string) (runtimefence.Fence, error)
	AdmitPreparedReplacementWS(ctx context.Context, prepared flow.PreparedReplacementWS) (flow.PreparedReplacementWS, func(), error)
	ApplyUserMessageHookAndPersistUserTurn(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, messagepkg.Message, error)
	LinkOutboundAssets(ctx context.Context, botID, sessionID string, assets []messagepkg.AssetRef) error
	PrepareUserMessageWS(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error)
	PrepareEditLatestMessageWS(ctx context.Context, input flow.EditLatestMessageInput) (flow.PreparedReplacementWS, error)
	PrepareRetryLatestMessageWS(ctx context.Context, input flow.RetryLatestMessageInput) (flow.PreparedReplacementWS, error)
	PrepareToolApprovalResponse(ctx context.Context, input flow.ToolApprovalResponseInput) (runtimefence.PreservedDecision, error)
	PrepareUserInputResponseTarget(ctx context.Context, input flow.UserInputResponseInput) (runtimefence.PreservedDecision, error)
	RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, eventCh chan<- flow.WSStreamEvent) error
	RespondUserInput(ctx context.Context, input flow.UserInputResponseInput, eventCh chan<- flow.WSStreamEvent) error
	DeferSessionCompaction(botID, sessionID, streamID string) func()
	SessionTurnActive(botID, sessionID string) bool
	StreamChatWS(ctx context.Context, req conversation.ChatRequest, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}) error
	StreamPreparedReplacementWS(ctx context.Context, prepared flow.PreparedReplacementWS, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}) error
	ValidatePreparedReplacementWS(ctx context.Context, prepared flow.PreparedReplacementWS) error
	ValidateWorkspaceTarget(ctx context.Context, botID, targetID string) error
}

// NewLocalChannelHandler creates a local channel handler.
func NewLocalChannelHandler(channelType channel.ChannelType, channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, routeHub *local.RouteHub, botService *bots.Service, accountService *accounts.Service, sessionService *sessionpkg.Service) *LocalChannelHandler {
	return &LocalChannelHandler{
		channelType:         channelType,
		channelManager:      channelManager,
		channelStore:        channelStore,
		chatService:         chatService,
		routeHub:            routeHub,
		botService:          botService,
		accountService:      accountService,
		sessionService:      sessionService,
		wsSkillTurns:        newWSRequestedSkillTurnRegistry(),
		runtimeCommandSlots: make(chan struct{}, maxConcurrentWSRuntimeCommands),
		logger:              slog.Default().With(slog.String("handler", "local_channel")),
	}
}

const maxConcurrentWSRuntimeCommands = 64

func (h *LocalChannelHandler) tryAcquireRuntimeCommand() bool {
	if h == nil {
		return false
	}
	h.runtimeCommandsMu.Lock()
	if h.runtimeCommandSlots == nil {
		h.runtimeCommandSlots = make(chan struct{}, maxConcurrentWSRuntimeCommands)
	}
	slots := h.runtimeCommandSlots
	h.runtimeCommandsMu.Unlock()
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *LocalChannelHandler) releaseRuntimeCommand() {
	if h == nil {
		return
	}
	h.runtimeCommandsMu.Lock()
	slots := h.runtimeCommandSlots
	h.runtimeCommandsMu.Unlock()
	if slots == nil {
		return
	}
	select {
	case <-slots:
	default:
	}
}

// SetResolver sets the flow resolver for WebSocket streaming.
func (h *LocalChannelHandler) SetResolver(resolver localChannelResolver) {
	h.resolver = resolver
	h.bindSessionRuntimeCommandHandler()
}

func (h *LocalChannelHandler) SetCommandHandler(handler *command.Handler) {
	h.commandHandler = handler
}

func (h *LocalChannelHandler) SetRuntimeSkillResolver(resolver runtimeSkillResolver) {
	h.skillResolver = resolver
}

func (h *LocalChannelHandler) SetSessionRuntime(manager *sessionruntime.Manager) {
	h.sessionRuntime = manager
	h.bindSessionRuntimeCommandHandler()
}

func (h *LocalChannelHandler) bindSessionRuntimeCommandHandler() {
	if h == nil {
		return
	}
	decisionruntime.BindCommandHandlers(h.sessionRuntime, h.resolver)
}

// SetAuthTokenConfig configures runtime token minting for ACP-backed local WS streams.
func (h *LocalChannelHandler) SetAuthTokenConfig(jwtSecret string, ttl time.Duration) {
	h.jwtSecret = strings.TrimSpace(jwtSecret)
	h.tokenTTL = ttl
}

// SetMediaService sets the media service for WebSocket attachment ingestion.
func (h *LocalChannelHandler) SetMediaService(svc *media.Service) {
	h.mediaService = svc
}

// SetSpeechService configures speech synthesis for handling speech_delta events.
func (h *LocalChannelHandler) SetSpeechService(synth localSpeechSynthesizer, resolver localSpeechModelResolver) {
	h.speechService = synth
	h.speechModelResolver = resolver
}

// Register registers the local channel routes.
func (h *LocalChannelHandler) Register(e *echo.Echo) {
	prefix := fmt.Sprintf("/bots/:bot_id/%s", h.channelType.String())
	group := e.Group(prefix)
	group.GET("/stream", h.StreamMessages)
	group.GET("/ws", h.HandleWebSocket)
	e.GET("/bots/:bot_id/sessions/:session_id/runtime", h.GetSessionRuntime)
	e.POST("/bots/:bot_id/quick-actions/execute", h.ExecuteQuickAction)
}

type QuickActionExecuteRequest struct {
	ActionID      string         `json:"action_id"`
	Params        map[string]any `json:"params,omitempty"`
	InvocationID  string         `json:"invocation_id,omitempty"`
	ComposerScope string         `json:"composer_scope,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
}

type CommandEventResponse struct {
	Type          string               `json:"type"`
	InvocationID  string               `json:"invocation_id,omitempty"`
	ComposerScope string               `json:"composer_scope,omitempty"`
	SessionID     string               `json:"session_id,omitempty"`
	ActionID      string               `json:"action_id,omitempty"`
	Terminal      bool                 `json:"terminal"`
	Result        *CommandActionResult `json:"result,omitempty"`
	Error         *CommandActionError  `json:"error,omitempty"`
}

type CommandActionResult struct {
	Kind  string                  `json:"kind"`
	Title string                  `json:"title,omitempty"`
	Text  string                  `json:"text,omitempty"`
	Items []CommandActionListItem `json:"items,omitempty"`
}

type CommandActionListItem struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

type CommandActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GetSessionRuntime godoc
// @Summary Get session runtime state
// @Description Returns the current live runtime snapshot for a chat session.
// @Tags sessions
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} sessionruntime.Snapshot
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} apperror.Problem
// @Router /bots/{bot_id}/sessions/{session_id}/runtime [get].
func (h *LocalChannelHandler) GetSessionRuntime(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if botID == "" || sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and session_id are required")
	}
	if err := h.authorizeWSRuntimeSession(c, channelIdentityID, botID, sessionID); err != nil {
		return err
	}
	if h.sessionRuntime == nil {
		return c.JSON(http.StatusOK, sessionruntime.EmptySnapshot(botID, sessionID))
	}
	snapshot, err := h.sessionRuntime.Snapshot(c.Request().Context(), botID, sessionID)
	if err != nil {
		return apperror.Wrap(apperror.CodeSessionRuntimeUnavailable, err, nil)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// ExecuteQuickAction godoc
// @Summary Execute a Web quick action
// @Description Runs a typed Web quick action such as help or skill.list and returns a command_result or command_error envelope.
// @Tags quick-actions
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body QuickActionExecuteRequest true "Quick action payload"
// @Success 200 {object} CommandEventResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/quick-actions/execute [post].
func (h *LocalChannelHandler) ExecuteQuickAction(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	var req QuickActionExecuteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	actionID := strings.TrimSpace(req.ActionID)
	sessionID := strings.TrimSpace(req.SessionID)
	skillActivationAllowed := true
	if sessionID != "" {
		if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
			return err
		}
		supported, supportErr := h.wsSessionSupportsRequestedSkills(c.Request().Context(), sessionID)
		if supportErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, supportErr.Error())
		}
		skillActivationAllowed = supported
	}
	if !quickActionSkillActivationAllowedHint(req.Params) {
		skillActivationAllowed = false
	}
	result, slashErr := h.executeWebQuickAction(c.Request().Context(), botID, actionID, skillActivationAllowed)
	event := commandEvent(req.InvocationID, req.ComposerScope, sessionID, actionID)
	if slashErr != nil {
		event.Type = "command_error"
		event.Error = &CommandActionError{Code: slashErr.Code, Message: slashUserMessage(slashErr.Code)}
		return c.JSON(http.StatusOK, event)
	}
	event.Type = "command_result"
	event.Result = result
	return c.JSON(http.StatusOK, event)
}

func quickActionSkillActivationAllowedHint(params map[string]any) bool {
	if params == nil {
		return true
	}
	allowed, ok := params["skill_activation_allowed"].(bool)
	return !ok || allowed
}

func commandEvent(invocationID, composerScope, sessionID, actionID string) CommandEventResponse {
	return CommandEventResponse{
		InvocationID:  strings.TrimSpace(invocationID),
		ComposerScope: strings.TrimSpace(composerScope),
		SessionID:     strings.TrimSpace(sessionID),
		ActionID:      strings.TrimSpace(actionID),
		Terminal:      true,
	}
}

func (h *LocalChannelHandler) executeWebQuickAction(ctx context.Context, botID, actionID string, skillActivationAllowed bool) (*CommandActionResult, *slash.Error) {
	switch strings.TrimSpace(actionID) {
	case "help":
		items := []CommandActionListItem{
			{ID: "help", Title: "/help", Description: "Show available quick actions", Kind: "quick_action"},
			{ID: "new", Title: "/new", Description: "Start a new session", Kind: "quick_action"},
			{ID: "compact", Title: "/compact", Description: "Compact the current session history", Kind: "quick_action"},
		}
		labels := []string{"/help", "/new", "/compact"}
		text := "Available Web quick actions: %s."
		// skillActivationAllowed already reflects a plain (non-ACP) chat
		// session, which is also the only context where the model picker
		// applies, so it doubles as the /model gate.
		if skillActivationAllowed {
			items = append(items,
				CommandActionListItem{ID: "skill.list", Title: "/skill list", Description: "Show runtime-usable skills", Kind: "quick_action"},
				CommandActionListItem{ID: "model", Title: "/model", Description: "Switch the chat model", Kind: "quick_action"},
			)
			labels = append(labels, "/skill list", "/model")
			text = "Available Web quick actions: %s. To activate a skill, send /<skill-name> or /<skill-name> <prompt>."
		}
		return &CommandActionResult{
			Kind:  "list",
			Title: "Quick actions",
			Items: items,
			Text:  fmt.Sprintf(text, strings.Join(labels, ", ")),
		}, nil
	case "skill.list":
		if !skillActivationAllowed {
			err := slash.Error{Code: slash.CodeUnsupportedSkillSlashContext}
			return nil, &err
		}
		if h.skillResolver == nil {
			err := slash.Error{Code: slash.CodeRequestedSkillNotRuntimeUsable, Msg: "skill resolver not configured"}
			return nil, &err
		}
		catalog, err := h.skillResolver.ListSafeSkillCatalog(ctx, botID)
		if err != nil {
			code := slashErrorCode(err)
			if code == "" {
				code = slash.CodeRequestedSkillNotRuntimeUsable
			}
			slashErr := slash.Error{Code: code, Msg: err.Error()}
			return nil, &slashErr
		}
		items := make([]CommandActionListItem, 0, len(catalog))
		for _, item := range catalog {
			items = append(items, CommandActionListItem{
				ID:          item.Name,
				Title:       item.Name,
				Description: item.Description,
				Kind:        "skill",
			})
		}
		return &CommandActionResult{Kind: "list", Title: "Skills", Items: items}, nil
	default:
		err := slash.Error{Code: slash.CodeUnsupportedWebCommand}
		return nil, &err
	}
}

func slashErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var slashErr slash.Error
	if errors.As(err, &slashErr) {
		return slashErr.Code
	}
	return ""
}

func slashUserMessage(code string) string {
	switch code {
	case slash.CodeUnknownSlash:
		return "Unknown slash command."
	case slash.CodeUnsupportedWebCommand:
		return "This slash command is not available in Web."
	case slash.CodeInvalidSkillSlashSyntax:
		return "Invalid skill slash syntax. Use /<skill-name> [prompt] or pick a skill from the composer."
	case slash.CodeRequestedSkillNotFound:
		return "Requested skill was not found."
	case slash.CodeRequestedSkillAmbiguous:
		return "Requested skill is ambiguous."
	case slash.CodeRequestedSkillDisabled:
		return "Requested skill is disabled."
	case slash.CodeRequestedSkillNotRuntimeUsable:
		return "Requested skill is not available for chat."
	case slash.CodeTooManyRequestedSkills:
		return "Too many skills selected."
	case slash.CodeRequestedSkillContextTooLarge:
		return "Selected skill context is too large."
	case slash.CodeSlashAttachmentsUnsupported:
		return "Slash commands cannot be sent with attachments."
	case slash.CodeUnsupportedSkillSlashContext:
		return "Requested skills are not supported in this context."
	case slash.CodeUnsupportedLegacyEndpoint:
		return "Skill activation requires WebSocket. Reconnect and try again."
	case slash.CodePermissionDenied:
		return "You do not have permission to run this command."
	case slash.CodeReservedSkillMetadata:
		return "Reserved skill metadata cannot be supplied by clients."
	case slash.CodeInvalidQuickActionScope:
		return "This quick action cannot be scoped to a session."
	default:
		return "Slash command failed."
	}
}

func (h *LocalChannelHandler) resolveWebRequestedSkillContexts(ctx context.Context, botID string, requested []webRequestedSkill) ([]conversation.RequestedSkillContext, error) {
	names := make([]string, 0, len(requested))
	for _, item := range requested {
		names = append(names, strings.TrimSpace(item.Name))
	}
	return h.resolveWebTextRequestedSkillContexts(ctx, botID, names)
}

func (h *LocalChannelHandler) resolveWebTextRequestedSkillContexts(ctx context.Context, botID string, names []string) ([]conversation.RequestedSkillContext, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if h.skillResolver == nil {
		return nil, slash.NewError(slash.CodeRequestedSkillNotRuntimeUsable)
	}
	resolved, err := h.skillResolver.ResolveTextRequestedSkills(ctx, botID, names)
	if err != nil {
		return nil, err
	}
	return skillset.RequestedSkillContexts(resolved), nil
}

func (h *LocalChannelHandler) classifyWebSlash(text string, hasAttachments bool, surface slash.Surface) slash.Decision {
	return slash.Classify(slash.ClassifyInput{
		Text:           text,
		HasAttachments: hasAttachments,
		Surface:        surface,
		IsGroup:        false,
		Directed:       true,
		SupportsMode:   false,
		KnownCommand: func(resource string) bool {
			if resource == "help" || resource == "skill" {
				return true
			}
			return h.commandHandler != nil && h.commandHandler.HasCommandResource(resource)
		},
		WebActionSupported: func(resource, action string) bool {
			return webActionID(resource, action) != ""
		},
	})
}

func webActionID(resource, action string) string {
	resource = strings.TrimSpace(strings.ToLower(resource))
	action = strings.TrimSpace(strings.ToLower(action))
	switch {
	case resource == "help" && action == "":
		return "help"
	case resource == "skill" && (action == "" || action == "list"):
		return "skill.list"
	default:
		return ""
	}
}

func sendWSCommandError(ctx context.Context, writer *wsWriter, msg wsClientMessage, code string) {
	event := commandEvent(msg.InvocationID, msg.ComposerScope, msg.SessionID, "")
	event.Type = "command_error"
	event.Error = &CommandActionError{Code: code, Message: slashUserMessage(code)}
	writer.SendJSONBounded(ctx, event)
}

func sendWSCommandResult(ctx context.Context, writer *wsWriter, msg wsClientMessage, actionID string, result *CommandActionResult) {
	event := commandEvent(msg.InvocationID, msg.ComposerScope, msg.SessionID, actionID)
	event.Type = "command_result"
	event.Result = result
	writer.SendJSONBounded(ctx, event)
}

func newSessionRuntimeAppError(err error, fallback apperror.Code) error {
	if err == nil {
		return nil
	}
	if _, ok := apperror.As(err); ok {
		return err
	}
	code := fallback
	switch {
	case errors.Is(err, sessionruntime.ErrCommandTargetNotActive),
		errors.Is(err, sessionruntime.ErrCommandTargetMismatch),
		errors.Is(err, sessionruntime.ErrRunOwnershipLost):
		code = apperror.CodeSessionRuntimeTargetNotActive
	case errors.Is(err, sessionruntime.ErrCommandBusy):
		code = apperror.CodeSessionRuntimeCommandBusy
	case errors.Is(err, sessionruntime.ErrManagerClosed),
		errors.Is(err, sessionruntime.ErrCommandOwnerUnavailable),
		errors.Is(err, sessionruntime.ErrBackendConflict),
		errors.Is(err, context.DeadlineExceeded):
		code = apperror.CodeSessionRuntimeUnavailable
	case errors.Is(err, context.Canceled):
		code = apperror.CodeSessionRuntimeInterrupted
	}
	return apperror.Wrap(code, err, nil)
}

func newWSSidebandError(err error, fallback apperror.Code) *CommandActionError {
	if feedback := acpFeedbackError(err); feedback != nil {
		return &CommandActionError{Code: feedback.Code, Message: feedback.Message}
	}
	publicErr := newSessionRuntimeAppError(err, fallback)
	public, ok := apperror.PublicFrom(publicErr, "")
	if !ok {
		return &CommandActionError{Code: string(apperror.CodeSessionRuntimeCommandFailed), Message: "The runtime command could not be completed."}
	}
	return &CommandActionError{Code: string(public.Code), Message: public.Detail}
}

func (h *LocalChannelHandler) sendWSSidebandResult(ctx context.Context, writer *wsWriter, msg wsClientMessage, actionID string, err error) {
	invocationID := strings.TrimSpace(msg.InvocationID)
	if invocationID == "" {
		invocationID = strings.TrimSpace(msg.StreamID)
	}
	event := commandEvent(invocationID, msg.ComposerScope, msg.SessionID, actionID)
	if err != nil {
		if h != nil && h.logger != nil {
			h.logger.Warn("ws runtime side-band command failed", slog.Any("error", err), slog.String("action_id", actionID), slog.String("session_id", msg.SessionID))
		}
		fallback := apperror.CodeSessionRuntimeCommandFailed
		if actionID == "runtime_subscribe" {
			fallback = apperror.CodeSessionRuntimeUnavailable
		}
		event.Type = "command_error"
		event.Error = newWSSidebandError(err, fallback)
	} else {
		event.Type = "command_result"
		event.Result = &CommandActionResult{Kind: "ack"}
	}
	writer.SendJSONBounded(ctx, event)
}

func (h *LocalChannelHandler) sendWSRuntimeError(ctx context.Context, writer *wsWriter, streamID, sessionID string, err error, fallback apperror.Code) {
	if h != nil && h.logger != nil {
		h.logger.Warn("ws session runtime request failed", slog.Any("error", err), slog.String("stream_id", streamID), slog.String("session_id", sessionID))
	}
	sendWSErrorFromError(ctx, writer, streamID, sessionID, newSessionRuntimeAppError(err, fallback))
}

// StreamMessages godoc
// @Summary Subscribe to local channel events via SSE
// @Description Open a persistent SSE connection to receive real-time stream events for the given bot.
// @Tags local-channel
// @Produce text/event-stream
// @Param bot_id path string true "Bot ID"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/web/stream [get].
func (h *LocalChannelHandler) StreamMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	if err := h.ensureBotParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}
	if h.routeHub == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "route hub not configured")
	}

	setSSEHeaders(c)
	c.Response().WriteHeader(http.StatusOK)

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	writer := c.Response().Writer

	_, stream, cancel := h.routeHub.Subscribe(botID)
	defer cancel()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case msg, ok := <-stream:
			if !ok {
				return nil
			}
			data, err := formatLocalStreamEvent(msg.Event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", string(data)); err != nil {
				return nil // client disconnected
			}
			flusher.Flush()
		}
	}
}

func formatLocalStreamEvent(event channel.StreamEvent) ([]byte, error) {
	return json.Marshal(event)
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type wsClientMessage struct {
	Type              string                     `json:"type"`
	StreamID          string                     `json:"stream_id,omitempty"`
	Generation        string                     `json:"generation,omitempty"`
	Text              string                     `json:"text,omitempty"`
	SessionID         string                     `json:"session_id,omitempty"`
	InvocationID      string                     `json:"invocation_id,omitempty"`
	ComposerScope     string                     `json:"composer_scope,omitempty"`
	MessageID         string                     `json:"message_id,omitempty"`
	Attachments       []json.RawMessage          `json:"attachments,omitempty"`
	RequestedSkills   []webRequestedSkill        `json:"requested_skills,omitempty"`
	ModelID           string                     `json:"model_id,omitempty"`
	ReasoningEffort   string                     `json:"reasoning_effort,omitempty"`
	WorkspaceTargetID string                     `json:"workspace_target_id,omitempty"`
	ApprovalID        string                     `json:"approval_id,omitempty"`
	UserInputID       string                     `json:"user_input_id,omitempty"`
	ShortID           int                        `json:"short_id,omitempty"`
	ToolCallID        string                     `json:"tool_call_id,omitempty"`
	Decision          string                     `json:"decision,omitempty"`
	Reason            string                     `json:"reason,omitempty"`
	Answers           []userinput.QuestionAnswer `json:"answers,omitempty"`
	Canceled          bool                       `json:"canceled,omitempty"`
}

type webRequestedSkill struct {
	Name string `json:"name"`
}

type wsOutboundEvent struct {
	Type      string `json:"type"`
	StreamID  string `json:"stream_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Data      any    `json:"data,omitempty"`
	Message   string `json:"message,omitempty"`
	Feedback  any    `json:"feedback,omitempty"`
}

type activeWSStream struct {
	streamID  string
	sessionID string
	cancel    context.CancelFunc
	abortCh   chan struct{}
	reusedID  bool
}

type wsRuntimeSubscription struct {
	close func()
	done  chan struct{}
}

func (s *wsRuntimeSubscription) stop() {
	if s == nil {
		return
	}
	s.close()
	<-s.done
}

type wsStreamRegistry struct {
	mu                  sync.Mutex
	protocolCond        *sync.Cond
	byID                map[string]*activeWSStream
	runtimeSessions     map[string]struct{}
	legacyEnqueues      map[string]int
	seenIDs             map[string]struct{}
	runtimeSessionLimit int
	seenStreamIDLimit   int
}

const (
	defaultWSRuntimeSessionLimit = 128
	defaultWSSeenStreamIDLimit   = 1024
)

func (r *wsStreamRegistry) enableRuntimeProtocol(sessionID string) {
	_ = r.enableRuntimeProtocolAndSend(sessionID, nil)
}

func (r *wsStreamRegistry) enableRuntimeProtocolAndSend(sessionID string, send func()) bool {
	if r == nil {
		return false
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return false
	}
	r.mu.Lock()
	if _, enabled := r.runtimeSessions[sid]; !enabled {
		if len(r.runtimeSessions) >= r.runtimeSessionLimit {
			r.mu.Unlock()
			return false
		}
	}
	r.runtimeSessions[sid] = struct{}{}
	for r.legacyEnqueues[sid] > 0 {
		r.protocolCond.Wait()
	}
	r.mu.Unlock()
	if send != nil {
		send()
	}
	return true
}

func (r *wsStreamRegistry) forwardLegacyIfEnabled(sessionID string, forward func()) bool {
	if r == nil || forward == nil {
		return false
	}
	sid := strings.TrimSpace(sessionID)
	r.mu.Lock()
	_, enabled := r.runtimeSessions[sid]
	if !enabled {
		r.legacyEnqueues[sid]++
	}
	r.mu.Unlock()
	if enabled {
		return false
	}
	defer func() {
		r.mu.Lock()
		if r.legacyEnqueues[sid] <= 1 {
			delete(r.legacyEnqueues, sid)
		} else {
			r.legacyEnqueues[sid]--
		}
		r.protocolCond.Broadcast()
		r.mu.Unlock()
	}()
	forward()
	return true
}

func (r *wsStreamRegistry) legacyProtocolEnabled(sessionID string) bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, enabled := r.runtimeSessions[strings.TrimSpace(sessionID)]
	return !enabled
}

type wsRequestedSkillTurnRegistry struct {
	mu     sync.Mutex
	active map[string]int
}

func newWSStreamRegistry() *wsStreamRegistry {
	return newWSStreamRegistryWithLimits(defaultWSRuntimeSessionLimit, defaultWSSeenStreamIDLimit)
}

func newWSStreamRegistryWithLimits(runtimeSessionLimit, seenStreamIDLimit int) *wsStreamRegistry {
	if runtimeSessionLimit <= 0 {
		runtimeSessionLimit = defaultWSRuntimeSessionLimit
	}
	if seenStreamIDLimit <= 0 {
		seenStreamIDLimit = defaultWSSeenStreamIDLimit
	}
	registry := &wsStreamRegistry{
		byID:                make(map[string]*activeWSStream),
		runtimeSessions:     make(map[string]struct{}),
		legacyEnqueues:      make(map[string]int),
		seenIDs:             make(map[string]struct{}),
		runtimeSessionLimit: runtimeSessionLimit,
		seenStreamIDLimit:   seenStreamIDLimit,
	}
	registry.protocolCond = sync.NewCond(&registry.mu)
	return registry
}

func newWSRequestedSkillTurnRegistry() *wsRequestedSkillTurnRegistry {
	return &wsRequestedSkillTurnRegistry{active: make(map[string]int)}
}

func wsRequestedSkillTurnKey(botID, sessionID string) string {
	return strings.TrimSpace(botID) + ":" + strings.TrimSpace(sessionID)
}

func (r *wsRequestedSkillTurnRegistry) reserve(botID, sessionID, streamID string) (func(), bool) {
	key := wsRequestedSkillTurnKey(botID, sessionID)
	if r == nil || strings.TrimSpace(botID) == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(streamID) == "" {
		return func() {}, true
	}

	r.mu.Lock()
	if r.active == nil {
		r.active = make(map[string]int)
	}
	if r.active[key] > 0 {
		r.mu.Unlock()
		return nil, false
	}
	r.active[key] = 1
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			switch refs := r.active[key] - 1; {
			case refs > 0:
				r.active[key] = refs
			default:
				delete(r.active, key)
			}
			r.mu.Unlock()
		})
	}, true
}

func (r *wsRequestedSkillTurnRegistry) enter(botID, sessionID, streamID string) func() {
	key := wsRequestedSkillTurnKey(botID, sessionID)
	if r == nil || strings.TrimSpace(botID) == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(streamID) == "" {
		return func() {}
	}
	r.mu.Lock()
	if r.active == nil {
		r.active = make(map[string]int)
	}
	r.active[key]++
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			switch refs := r.active[key] - 1; {
			case refs > 0:
				r.active[key] = refs
			default:
				delete(r.active, key)
			}
			r.mu.Unlock()
		})
	}
}

func (h *LocalChannelHandler) reserveWSRequestedSkillTurn(botID, sessionID, streamID string) (func(), bool) {
	if h == nil {
		return func() {}, true
	}
	h.wsSkillTurnsMu.Lock()
	if h.wsSkillTurns == nil {
		h.wsSkillTurns = newWSRequestedSkillTurnRegistry()
	}
	registry := h.wsSkillTurns
	h.wsSkillTurnsMu.Unlock()
	return registry.reserve(botID, sessionID, streamID)
}

func (h *LocalChannelHandler) enterWSMessageTurn(botID, sessionID, streamID string) func() {
	if h == nil {
		return func() {}
	}
	h.wsSkillTurnsMu.Lock()
	if h.wsSkillTurns == nil {
		h.wsSkillTurns = newWSRequestedSkillTurnRegistry()
	}
	registry := h.wsSkillTurns
	h.wsSkillTurnsMu.Unlock()
	return registry.enter(botID, sessionID, streamID)
}

func (r *wsStreamRegistry) register(stream *activeWSStream) error {
	streamID := strings.TrimSpace(stream.streamID)
	if streamID == "" {
		return errors.New("stream_id is required")
	}

	sessionID := strings.TrimSpace(stream.sessionID)
	key := sessionID + "\x00" + streamID
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[key]; exists {
		return fmt.Errorf("stream_id %q is already active", streamID)
	}
	if _, stream.reusedID = r.seenIDs[key]; !stream.reusedID {
		if len(r.seenIDs) >= r.seenStreamIDLimit {
			return errors.New("websocket stream id limit reached; reconnect before starting another stream")
		}
		r.seenIDs[key] = struct{}{}
	}
	stream.streamID = streamID
	stream.sessionID = sessionID
	r.byID[key] = stream
	return nil
}

func (r *wsStreamRegistry) streamLocked(streamID, sessionID string) *activeWSStream {
	id := strings.TrimSpace(streamID)
	sid := strings.TrimSpace(sessionID)
	if sid != "" {
		return r.byID[sid+"\x00"+id]
	}
	var matched *activeWSStream
	for _, stream := range r.byID {
		if stream == nil || stream.streamID != id {
			continue
		}
		if matched != nil {
			return nil
		}
		matched = stream
	}
	return matched
}

func (r *wsStreamRegistry) hasSession(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if r == nil || sessionID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, stream := range r.byID {
		if stream != nil && stream.sessionID == sessionID {
			return true
		}
	}
	return false
}

func (r *wsStreamRegistry) sessionForStream(streamID, sessionID string) (string, bool) {
	if r == nil {
		return "", false
	}
	id := strings.TrimSpace(streamID)
	sid := strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	stream := r.streamLocked(id, sid)
	if stream == nil {
		return "", false
	}
	registeredSessionID := strings.TrimSpace(stream.sessionID)
	if sid != "" && sid != registeredSessionID {
		return "", false
	}
	return registeredSessionID, registeredSessionID != ""
}

func (r *wsStreamRegistry) generationlessAbortAllowed(streamID, sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stream := r.streamLocked(streamID, sessionID)
	return stream != nil && !stream.reusedID
}

type sessionTurnActiveChecker interface {
	SessionTurnActive(botID, sessionID string) bool
}

func shouldRejectWSSkillActivationForActiveStream(activeStreams *wsStreamRegistry, activeTurns sessionTurnActiveChecker, botID, sessionID string, hasSkillActivation bool) bool {
	if !hasSkillActivation {
		return false
	}
	if activeStreams != nil && activeStreams.hasSession(sessionID) {
		return true
	}
	return activeTurns != nil && activeTurns.SessionTurnActive(botID, sessionID)
}

func (r *wsStreamRegistry) finish(streamID string, sessionIDs ...string) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID := ""
	if len(sessionIDs) > 0 {
		sessionID = strings.TrimSpace(sessionIDs[0])
	}
	stream := r.streamLocked(streamID, sessionID)
	if stream == nil {
		return
	}
	delete(r.byID, stream.sessionID+"\x00"+stream.streamID)
}

func (r *wsStreamRegistry) abort(streamID, sessionID string) bool {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)

	r.mu.Lock()
	stream := r.streamLocked(streamID, sessionID)
	r.mu.Unlock()
	if stream == nil {
		return false
	}
	if sessionID != "" && stream.sessionID != sessionID {
		return false
	}
	select {
	case stream.abortCh <- struct{}{}:
	default:
	}
	stream.cancel()
	return true
}

// wsWriter serialises all WebSocket writes through a single goroutine to
// avoid concurrent write panics with gorilla/websocket.
type wsWriteConnection interface {
	SetWriteDeadline(t time.Time) error
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type wsWriter struct {
	conn       wsWriteConnection
	ch         chan wsWriteRequest
	closeOnce  sync.Once
	stop       chan struct{}
	done       chan struct{}
	enqueueTTL time.Duration
	writeTTL   time.Duration
}

type wsWriteRequest struct {
	ctx      context.Context
	data     []byte
	accepted chan struct{}
}

const (
	wsWriterEnqueueTimeout = 5 * time.Second
	wsWriterWriteTimeout   = 5 * time.Second
)

const wsRuntimeSubscriptionSetupTimeout = 5 * time.Second

func newWSWriter(conn wsWriteConnection) *wsWriter {
	w := &wsWriter{
		conn:       conn,
		ch:         make(chan wsWriteRequest, 128),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		enqueueTTL: wsWriterEnqueueTimeout,
		writeTTL:   wsWriterWriteTimeout,
	}
	go w.loop()
	return w
}

func (w *wsWriter) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			return
		default:
		}

		select {
		case request := <-w.ch:
			if request.ctx.Err() != nil {
				continue
			}
			select {
			case <-w.stop:
				return
			default:
			}
			close(request.accepted)
			if err := w.conn.SetWriteDeadline(time.Now().Add(w.writeTTL)); err != nil {
				w.closeConnection()
				return
			}
			if err := w.conn.WriteMessage(websocket.TextMessage, request.data); err != nil {
				w.closeConnection()
				return
			}
		case <-w.stop:
			return
		}
	}
}

func (w *wsWriter) closeConnection() {
	w.closeOnce.Do(func() {
		close(w.stop)
	})
	if w.conn != nil {
		_ = w.conn.Close()
	}
}

func (w *wsWriter) SendContext(ctx context.Context, data []byte) bool {
	select {
	case <-w.stop:
		return false
	case <-ctx.Done():
		return false
	default:
	}

	request := wsWriteRequest{ctx: ctx, data: data, accepted: make(chan struct{})}
	select {
	case w.ch <- request:
	case <-w.stop:
		return false
	case <-ctx.Done():
		return false
	}

	select {
	case <-request.accepted:
		return true
	case <-w.stop:
		return false
	case <-ctx.Done():
		return false
	}
}

func (w *wsWriter) SendJSONContext(ctx context.Context, v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return w.SendContext(ctx, data)
}

func (w *wsWriter) SendJSONBounded(ctx context.Context, v any) bool {
	enqueueTTL := w.enqueueTTL
	if enqueueTTL <= 0 {
		enqueueTTL = wsWriterEnqueueTimeout
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, enqueueTTL)
	defer cancel()
	if w.SendJSONContext(enqueueCtx, v) {
		return true
	}
	if ctx.Err() == nil && errors.Is(enqueueCtx.Err(), context.DeadlineExceeded) {
		w.closeConnection()
	}
	return false
}

func (w *wsWriter) Close() {
	w.closeConnection()
	<-w.done
}

// extractRawBearerToken returns the raw JWT token suitable for passing to the
// gateway. The gateway WS handler receives the token directly (not as an HTTP
// header), so we must strip the "Bearer " prefix if present.
func extractRawBearerToken(c echo.Context) string {
	auth := strings.TrimSpace(c.Request().Header.Get("Authorization"))
	if auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return strings.TrimSpace(c.QueryParam("token"))
}

func (h *LocalChannelHandler) issueRuntimeOwnerBearerToken(runtimeOwnerAccountID, fallbackBearerToken string) string {
	runtimeOwnerAccountID = strings.TrimSpace(runtimeOwnerAccountID)
	if h == nil || strings.TrimSpace(h.jwtSecret) == "" || runtimeOwnerAccountID == "" || h.tokenTTL <= 0 {
		return fallbackBearerToken
	}
	signed, _, err := auth.GenerateToken(runtimeOwnerAccountID, h.jwtSecret, h.tokenTTL)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("issue ACP runtime token failed", slog.Any("error", err))
		}
		return fallbackBearerToken
	}
	return "Bearer " + signed
}

func sendWSError(ctx context.Context, writer *wsWriter, streamID, sessionID, message string) {
	writer.SendJSONBounded(ctx, wsOutboundEvent{
		Type:      "error",
		StreamID:  strings.TrimSpace(streamID),
		SessionID: strings.TrimSpace(sessionID),
		Message:   message,
	})
}

func newWSAppErrorEvent(streamID, sessionID string, err error) (wsOutboundEvent, bool) {
	public, ok := apperror.PublicFrom(err, "")
	if !ok {
		return wsOutboundEvent{}, false
	}
	return wsOutboundEvent{
		Type:      "error",
		StreamID:  strings.TrimSpace(streamID),
		SessionID: strings.TrimSpace(sessionID),
		Message:   public.Detail,
		Feedback:  public,
	}, true
}

func sendWSErrorFromError(ctx context.Context, writer *wsWriter, streamID, sessionID string, err error) {
	if event, ok := newWSAppErrorEvent(streamID, sessionID, err); ok {
		writer.SendJSONBounded(ctx, event)
		return
	}
	feedback := acpFeedbackError(err)
	if feedback == nil {
		sendWSError(ctx, writer, streamID, sessionID, wsErrorMessage(err))
		return
	}
	writer.SendJSONBounded(ctx, wsOutboundEvent{
		Type:      "error",
		StreamID:  strings.TrimSpace(streamID),
		SessionID: strings.TrimSpace(sessionID),
		Message:   strings.TrimSpace(feedback.Message),
		Feedback:  feedback,
	})
}

func (h *LocalChannelHandler) forwardWSStreamEvents(ctx, assetCtx context.Context, writer *wsWriter, botID, sessionID, streamID string, eventCh <-chan flow.WSStreamEvent) {
	_ = h.forwardWSStreamEventsResult(ctx, assetCtx, writer, botID, sessionID, streamID, eventCh)
}

func legacyWSStreamForwarder(writer *wsWriter, streamID, sessionID string, gate func(func()) bool) func(context.Context, agentpkg.StreamEvent) {
	converter := conversation.NewUIMessageStreamConverter()
	sendUIMessages := func(ctx context.Context, uiMessages []conversation.UIMessage) bool {
		for _, uiMessage := range uiMessages {
			if !writer.SendJSONBounded(ctx, wsOutboundEvent{
				Type:      "message",
				StreamID:  streamID,
				SessionID: sessionID,
				Data:      uiMessage,
			}) {
				return false
			}
		}
		return true
	}
	return func(ctx context.Context, streamEvent agentpkg.StreamEvent) {
		forward := func() {
			switch streamEvent.Type {
			case agentpkg.EventAgentStart:
				writer.SendJSONBounded(ctx, wsOutboundEvent{Type: "start", StreamID: streamID, SessionID: sessionID})
			case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
				if sendUIMessages(ctx, converter.ConvertTerminalMessages(streamEvent.Messages)) {
					writer.SendJSONBounded(ctx, wsOutboundEvent{Type: "end", StreamID: streamID, SessionID: sessionID})
				}
			case agentpkg.EventError:
				public, _ := apperror.PublicFrom(apperror.New(apperror.CodeSessionRuntimeRunFailed, nil), "")
				writer.SendJSONBounded(ctx, wsOutboundEvent{
					Type: "error", StreamID: streamID, SessionID: sessionID,
					Message: public.Detail, Feedback: public,
				})
			default:
				sendUIMessages(ctx, converter.HandleEvent(conversation.UIStreamEventFromAgentEvent(streamEvent)))
			}
		}
		if gate == nil {
			forward()
			return
		}
		gate(forward)
	}
}

func (h *LocalChannelHandler) forwardWSStreamEventsResult(ctx, assetCtx context.Context, writer *wsWriter, botID, sessionID, streamID string, eventCh <-chan flow.WSStreamEvent) error {
	forwardLegacy := legacyWSStreamForwarder(writer, streamID, sessionID, nil)
	if h.sessionRuntime != nil {
		return errors.New("runtime-managed streams require the generation-aware runtime forwarder")
	}
	outboundAssetRefs := make([]messagepkg.AssetRef, 0)
	for event := range eventCh {
		processed := h.processWSEvent(ctx, botID, event)
		for _, p := range processed {
			if refs := extractAssetRefsFromProcessedEvent(p); len(refs) > 0 {
				outboundAssetRefs = append(outboundAssetRefs, refs...)
			}

			var streamEvent agentpkg.StreamEvent
			if err := json.Unmarshal(p, &streamEvent); err != nil {
				continue
			}
			forwardLegacy(ctx, streamEvent)
		}
	}
	if len(outboundAssetRefs) > 0 {
		if err := h.resolver.LinkOutboundAssets(assetCtx, botID, sessionID, outboundAssetRefs); err != nil {
			return fmt.Errorf("link outbound assets: %w", err)
		}
	}
	return nil
}

const (
	runtimeTextBatchWindow      = 20 * time.Millisecond
	runtimeTextBatchBytes       = 4 * 1024
	runtimeFinalizationTTL      = 10 * time.Second
	runtimeFinalizationAttempts = 3
)

type wsRuntimeFinalization struct {
	runtimeSource context.Context
	assetSource   context.Context
	ttl           time.Duration
	active        bool
	runtimeCtx    context.Context
	assetCtx      context.Context
	cancelRuntime context.CancelFunc
	cancelAsset   context.CancelFunc
}

func newWSRuntimeFinalization(runtimeSource, assetSource context.Context, ttl time.Duration) *wsRuntimeFinalization {
	return &wsRuntimeFinalization{runtimeSource: runtimeSource, assetSource: assetSource, ttl: ttl}
}

// begin separates bounded completion work from cancellation of agent execution.
func (f *wsRuntimeFinalization) begin() {
	if f == nil || f.active {
		return
	}
	ttl := f.ttl
	if ttl <= 0 {
		ttl = runtimeFinalizationTTL
	}
	deadline := time.Now().Add(ttl)
	f.runtimeCtx, f.cancelRuntime = context.WithDeadline(context.WithoutCancel(f.runtimeSource), deadline)
	f.assetCtx, f.cancelAsset = context.WithDeadline(context.WithoutCancel(f.assetSource), deadline)
	f.active = true
}

func (f *wsRuntimeFinalization) runtimeContext() context.Context {
	if !f.active && f.runtimeSource.Err() != nil {
		f.begin()
	}
	if f.active {
		return f.runtimeCtx
	}
	return f.runtimeSource
}

func (f *wsRuntimeFinalization) assetContext() context.Context {
	if !f.active && f.assetSource.Err() != nil {
		f.begin()
	}
	if f.active {
		return f.assetCtx
	}
	return f.assetSource
}

func (f *wsRuntimeFinalization) close() {
	if f == nil {
		return
	}
	if f.cancelRuntime != nil {
		f.cancelRuntime()
	}
	if f.cancelAsset != nil {
		f.cancelAsset()
	}
}

func retryRuntimeFinalization(ctx context.Context, operation func() error) error {
	var lastErr error
	delay := 50 * time.Millisecond
	for attempt := 0; attempt < runtimeFinalizationAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(lastErr, err)
		}
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt+1 == runtimeFinalizationAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(lastErr, ctx.Err())
		case <-timer.C:
		}
		delay *= 2
	}
	return lastErr
}

func (h *LocalChannelHandler) forwardRuntimeWSStreamEvents(ctx, assetCtx context.Context, handle sessionruntime.RunHandle, eventCh <-chan flow.WSStreamEvent, cancel context.CancelFunc, forwardLegacy func(context.Context, agentpkg.StreamEvent)) error {
	botID := handle.BotID
	sessionID := handle.SessionID
	streamID := handle.StreamID
	var pending *agentpkg.StreamEvent
	var timer *time.Timer
	var timerC <-chan time.Time
	var runtimeErr error
	outboundAssetRefs := make([]messagepkg.AssetRef, 0)
	finalization := newWSRuntimeFinalization(ctx, assetCtx, runtimeFinalizationTTL)
	defer finalization.close()
	linkOutboundAssets := func() error { //nolint:contextcheck // Finalization intentionally outlives execution cancellation.
		if len(outboundAssetRefs) == 0 {
			return nil
		}
		if err := h.resolver.LinkOutboundAssets(finalization.assetContext(), botID, sessionID, outboundAssetRefs); err != nil {
			return fmt.Errorf("link outbound assets: %w", err)
		}
		outboundAssetRefs = nil
		return nil
	}

	stopTimer := func() {
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	commit := func(event agentpkg.StreamEvent) { //nolint:contextcheck // Finalization intentionally outlives execution cancellation.
		if runtimeErr != nil {
			return
		}
		if _, err := h.sessionRuntime.HandleAgentEvent(finalization.runtimeContext(), handle, event); err != nil {
			h.logger.Warn("runtime state update failed", slog.String("stage", string(event.Type)), slog.Any("error", err), slog.String("stream_id", streamID))
			runtimeErr = fmt.Errorf("update session runtime: %w", err)
			if cancel != nil {
				cancel()
			}
		}
	}
	flush := func() {
		if pending == nil {
			return
		}
		commit(*pending)
		pending = nil
		stopTimer()
	}
	enqueue := func(event agentpkg.StreamEvent) {
		batchable := event.Type == agentpkg.EventTextDelta || event.Type == agentpkg.EventReasoningDelta
		if !batchable || event.Delta == "" {
			flush()
			commit(event)
			return
		}
		if pending != nil && pending.Type != event.Type {
			flush()
		}
		if pending == nil {
			pendingEvent := event
			pending = &pendingEvent
			timer = time.NewTimer(runtimeTextBatchWindow)
			timerC = timer.C
		} else {
			pending.Delta += event.Delta
		}
		if len(pending.Delta) >= runtimeTextBatchBytes {
			flush()
		}
	}
	process := func(raw flow.WSStreamEvent) { //nolint:contextcheck // Terminal processing intentionally switches to finalization contexts.
		if runtimeErr != nil {
			return
		}
		var rawEvent agentpkg.StreamEvent
		if json.Unmarshal(raw, &rawEvent) == nil && rawEvent.IsTerminal() {
			finalization.begin()
		}
		for _, processed := range h.processWSEvent(finalization.assetContext(), botID, raw) {
			if refs := extractAssetRefsFromProcessedEvent(processed); len(refs) > 0 {
				outboundAssetRefs = append(outboundAssetRefs, refs...)
			}
			var streamEvent agentpkg.StreamEvent
			if err := json.Unmarshal(processed, &streamEvent); err != nil {
				continue
			}
			terminal := streamEvent.IsTerminal()
			if terminal {
				flush()
				var assetErr error
				if streamEvent.HistoryCommitted {
					assetErr = retryRuntimeFinalization(finalization.assetContext(), linkOutboundAssets)
				}
				canonicalReady := streamEvent.HistoryCommitted && assetErr == nil
				finalizationError := ""
				if assetErr != nil {
					finalizationError = assetErr.Error()
				}
				if _, err := h.sessionRuntime.FinalizeAgentEvent(finalization.runtimeContext(), handle, streamEvent, canonicalReady, finalizationError); err != nil {
					runtimeErr = fmt.Errorf("finalize session runtime: %w", err)
					if cancel != nil {
						cancel()
					}
					return
				}
				if forwardLegacy != nil {
					forwardLegacy(finalization.runtimeContext(), streamEvent)
				}
				if assetErr != nil {
					runtimeErr = assetErr
					if cancel != nil {
						cancel()
					}
				}
				return
			}
			enqueue(streamEvent)
			if runtimeErr != nil {
				return
			}
			if forwardLegacy != nil {
				forwardLegacy(finalization.runtimeContext(), streamEvent)
			}
		}
	}

	for eventCh != nil {
		select {
		case event, ok := <-eventCh:
			if !ok {
				eventCh = nil
				break
			}
			process(event)
		case <-timerC:
			flush()
		}
	}
	flush()
	if runtimeErr == nil {
		runtimeErr = linkOutboundAssets()
	}
	return runtimeErr
}

type wsStreamRunner func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}, injectCh <-chan conversation.InjectMessage) error

func runtimeOperationFromPreparedReplacement(prepared flow.PreparedReplacementWS) *sessionruntime.RunOperationView {
	operation := prepared.Operation
	return &sessionruntime.RunOperationView{
		Kind:                 strings.TrimSpace(operation.Kind),
		ReplaceFromMessageID: strings.TrimSpace(operation.ReplaceFromMessageID),
		ReplacementUserTurn:  operation.ReplacementUserTurn,
	}
}

func (h *LocalChannelHandler) routeWSRuntimeResponse(baseCtx, connCtx context.Context, writer *wsWriter, botID, sessionID, targetID, actionID string, msg wsClientMessage, payload any, deferred func()) {
	if h == nil || h.sessionRuntime == nil {
		deferred()
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		h.sendWSSidebandResult(connCtx, writer, msg, actionID, err)
		return
	}
	commandType := actionID
	if !h.tryAcquireRuntimeCommand() {
		h.sendWSSidebandResult(connCtx, writer, msg, actionID, sessionruntime.ErrCommandBusy)
		return
	}
	go func() {
		defer h.releaseRuntimeCommand()
		handled, dispatchErr := h.sessionRuntime.DispatchActiveCommand(baseCtx, botID, sessionID, commandType, targetID, raw)
		if errors.Is(dispatchErr, sessionruntime.ErrCommandTargetNotActive) {
			handled = false
			dispatchErr = nil
		}
		if !handled {
			deferred()
			return
		}
		if connCtx.Err() == nil {
			h.sendWSSidebandResult(connCtx, writer, msg, actionID, dispatchErr)
		}
	}()
}

func (h *LocalChannelHandler) startWSStreamWithAdmissionBuilder(baseCtx, connCtx context.Context, activeStreams *wsStreamRegistry, writer *wsWriter, botID, sessionID, streamID, logLabel string, onFinish func(), activationOptions runtimefence.ActivationOptions, admissionBuilder func(context.Context) (sessionruntime.RunAdmissionView, error), runner wsStreamRunner) {
	streamCtx, streamCancel := context.WithCancel(baseCtx)
	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 16)
	if err := activeStreams.register(&activeWSStream{
		streamID:  streamID,
		sessionID: sessionID,
		cancel:    streamCancel,
		abortCh:   abortCh,
	}); err != nil {
		streamCancel()
		if onFinish != nil {
			onFinish()
		}
		h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
		return
	}
	releaseCompaction := func() {}
	if h.resolver != nil {
		releaseCompaction = h.resolver.DeferSessionCompaction(botID, sessionID, streamID)
	}
	go func() {
		defer streamCancel()
		defer releaseCompaction()
		if h.sessionRuntime == nil {
			defer close(injectCh)
		}
		var persistenceFence runtimefence.Fence
		var ownershipCancel context.CancelCauseFunc
		var runHandle sessionruntime.RunHandle
		runtimeCtx := streamCtx
		assetCtx := streamCtx
		if h.sessionRuntime != nil {
			if h.sessionRuntime.IsDistributed() {
				// Redis first reserves the run in admitting state. PostgreSQL
				// activation inside the admission builder below is the durable
				// persistence cutover; it serializes behind any older fenced writer.
				fence, err := h.resolver.AllocateRuntimePersistenceFence(streamCtx, botID, sessionID)
				if err != nil {
					activeStreams.finish(streamID, sessionID)
					if onFinish != nil {
						onFinish()
					}
					if connCtx.Err() == nil && !errors.Is(err, context.Canceled) {
						h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
					}
					return
				}
				persistenceFence = fence
				runtimeCtx = runtimefence.WithContext(streamCtx, fence)
				assetCtx = runtimefence.WithContext(streamCtx, fence)
				authorityCtx, revokeOwnership := context.WithCancelCause(context.WithoutCancel(runtimeCtx))
				ownershipCancel = revokeOwnership
				runtimeCtx = flow.WithTerminalHookAuthority(runtimeCtx, agentpkg.TerminalHookAuthority{
					Context: authorityCtx,
					Validate: func(ctx context.Context) error {
						return h.sessionRuntime.ValidateRunOwnership(ctx, runHandle)
					},
				})
			}
			runtimeAdmissionBuilder := func(ctx context.Context, _ sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
				return admissionBuilder(ctx)
			}
			if persistenceFence.Valid() {
				runtimeAdmissionBuilder = func(ctx context.Context, handle sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
					ctx = runtimefence.WithContext(ctx, persistenceFence)
					if err := h.sessionRuntime.ValidateRunOwnership(ctx, handle); err != nil {
						return sessionruntime.RunAdmissionView{}, err
					}
					if err := h.resolver.ActivateRuntimePersistenceFenceWithOptions(ctx, persistenceFence, activationOptions); err != nil {
						return sessionruntime.RunAdmissionView{}, err
					}
					if err := h.sessionRuntime.ValidateRunOwnership(ctx, handle); err != nil {
						return sessionruntime.RunAdmissionView{}, err
					}
					return admissionBuilder(ctx)
				}
			}
			var err error
			runHandle, err = h.sessionRuntime.StartRunWithOptions(runtimeCtx, sessionruntime.RunStartOptions{
				BotID:            botID,
				SessionID:        sessionID,
				StreamID:         streamID,
				AdmissionBuilder: runtimeAdmissionBuilder,
				OwnershipCancel:  ownershipCancel,
				AbortCh:          abortCh,
				Cancel:           streamCancel,
				InjectCh:         injectCh,
			})
			if err != nil {
				activeStreams.finish(streamID, sessionID)
				if onFinish != nil {
					onFinish()
				}
				if connCtx.Err() == nil && !errors.Is(err, context.Canceled) {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
				}
				return
			}
		} else if admissionBuilder != nil {
			if _, err := admissionBuilder(streamCtx); err != nil {
				activeStreams.finish(streamID, sessionID)
				if onFinish != nil {
					onFinish()
				}
				if connCtx.Err() == nil && !errors.Is(err, context.Canceled) {
					sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				}
				return
			}
		}

		eventCh := make(chan flow.WSStreamEvent, 64)
		forwardDone := make(chan error, 1)
		go func() {
			if h.sessionRuntime != nil {
				forwardDone <- h.forwardRuntimeWSStreamEvents(runtimeCtx, assetCtx, runHandle, eventCh, streamCancel, legacyWSStreamForwarder(writer, streamID, sessionID, func(forward func()) bool {
					return activeStreams.forwardLegacyIfEnabled(sessionID, forward)
				}))
				return
			}
			forwardDone <- h.forwardWSStreamEventsResult(runtimeCtx, assetCtx, writer, botID, sessionID, streamID, eventCh)
		}()

		runnerCtx := runtimeCtx
		if h.sessionRuntime != nil {
			runnerCtx = flow.WithTerminalEventDeliveryTimeout(runnerCtx, runtimeFinalizationTTL)
			runnerCtx = flow.WithPersistenceGuard(runnerCtx, func(ctx context.Context) error {
				return h.sessionRuntime.ValidateRunOwnership(ctx, runHandle)
			})
		}
		err := func() error {
			defer close(eventCh)
			return runner(runnerCtx, eventCh, abortCh, injectCh)
		}()
		forwardErr := <-forwardDone
		if forwardErr != nil {
			err = forwardErr
		}
		activeStreams.finish(streamID, sessionID)
		if onFinish != nil {
			onFinish()
		}
		if err != nil {
			if errors.Is(err, sessionruntime.ErrTerminalCommitPending) {
				h.logger.Warn("runtime terminal commit deferred for retry", slog.Any("error", err), slog.String("stream_id", streamID))
				return
			}
			if flow.IsCanceledStreamError(err) && streamCtx.Err() != nil {
				h.logger.Debug("ws stream canceled",
					slog.String("operation", logLabel),
					slog.String("bot_id", botID),
					slog.String("session_id", sessionID),
					slog.String("stream_id", streamID),
				)
				if h.sessionRuntime != nil {
					if finishErr := h.sessionRuntime.FinishRun(context.WithoutCancel(baseCtx), runHandle, "", ""); finishErr != nil {
						h.logger.Warn("finish canceled runtime run failed", slog.Any("error", finishErr), slog.String("stream_id", streamID))
					}
				}
				return
			}
			privateErr := err
			if cause := apperror.CauseOf(err); cause != nil {
				privateErr = cause
			}
			h.logger.Error("ws stream error",
				slog.String("operation", logLabel),
				slog.String("error_code", string(apperror.CodeOf(err))),
				slog.Any("error", privateErr),
				slog.String("bot_id", botID),
				slog.String("session_id", sessionID))
			if connCtx.Err() == nil && h.sessionRuntime == nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
			}
			if h.sessionRuntime != nil {
				if finishErr := h.sessionRuntime.FinishRun(context.WithoutCancel(baseCtx), runHandle, sessionruntime.RunStatusErrored, wsErrorMessage(err)); finishErr != nil {
					h.logger.Warn("finish errored runtime run failed", slog.Any("error", finishErr), slog.String("stream_id", streamID))
				}
			}
			return
		}
		if h.sessionRuntime != nil {
			if finishErr := h.sessionRuntime.FinishRun(context.WithoutCancel(baseCtx), runHandle, "", ""); finishErr != nil {
				h.logger.Warn("finish runtime run failed", slog.Any("error", finishErr), slog.String("stream_id", streamID))
			}
		}
	}()
}

// HandleWebSocket godoc
// @Summary WebSocket chat endpoint
// @Description Upgrade to WebSocket for bidirectional chat streaming with abort support.
// @Tags local-channel
// @Param bot_id path string true "Bot ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/web/ws [get].
func (h *LocalChannelHandler) HandleWebSocket(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	_, perms, err := h.authorizeBotSessionAccess(c.Request().Context(), channelIdentityID, botID)
	if err != nil {
		return err
	}
	if !canOpenLocalWebSocket(perms) {
		if err := h.ensureBotParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
			return err
		}
	}
	if h.resolver == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "resolver not configured")
	}

	conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	rawToken := extractRawBearerToken(c)
	bearerToken := "Bearer " + rawToken

	writer := newWSWriter(conn)
	defer func() {
		_ = conn.Close()
		writer.Close()
	}()

	connCtx, connCancel := context.WithCancel(context.Background())
	defer connCancel()
	streamBaseCtx := context.WithoutCancel(c.Request().Context())
	activeStreams := newWSStreamRegistry()
	runtimeSubscriptions := make(map[string]*wsRuntimeSubscription)
	var runtimeSubscriptionsMu sync.Mutex
	defer func() {
		runtimeSubscriptionsMu.Lock()
		subscriptions := make([]*wsRuntimeSubscription, 0, len(runtimeSubscriptions))
		for key, sub := range runtimeSubscriptions {
			delete(runtimeSubscriptions, key)
			subscriptions = append(subscriptions, sub)
		}
		runtimeSubscriptionsMu.Unlock()
		for _, sub := range subscriptions {
			sub.stop()
		}
	}()

	startRuntimeSubscription := func(msg wsClientMessage) {
		sessionID := strings.TrimSpace(msg.SessionID)
		if sessionID == "" {
			h.sendWSSidebandResult(connCtx, writer, msg, "runtime_subscribe", errors.New("session_id is required"))
			return
		}
		subKey := sessionruntime.Key{BotID: botID, SessionID: sessionID}.String()
		setupCtx, cancelSetup := context.WithCancel(connCtx)
		entry := &wsRuntimeSubscription{
			close: sync.OnceFunc(cancelSetup),
			done:  make(chan struct{}),
		}
		runtimeSubscriptionsMu.Lock()
		oldSubscription := runtimeSubscriptions[subKey]
		runtimeSubscriptions[subKey] = entry
		runtimeSubscriptionsMu.Unlock()
		if oldSubscription != nil {
			oldSubscription.stop()
		}

		go func() {
			defer close(entry.done)
			defer func() {
				runtimeSubscriptionsMu.Lock()
				if runtimeSubscriptions[subKey] == entry {
					delete(runtimeSubscriptions, subKey)
				}
				runtimeSubscriptionsMu.Unlock()
			}()
			setupTimer := time.AfterFunc(h.runtimeSubscriptionSetupTimeout(), func() {
				cancelSetup()
				_ = conn.Close()
			})
			defer setupTimer.Stop()
			if err := h.authorizeWSRuntimeSessionContext(setupCtx, channelIdentityID, botID, sessionID); err != nil {
				if setupCtx.Err() == nil {
					h.sendWSSidebandResult(setupCtx, writer, msg, "runtime_subscribe", err)
				}
				return
			}
			h.logger.Debug("ws runtime subscribe", slog.String("bot_id", botID), slog.String("session_id", sessionID))

			var (
				initial sessionruntime.Event
				sub     sessionruntime.Subscription
			)
			if h.sessionRuntime == nil {
				emptySnapshot := sessionruntime.EmptySnapshot(botID, sessionID)
				initial = sessionruntime.Event{
					Type: sessionruntime.EventRuntimeSnapshot, BotID: botID, SessionID: sessionID, Snapshot: &emptySnapshot,
				}
			} else {
				var err error
				sub, err = h.sessionRuntime.Subscribe(setupCtx, botID, sessionID)
				if err != nil {
					if setupCtx.Err() == nil {
						h.sendWSSidebandResult(setupCtx, writer, msg, "runtime_subscribe", err)
					}
					return
				}
				defer sub.Close()
				select {
				case <-setupCtx.Done():
					return
				case event, ok := <-sub.C:
					if !ok || event.Type != sessionruntime.EventRuntimeSnapshot || event.Snapshot == nil {
						h.sendWSSidebandResult(setupCtx, writer, msg, "runtime_subscribe", errors.New("runtime subscription did not provide an initial snapshot"))
						return
					}
					initial = event
				}
			}
			if !setupTimer.Stop() || setupCtx.Err() != nil {
				return
			}
			if !activeStreams.enableRuntimeProtocolAndSend(sessionID, func() {
				writer.SendJSONBounded(setupCtx, initial)
			}) {
				h.sendWSSidebandResult(setupCtx, writer, msg, "runtime_subscribe", errors.New("runtime session subscription limit reached; reconnect required"))
				_ = conn.Close()
				return
			}
			h.sendWSSidebandResult(setupCtx, writer, msg, "runtime_subscribe", nil)
			if h.sessionRuntime == nil {
				<-setupCtx.Done()
				return
			}

			authTicker := time.NewTicker(h.runtimeSubscriptionAuthInterval())
			defer authTicker.Stop()
			for {
				select {
				case <-setupCtx.Done():
					return
				case <-authTicker.C:
					authCtx, cancelAuth := context.WithTimeout(setupCtx, h.runtimeSubscriptionAuthInterval())
					authErr := h.authorizeWSRuntimeSessionContext(authCtx, channelIdentityID, botID, sessionID)
					cancelAuth()
					if authErr != nil {
						if setupCtx.Err() != nil {
							return
						}
						h.logger.Info("ws runtime access revoked", slog.String("bot_id", botID), slog.String("session_id", sessionID))
						_ = conn.Close()
						return
					}
				case event, ok := <-sub.C:
					if !ok {
						return
					}
					if !writer.SendJSONBounded(setupCtx, event) {
						return
					}
				}
			}
		}()
	}

	for {
		_, raw, readErr := conn.ReadMessage()
		if readErr != nil {
			connCancel()
			h.logger.Debug("ws disconnected; active stream can finish in background",
				slog.String("bot_id", botID),
				slog.Any("error", readErr),
			)
			break
		}
		var msg wsClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			h.logger.Warn("ws: unmarshal failed",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
			writer.SendJSONBounded(connCtx, map[string]string{"type": "error", "message": "invalid message format"})
			continue
		}

		switch msg.Type {
		case "runtime_subscribe":
			startRuntimeSubscription(msg)

		case "runtime_unsubscribe":
			sessionID := strings.TrimSpace(msg.SessionID)
			if sessionID == "" {
				sendWSError(connCtx, writer, strings.TrimSpace(msg.StreamID), "", "session_id is required")
				continue
			}
			subKey := sessionruntime.Key{BotID: botID, SessionID: sessionID}.String()
			runtimeSubscriptionsMu.Lock()
			sub := runtimeSubscriptions[subKey]
			if sub != nil {
				delete(runtimeSubscriptions, subKey)
			}
			runtimeSubscriptionsMu.Unlock()
			if sub != nil {
				sub.stop()
			}
			h.sendWSSidebandResult(connCtx, writer, msg, "runtime_unsubscribe", nil)

		case "abort":
			streamID := strings.TrimSpace(msg.StreamID)
			sessionID := strings.TrimSpace(msg.SessionID)
			generation := strings.TrimSpace(msg.Generation)
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if generation == "" {
				if localSessionID, ok := activeStreams.sessionForStream(streamID, sessionID); ok {
					if err := h.authorizeWSRuntimeSession(c, channelIdentityID, botID, localSessionID); err != nil {
						sendWSErrorFromError(connCtx, writer, streamID, localSessionID, err)
						continue
					}
					if h.sessionRuntime == nil {
						if activeStreams.abort(streamID, localSessionID) {
							continue
						}
					} else if activeStreams.generationlessAbortAllowed(streamID, localSessionID) {
						// Once admitted, Manager owns the state transition to aborting.
						// The local registry is only a fallback for the short window before
						// runtime control registration completes.
						aborted, abortErr := h.sessionRuntime.Abort(streamBaseCtx, botID, localSessionID, streamID)
						if abortErr != nil {
							h.sendWSRuntimeError(connCtx, writer, streamID, localSessionID, abortErr, apperror.CodeSessionRuntimeCommandFailed)
							continue
						}
						if aborted {
							continue
						}
						if activeStreams.abort(streamID, localSessionID) {
							// The runtime goroutine may register its control between the
							// first lookup and local cancellation. Recheck after cancellation;
							// StartRun also checks the canceled context before registration.
							if _, recheckErr := h.sessionRuntime.Abort(streamBaseCtx, botID, localSessionID, streamID); recheckErr != nil && !errors.Is(recheckErr, sessionruntime.ErrCommandTargetNotActive) {
								h.sendWSRuntimeError(connCtx, writer, streamID, localSessionID, recheckErr, apperror.CodeSessionRuntimeCommandFailed)
							}
							continue
						}
					}
				}
			}
			if sessionID == "" {
				sendWSError(connCtx, writer, streamID, "", "session_id is required")
				continue
			}
			if err := h.authorizeWSRuntimeSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			aborted := false
			if h.sessionRuntime != nil {
				if generation == "" {
					sendWSError(connCtx, writer, streamID, sessionID, "generation is required")
					continue
				}
				var err error
				aborted, err = h.sessionRuntime.AbortRun(streamBaseCtx, sessionruntime.RunHandle{
					BotID: botID, SessionID: sessionID, StreamID: streamID, Generation: generation,
				})
				if err != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeCommandFailed)
					continue
				}
			}
			if !aborted {
				if h.sessionRuntime == nil && activeStreams.abort(streamID, sessionID) {
					continue
				}
				if h.sessionRuntime != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, sessionruntime.ErrCommandTargetMismatch, apperror.CodeSessionRuntimeTargetNotActive)
				}
			}

		case "steer_current_run":
			streamID := strings.TrimSpace(msg.StreamID)
			sessionID := strings.TrimSpace(msg.SessionID)
			generation := strings.TrimSpace(msg.Generation)
			if sessionID == "" {
				sendWSError(connCtx, writer, streamID, "", "session_id is required")
				continue
			}
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if strings.TrimSpace(msg.Text) == "" {
				sendWSError(connCtx, writer, streamID, sessionID, "text is required")
				continue
			}
			if err := h.authorizeWSRuntimeSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if h.sessionRuntime == nil {
				h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, sessionruntime.ErrManagerClosed, apperror.CodeSessionRuntimeUnavailable)
				continue
			}
			if generation == "" {
				sendWSError(connCtx, writer, streamID, sessionID, "generation is required")
				continue
			}
			if _, err := h.sessionRuntime.SteerRun(streamBaseCtx, sessionruntime.RunHandle{
				BotID: botID, SessionID: sessionID, StreamID: streamID, Generation: generation,
			}, msg.Text); err != nil {
				h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeCommandFailed)
			}

		case "tool_approval_response":
			sessionID := strings.TrimSpace(msg.SessionID)
			if sessionID == "" {
				sendWSError(connCtx, writer, strings.TrimSpace(msg.StreamID), "", "session_id is required")
				continue
			}
			streamID := strings.TrimSpace(msg.StreamID)
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			explicitID := strings.TrimSpace(msg.ApprovalID)
			if explicitID == "" && msg.ShortID > 0 {
				explicitID = strconv.Itoa(msg.ShortID)
			}
			responseMsg := msg
			responseInput := flow.ToolApprovalResponseInput{
				BotID:                  botID,
				SessionID:              sessionID,
				ActorChannelIdentityID: channelIdentityID,
				ActorUserID:            channelIdentityID,
				ApprovalID:             strings.TrimSpace(msg.ApprovalID),
				ExplicitID:             explicitID,
				Decision:               strings.TrimSpace(msg.Decision),
				Reason:                 strings.TrimSpace(msg.Reason),
				ChatToken:              bearerToken,
			}
			deferred := func() {
				preserved, err := h.resolver.PrepareToolApprovalResponse(streamBaseCtx, responseInput)
				if err != nil {
					h.sendWSSidebandResult(connCtx, writer, responseMsg, "tool_approval_response", err)
					return
				}
				suppressActivePromptAttach := activeStreams.hasSession(sessionID)
				releaseWSMessageTurn := h.enterWSMessageTurn(botID, sessionID, streamID)
				h.startWSStreamWithAdmissionBuilder(streamBaseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, "ws approval stream error", releaseWSMessageTurn, runtimefence.ActivationOptions{PreserveDecision: &preserved}, func(context.Context) (sessionruntime.RunAdmissionView, error) {
					return sessionruntime.RunAdmissionView{}, nil
				},
					func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
						input := responseInput
						input.SuppressActivePromptAttach = suppressActivePromptAttach
						return h.resolver.RespondToolApproval(ctx, input, eventCh)
					},
				)
			}
			routedInput := responseInput
			routedInput.ChatToken = ""
			routedInput.SuppressActivePromptAttach = true
			h.routeWSRuntimeResponse(streamBaseCtx, connCtx, writer, botID, sessionID, explicitID, sessionruntime.CommandToolApprovalResponse, responseMsg, routedInput, deferred)

		case "user_input_response":
			sessionID := strings.TrimSpace(msg.SessionID)
			if sessionID == "" {
				sendWSError(connCtx, writer, strings.TrimSpace(msg.StreamID), "", "session_id is required")
				continue
			}
			streamID := strings.TrimSpace(msg.StreamID)
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			explicitID := strings.TrimSpace(msg.UserInputID)
			if explicitID == "" && msg.ShortID > 0 {
				explicitID = strconv.Itoa(msg.ShortID)
			}
			responseMsg := msg
			responseInput := flow.UserInputResponseInput{
				BotID:                  botID,
				SessionID:              sessionID,
				ActorChannelIdentityID: channelIdentityID,
				ActorUserID:            channelIdentityID,
				UserInputID:            strings.TrimSpace(msg.UserInputID),
				ExplicitID:             explicitID,
				Answers:                msg.Answers,
				Canceled:               msg.Canceled,
				Reason:                 strings.TrimSpace(msg.Reason),
				ChatToken:              bearerToken,
			}
			deferred := func() {
				preserved, err := h.resolver.PrepareUserInputResponseTarget(streamBaseCtx, responseInput)
				if err != nil {
					h.sendWSSidebandResult(connCtx, writer, responseMsg, "user_input_response", err)
					return
				}
				suppressActivePromptAttach := activeStreams.hasSession(sessionID)
				releaseWSMessageTurn := h.enterWSMessageTurn(botID, sessionID, streamID)
				h.startWSStreamWithAdmissionBuilder(streamBaseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, "ws user input stream error", releaseWSMessageTurn, runtimefence.ActivationOptions{PreserveDecision: &preserved}, func(context.Context) (sessionruntime.RunAdmissionView, error) {
					return sessionruntime.RunAdmissionView{}, nil
				},
					func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
						input := responseInput
						input.SuppressActivePromptAttach = suppressActivePromptAttach
						return h.resolver.RespondUserInput(ctx, input, eventCh)
					},
				)
			}
			routedInput := responseInput
			routedInput.ChatToken = ""
			routedInput.SuppressActivePromptAttach = true
			h.routeWSRuntimeResponse(streamBaseCtx, connCtx, writer, botID, sessionID, explicitID, sessionruntime.CommandUserInputResponse, responseMsg, routedInput, deferred)

		case "message":
			text := strings.TrimSpace(msg.Text)
			sessionID := strings.TrimSpace(msg.SessionID)
			streamID := strings.TrimSpace(msg.StreamID)
			workspaceTargetID := strings.TrimSpace(msg.WorkspaceTargetID)

			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if sessionID != "" {
				if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
					sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
					continue
				}
			}
			if err := authorizeWorkspaceTargetSelection(perms, workspaceTargetID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if workspaceTargetID != "" {
				if err := h.resolver.ValidateWorkspaceTarget(streamBaseCtx, botID, workspaceTargetID); err != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
					continue
				}
			}

			hasRequestedSkills := len(msg.RequestedSkills) > 0
			if hasRequestedSkills && strings.HasPrefix(text, "/") {
				sendWSCommandError(connCtx, writer, msg, slash.CodeInvalidSkillSlashSyntax)
				continue
			}
			decision := h.classifyWebSlash(text, len(msg.Attachments) > 0, slash.SurfaceWebWS)
			var pendingSkillIntent *slash.SkillIntent
			switch decision.Kind {
			case slash.DecisionNormalChat:
			case slash.DecisionCommandAction:
				if err := h.authorizeWSChatAccess(streamBaseCtx, channelIdentityID, botID); err != nil {
					sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
					continue
				}
				actionID := webActionID(decision.Command.Resource, decision.Command.Action)
				skillActivationAllowed := true
				if strings.TrimSpace(sessionID) != "" {
					supported, supportErr := h.wsSessionSupportsRequestedSkills(streamBaseCtx, sessionID)
					if supportErr != nil {
						sendWSErrorFromError(connCtx, writer, streamID, sessionID, supportErr)
						continue
					}
					skillActivationAllowed = supported
				}
				result, slashErr := h.executeWebQuickAction(streamBaseCtx, botID, actionID, skillActivationAllowed)
				if slashErr != nil {
					sendWSCommandError(connCtx, writer, msg, slashErr.Code)
				} else {
					sendWSCommandResult(connCtx, writer, msg, actionID, result)
				}
				continue
			case slash.DecisionSkillIntent:
				intent := decision.SkillIntent
				pendingSkillIntent = &intent
			case slash.DecisionUnsupportedCommand, slash.DecisionUnknownSlash, slash.DecisionReject:
				code := decision.Code
				if code == "" {
					code = slash.CodeUnknownSlash
				}
				sendWSCommandError(connCtx, writer, msg, code)
				continue
			default:
				sendWSCommandError(connCtx, writer, msg, slash.CodeUnknownSlash)
				continue
			}

			hasSkillActivation := hasRequestedSkills || pendingSkillIntent != nil
			if text == "" && len(msg.Attachments) == 0 && !hasSkillActivation {
				sendWSError(connCtx, writer, streamID, sessionID, "message text or attachments required")
				continue
			}
			if sessionID == "" || hasSkillActivation {
				if err := h.authorizeWSChatAccess(streamBaseCtx, channelIdentityID, botID); err != nil {
					sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
					continue
				}
			}
			chatAttachments, attachmentErr := parseWSClientAttachments(msg.Attachments)
			if attachmentErr != nil {
				code := slashErrorCode(attachmentErr)
				if code == "" {
					code = slash.CodeReservedSkillMetadata
				}
				sendWSCommandError(connCtx, writer, msg, code)
				continue
			}

			var requestedSkillContexts []conversation.RequestedSkillContext
			var skillActivation *conversation.SkillActivation
			userMessageKind := ""
			userVisibleText := ""
			streamText := text
			streamModelText := text
			var err error
			sessionAuthorized := false
			var releaseActiveWSTurn func()
			releaseActiveWSTurnNow := func() {
				if releaseActiveWSTurn != nil {
					releaseActiveWSTurn()
					releaseActiveWSTurn = nil
				}
			}
			if sessionID != "" {
				sessionAuthorized = true
				if hasSkillActivation {
					if shouldRejectWSSkillActivationForActiveStream(activeStreams, h.resolver, botID, sessionID, true) {
						sendWSCommandError(connCtx, writer, msg, slash.CodeUnsupportedSkillSlashContext)
						continue
					}
					supported, supportErr := h.wsSessionSupportsRequestedSkills(streamBaseCtx, sessionID)
					if supportErr != nil {
						sendWSErrorFromError(connCtx, writer, streamID, sessionID, supportErr)
						continue
					}
					if !supported {
						sendWSCommandError(connCtx, writer, msg, slash.CodeUnsupportedSkillSlashContext)
						continue
					}
					var reserved bool
					releaseActiveWSTurn, reserved = h.reserveWSRequestedSkillTurn(botID, sessionID, streamID)
					if !reserved {
						sendWSCommandError(connCtx, writer, msg, slash.CodeUnsupportedSkillSlashContext)
						continue
					}
				} else {
					releaseActiveWSTurn = h.enterWSMessageTurn(botID, sessionID, streamID)
				}
			}

			if hasRequestedSkills {
				requestedSkillContexts, err = h.resolveWebRequestedSkillContexts(streamBaseCtx, botID, msg.RequestedSkills)
				if err != nil {
					code := slashErrorCode(err)
					if code == "" {
						code = slash.CodeUnsupportedSkillSlashContext
					}
					sendWSCommandError(connCtx, writer, msg, code)
					releaseActiveWSTurnNow()
					continue
				}
			}
			if pendingSkillIntent != nil {
				requestedSkillContexts, err = h.resolveWebTextRequestedSkillContexts(streamBaseCtx, botID, pendingSkillIntent.Names)
				if err != nil {
					code := slashErrorCode(err)
					if code == "" {
						code = slash.CodeUnsupportedSkillSlashContext
					}
					sendWSCommandError(connCtx, writer, msg, code)
					releaseActiveWSTurnNow()
					continue
				}
			}
			if hasSkillActivation {
				prompt := text
				if pendingSkillIntent != nil {
					prompt = pendingSkillIntent.Prompt
				}
				skillActivation = conversation.NewSkillActivation(requestedSkillContexts, prompt)
				streamText = strings.TrimSpace(prompt)
				streamModelText = strings.TrimSpace(conversation.SkillActivationModelQuery(skillActivation))
				userMessageKind = conversation.UserMessageKindSkillActivation
				userVisibleText = strings.TrimSpace(prompt)
			}

			if sessionID == "" {
				if h.sessionService == nil {
					sendWSError(connCtx, writer, streamID, "", "session service not configured")
					releaseActiveWSTurnNow()
					continue
				}
				created, createErr := h.createWSChatSession(streamBaseCtx, botID, channelIdentityID)
				if createErr != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, "", createErr, apperror.CodeSessionRuntimeRunFailed)
					releaseActiveWSTurnNow()
					continue
				}
				sessionID = created.ID
				msg.SessionID = sessionID
				if hasSkillActivation {
					var reserved bool
					releaseActiveWSTurn, reserved = h.reserveWSRequestedSkillTurn(botID, sessionID, streamID)
					if !reserved {
						sendWSCommandError(connCtx, writer, msg, slash.CodeUnsupportedSkillSlashContext)
						continue
					}
				} else {
					releaseActiveWSTurn = h.enterWSMessageTurn(botID, sessionID, streamID)
				}
				writer.SendJSONBounded(connCtx, map[string]any{
					"type":       "session_created",
					"stream_id":  streamID,
					"session_id": sessionID,
				})
			}
			if !sessionAuthorized {
				if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
					sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
					releaseActiveWSTurnNow()
					continue
				}
			}
			acpInfo, err := h.authorizeWSACPExecution(c.Request().Context(), channelIdentityID, botID, sessionID)
			if err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				releaseActiveWSTurnNow()
				continue
			}
			if acpInfo.IsACP && len(requestedSkillContexts) > 0 {
				sendWSCommandError(connCtx, writer, msg, slash.CodeUnsupportedSkillSlashContext)
				releaseActiveWSTurnNow()
				continue
			}
			streamToken := bearerToken
			if acpInfo.IsACP {
				streamToken = h.issueRuntimeOwnerBearerToken(acpInfo.RuntimeOwnerAccountID, bearerToken)
			}
			externalMessageID := streamID
			var preparedReq *conversation.ChatRequest
			buildRequest := func(ingestedAttachments []conversation.ChatAttachment) conversation.ChatRequest {
				return conversation.ChatRequest{
					BotID:                   botID,
					ChatID:                  botID,
					SessionID:               sessionID,
					StreamID:                streamID,
					UserID:                  channelIdentityID,
					SourceChannelIdentityID: channelIdentityID,
					ExternalMessageID:       externalMessageID,
					ConversationType:        channel.ConversationTypePrivate,
					Query:                   streamText,
					ModelQuery:              streamModelText,
					RawQuery:                userVisibleText,
					UserMessageKind:         userMessageKind,
					UserVisibleText:         userVisibleText,
					SkillActivation:         skillActivation,
					Token:                   streamToken,
					ChatToken:               bearerToken,
					CurrentChannel:          h.channelType.String(),
					ReplyTarget:             botID,
					Channels:                []string{h.channelType.String()},
					Attachments:             ingestedAttachments,
					RequestedSkills:         requestedSkillContexts,
					WorkspaceTargetID:       workspaceTargetID,
					SkipMemoryExtraction:    hasSkillActivation && userVisibleText == "",
					SkipTitleGeneration:     hasSkillActivation && userVisibleText == "",
					Model:                   strings.TrimSpace(msg.ModelID),
					ReasoningEffort:         strings.TrimSpace(msg.ReasoningEffort),
					ToolHTTPURL:             buildACPMCPToolsURL(c, botID),
				}
			}
			admissionBuilder := func(ctx context.Context) (sessionruntime.RunAdmissionView, error) {
				if err := ctx.Err(); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				ingestedAttachments := h.ingestWSInboundAttachments(ctx, botID, chatAttachments)
				if err := ctx.Err(); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				req := buildRequest(ingestedAttachments)
				if hasSkillActivation {
					prepared, persisted, persistErr := h.resolver.ApplyUserMessageHookAndPersistUserTurn(ctx, req)
					if persistErr != nil {
						return sessionruntime.RunAdmissionView{}, persistErr
					}
					preparedReq = &prepared
					turns := conversation.ConvertMessagesToUITurns([]messagepkg.Message{persisted})
					if len(turns) > 0 {
						writer.SendJSONBounded(ctx, wsOutboundEvent{
							Type:      "user_message",
							StreamID:  streamID,
							SessionID: sessionID,
							Data:      turns[0],
						})
					}
					return sessionruntime.RunAdmissionView{}, nil
				}
				if acpInfo.IsACP {
					// ACP owns its leading-user persistence and historically bypasses
					// the in-process model runtime's user-message hook.
					preparedReq = &req
					return sessionruntime.RunAdmissionView{
						RequestUserTurn: flow.RuntimeRequestUserTurn(req, time.Now().UTC()),
					}, nil
				}
				prepared, prepareErr := h.resolver.PrepareUserMessageWS(ctx, req)
				if prepareErr != nil {
					return sessionruntime.RunAdmissionView{}, prepareErr
				}
				preparedReq = &prepared
				return sessionruntime.RunAdmissionView{
					RequestUserTurn: flow.RuntimeRequestUserTurn(prepared, time.Now().UTC()),
				}, nil
			}
			h.startWSStreamWithAdmissionBuilder(streamBaseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, "ws stream error", releaseActiveWSTurn, runtimefence.ActivationOptions{}, admissionBuilder,
				func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}, injectCh <-chan conversation.InjectMessage) error {
					if preparedReq == nil {
						return errors.New("runtime request was not prepared during admission")
					}
					req := *preparedReq
					req.InjectCh = injectCh
					return h.resolver.StreamChatWS(ctx, req, eventCh, abortCh)
				},
			)

		case "retry_message":
			sessionID := strings.TrimSpace(msg.SessionID)
			streamID := strings.TrimSpace(msg.StreamID)
			messageID := strings.TrimSpace(msg.MessageID)
			workspaceTargetID := strings.TrimSpace(msg.WorkspaceTargetID)
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if sessionID == "" {
				sendWSError(connCtx, writer, streamID, "", "session_id is required")
				continue
			}
			if messageID == "" {
				sendWSError(connCtx, writer, streamID, sessionID, "message_id is required")
				continue
			}
			if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if err := authorizeWorkspaceTargetSelection(perms, workspaceTargetID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if workspaceTargetID != "" {
				if err := h.resolver.ValidateWorkspaceTarget(streamBaseCtx, botID, workspaceTargetID); err != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
					continue
				}
			}

			prepared, err := h.resolver.PrepareRetryLatestMessageWS(c.Request().Context(), flow.RetryLatestMessageInput{
				BotID:                  botID,
				SessionID:              sessionID,
				StreamID:               streamID,
				MessageID:              messageID,
				ActorChannelIdentityID: channelIdentityID,
				ActorUserID:            channelIdentityID,
				ChatToken:              bearerToken,
				Model:                  strings.TrimSpace(msg.ModelID),
				ReasoningEffort:        strings.TrimSpace(msg.ReasoningEffort),
				WorkspaceTargetID:      workspaceTargetID,
				ToolHTTPURL:            buildACPMCPToolsURL(c, botID),
			})
			if err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			prepared, releaseReplacement, err := h.resolver.AdmitPreparedReplacementWS(c.Request().Context(), prepared)
			if err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			h.startWSStreamWithAdmissionBuilder(streamBaseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, "ws retry stream error", releaseReplacement, runtimefence.ActivationOptions{}, func(ctx context.Context) (sessionruntime.RunAdmissionView, error) {
				if err := h.resolver.ValidatePreparedReplacementWS(ctx, prepared); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				return sessionruntime.RunAdmissionView{Operation: runtimeOperationFromPreparedReplacement(prepared)}, nil
			},
				func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}, _ <-chan conversation.InjectMessage) error {
					return h.resolver.StreamPreparedReplacementWS(ctx, prepared, eventCh, abortCh)
				},
			)

		case "edit_message":
			text := strings.TrimSpace(msg.Text)
			sessionID := strings.TrimSpace(msg.SessionID)
			streamID := strings.TrimSpace(msg.StreamID)
			messageID := strings.TrimSpace(msg.MessageID)
			workspaceTargetID := strings.TrimSpace(msg.WorkspaceTargetID)
			if streamID == "" {
				sendWSError(connCtx, writer, "", sessionID, "stream_id is required")
				continue
			}
			if sessionID == "" {
				sendWSError(connCtx, writer, streamID, "", "session_id is required")
				continue
			}
			if messageID == "" {
				sendWSError(connCtx, writer, streamID, sessionID, "message_id is required")
				continue
			}
			chatAttachments, attachmentErr := parseWSClientAttachments(msg.Attachments)
			if attachmentErr != nil {
				code := slashErrorCode(attachmentErr)
				if code == "" {
					code = slash.CodeReservedSkillMetadata
				}
				sendWSCommandError(connCtx, writer, msg, code)
				continue
			}
			if text == "" && len(chatAttachments) == 0 {
				sendWSError(connCtx, writer, streamID, sessionID, "message text or attachments required")
				continue
			}
			if err := h.authorizeWSSession(c, channelIdentityID, botID, sessionID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if err := authorizeWorkspaceTargetSelection(perms, workspaceTargetID); err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			if workspaceTargetID != "" {
				if err := h.resolver.ValidateWorkspaceTarget(streamBaseCtx, botID, workspaceTargetID); err != nil {
					h.sendWSRuntimeError(connCtx, writer, streamID, sessionID, err, apperror.CodeSessionRuntimeRunFailed)
					continue
				}
			}

			prepared, err := h.resolver.PrepareEditLatestMessageWS(c.Request().Context(), flow.EditLatestMessageInput{
				BotID:                  botID,
				SessionID:              sessionID,
				StreamID:               streamID,
				MessageID:              messageID,
				Text:                   text,
				Attachments:            chatAttachments,
				ActorChannelIdentityID: channelIdentityID,
				ActorUserID:            channelIdentityID,
				ChatToken:              bearerToken,
				Model:                  strings.TrimSpace(msg.ModelID),
				ReasoningEffort:        strings.TrimSpace(msg.ReasoningEffort),
				WorkspaceTargetID:      workspaceTargetID,
				ToolHTTPURL:            buildACPMCPToolsURL(c, botID),
			})
			if err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			prepared, releaseReplacement, err := h.resolver.AdmitPreparedReplacementWS(c.Request().Context(), prepared)
			if err != nil {
				sendWSErrorFromError(connCtx, writer, streamID, sessionID, err)
				continue
			}
			h.startWSStreamWithAdmissionBuilder(streamBaseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, "ws edit stream error", releaseReplacement, runtimefence.ActivationOptions{}, func(ctx context.Context) (sessionruntime.RunAdmissionView, error) {
				if err := h.resolver.ValidatePreparedReplacementWS(ctx, prepared); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				ingestedAttachments := h.ingestWSInboundAttachments(ctx, botID, chatAttachments)
				if err := ctx.Err(); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				var attachmentErr error
				prepared, attachmentErr = prepared.WithAttachments(ingestedAttachments)
				if attachmentErr != nil {
					return sessionruntime.RunAdmissionView{}, attachmentErr
				}
				return sessionruntime.RunAdmissionView{Operation: runtimeOperationFromPreparedReplacement(prepared)}, nil
			},
				func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}, _ <-chan conversation.InjectMessage) error {
					return h.resolver.StreamPreparedReplacementWS(ctx, prepared, eventCh, abortCh)
				},
			)

		default:
			sendWSError(connCtx, writer, strings.TrimSpace(msg.StreamID), strings.TrimSpace(msg.SessionID), "unknown message type: "+msg.Type)
		}
	}
	return nil
}

func (h *LocalChannelHandler) createWSChatSession(ctx context.Context, botID, channelIdentityID string) (sessionpkg.Session, error) {
	if h == nil || h.sessionService == nil {
		return sessionpkg.Session{}, errors.New("session service not configured")
	}
	return h.sessionService.Create(ctx, sessionpkg.CreateInput{
		BotID:           strings.TrimSpace(botID),
		ChannelType:     h.channelType.String(),
		Type:            sessionpkg.TypeChat,
		CreatedByUserID: strings.TrimSpace(channelIdentityID),
	})
}

func (h *LocalChannelHandler) wsSessionSupportsRequestedSkills(ctx context.Context, sessionID string) (bool, error) {
	if h == nil || h.sessionService == nil || strings.TrimSpace(sessionID) == "" {
		return false, nil
	}
	sess, err := h.sessionService.Get(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return sessionpkg.SupportsSkillActivation(sess.SessionMode, sess.Type, sess.RuntimeType), nil
}

func (h *LocalChannelHandler) authorizeWSACPExecution(ctx context.Context, channelIdentityID, botID, sessionID string) (flow.ACPSessionExecutionInfo, error) {
	if h == nil || h.resolver == nil {
		return flow.ACPSessionExecutionInfo{}, nil
	}
	info, err := h.resolver.ACPSessionExecutionInfo(ctx, sessionID)
	if err != nil || !info.IsACP {
		return info, err
	}
	if strings.TrimSpace(info.RuntimeOwnerAccountID) == "" {
		feedback := acpRuntimeOwnerMissingFeedback()
		return info, echo.NewHTTPError(feedback.HTTPStatus, feedback)
	}
	bot, err := AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
	if err != nil {
		if isHTTPStatus(err, http.StatusForbidden) {
			feedback := acpNoWorkspaceExecFeedback("missing_workspace_exec", "You do not have permission to run workspace commands for this bot.")
			return info, echo.NewHTTPError(feedback.HTTPStatus, feedback)
		}
		return info, err
	}
	if strings.TrimSpace(info.BotID) != "" && info.BotID != bot.ID {
		return info, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	perms, err := h.resolveCurrentUserPermissions(ctx, channelIdentityID, bot.ID)
	if err != nil {
		return info, err
	}
	if err := authorizeACPRuntimeSessionAccess(channelIdentityID, perms, info.RuntimeOwnerAccountID); err != nil {
		return info, err
	}
	return info, nil
}

func wsErrorMessage(err error) string {
	const genericMessage = "The request could not be completed."
	if err == nil {
		return genericMessage
	}
	if public, ok := apperror.PublicFrom(err, ""); ok {
		return public.Detail
	}
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Code >= http.StatusInternalServerError {
			return genericMessage
		}
		switch msg := httpErr.Message.(type) {
		case interface{ Error() string }:
			return msg.Error()
		case string:
			return msg
		default:
			if msg != nil {
				return fmt.Sprint(msg)
			}
		}
	}
	return genericMessage
}

func (h *LocalChannelHandler) ensureBotParticipant(ctx context.Context, botID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	ok, err := h.chatService.IsParticipant(ctx, botID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, "bot access denied")
	}
	return nil
}

func canOpenLocalWebSocket(perms []string) bool {
	return bots.HasPermission(perms, bots.PermissionWorkspaceExec) || bots.HasPermission(perms, bots.PermissionManage)
}

func authorizeWorkspaceTargetSelection(perms []string, targetID string) error {
	if strings.TrimSpace(targetID) == "" {
		return nil
	}
	if bots.HasPermission(perms, bots.PermissionWorkspaceRead) {
		return nil
	}
	return echo.NewHTTPError(http.StatusForbidden, "workspace_read permission is required to select a computer")
}

func (*LocalChannelHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *LocalChannelHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionChat)
}

func (h *LocalChannelHandler) authorizeWSChatAccess(ctx context.Context, channelIdentityID, botID string) error {
	_, err := h.authorizeBotAccess(ctx, channelIdentityID, botID)
	return err
}

type wsSessionAuthorization struct {
	perms   []string
	session sessionpkg.Session
}

func (h *LocalChannelHandler) authorizeWSSession(c echo.Context, channelIdentityID, botID, sessionID string) error {
	_, err := h.authorizeWSSessionInfo(c.Request().Context(), channelIdentityID, botID, sessionID)
	return err
}

func (h *LocalChannelHandler) authorizeWSRuntimeSession(c echo.Context, channelIdentityID, botID, sessionID string) error {
	return h.authorizeWSRuntimeSessionContext(c.Request().Context(), channelIdentityID, botID, sessionID)
}

func (h *LocalChannelHandler) authorizeWSRuntimeSessionContext(ctx context.Context, channelIdentityID, botID, sessionID string) error {
	authz, err := h.authorizeWSSessionInfo(ctx, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	if !sessionpkg.IsACPRuntime(authz.session) {
		return nil
	}
	acpMeta := acpRuntimeSessionMetadata(authz.session)
	return authorizeACPRuntimeSessionAccess(channelIdentityID, authz.perms, sessionMetadataString(acpMeta, "runtime_owner_account_id"))
}

func (h *LocalChannelHandler) runtimeSubscriptionAuthInterval() time.Duration {
	if h != nil && h.runtimeAuthInterval > 0 {
		return h.runtimeAuthInterval
	}
	return 5 * time.Second
}

func (h *LocalChannelHandler) runtimeSubscriptionSetupTimeout() time.Duration {
	if h != nil && h.runtimeSetupTimeout > 0 {
		return h.runtimeSetupTimeout
	}
	return wsRuntimeSubscriptionSetupTimeout
}

func (h *LocalChannelHandler) authorizeWSSessionInfo(ctx context.Context, channelIdentityID, botID, sessionID string) (wsSessionAuthorization, error) {
	if h.sessionService == nil {
		return wsSessionAuthorization{}, echo.NewHTTPError(http.StatusInternalServerError, "session service not configured")
	}
	bot, perms, err := h.authorizeBotSessionAccess(ctx, channelIdentityID, botID)
	if err != nil {
		return wsSessionAuthorization{}, err
	}
	sess, err := h.sessionService.Get(ctx, sessionID)
	if err != nil || sess.BotID != bot.ID {
		return wsSessionAuthorization{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if !canAccessSession(sess, channelIdentityID, perms) {
		return wsSessionAuthorization{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return wsSessionAuthorization{perms: perms, session: sess}, nil
}

func (h *LocalChannelHandler) authorizeBotSessionAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, []string, error) {
	bot, err := AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionChat)
	if err != nil {
		bot, err = AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
		if err != nil {
			return bots.Bot{}, nil, err
		}
	}
	perms, err := h.resolveCurrentUserPermissions(ctx, channelIdentityID, bot.ID)
	if err != nil {
		return bots.Bot{}, nil, err
	}
	return bot, perms, nil
}

func (h *LocalChannelHandler) resolveCurrentUserPermissions(ctx context.Context, channelIdentityID, botID string) ([]string, error) {
	if h.botService == nil || h.accountService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	perms, err := h.botService.ResolveUserPermissions(ctx, botID, channelIdentityID, isAdmin)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return perms, nil
}

// ---------------------------------------------------------------------------
// WebSocket event processing — attachment ingestion + TTS extraction
// ---------------------------------------------------------------------------

type wsEventEnvelope struct {
	Type     string          `json:"type"`
	ToolName string          `json:"toolName,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// processWSEvent transforms a raw WS event, ingesting attachments and
// extracting TTS audio so the web frontend receives content_hash references.
func (h *LocalChannelHandler) processWSEvent(ctx context.Context, botID string, event json.RawMessage) []json.RawMessage {
	var envelope wsEventEnvelope
	if err := json.Unmarshal(event, &envelope); err != nil {
		return []json.RawMessage{event}
	}

	h.logger.Debug("ws event", slog.String("type", envelope.Type), slog.String("bot_id", botID))

	switch envelope.Type {
	case "attachment_delta":
		h.logger.Info("ws processing attachment_delta", slog.String("bot_id", botID))
		return h.wsIngestAttachments(ctx, botID, event)
	case "speech_delta":
		h.logger.Info("ws processing speech_delta", slog.String("bot_id", botID))
		return h.wsSynthesizeSpeech(ctx, botID, event)
	default:
		return []json.RawMessage{event}
	}
}

// wsIngestAttachments persists attachment data (container paths / data URLs)
// and rewrites them with content_hash so the web frontend can resolve them.
func (h *LocalChannelHandler) wsIngestAttachments(ctx context.Context, botID string, original json.RawMessage) []json.RawMessage {
	if h.mediaService == nil {
		return []json.RawMessage{original}
	}

	var event map[string]any
	if err := json.Unmarshal(original, &event); err != nil {
		return []json.RawMessage{original}
	}

	rawItems, _ := event["attachments"].([]any)
	if len(rawItems) == 0 {
		return []json.RawMessage{original}
	}

	changed := false
	for i, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		bundle := attachmentpkg.BundleFromMap(item)
		if strings.TrimSpace(bundle.ContentHash) != "" {
			continue
		}
		if bundle.Path == "" && bundle.Base64 == "" {
			continue
		}
		if ingested, ok := h.ingestSingleAttachment(ctx, botID, bundle); ok {
			rawItems[i] = applyBundleToItemMap(maps.Clone(item), ingested)
			changed = true
		}
	}

	if !changed {
		h.logger.Debug("ws attachment_delta: no items needed ingestion", slog.String("bot_id", botID))
		return []json.RawMessage{original}
	}

	h.logger.Info("ws attachment_delta: ingested attachments", slog.String("bot_id", botID), slog.Int("count", len(rawItems)))

	out, err := json.Marshal(event)
	if err != nil {
		return []json.RawMessage{original}
	}
	return []json.RawMessage{out}
}

func (h *LocalChannelHandler) ingestSingleAttachment(ctx context.Context, botID string, bundle attachmentpkg.Bundle) (attachmentpkg.Bundle, bool) {
	bundle = bundle.Normalize()
	if bundle.Path != "" {
		asset, err := h.mediaService.IngestContainerFile(ctx, botID, bundle.Path)
		if err != nil {
			h.logger.Warn("ws ingest container file failed", slog.String("path", bundle.Path), slog.Any("error", err))
			return attachmentpkg.Bundle{}, false
		}
		return bundle.WithAsset(botID, asset), true
	}

	if bundle.Base64 != "" {
		mimeType := bundle.Mime
		if mimeType == "" {
			mimeType = attachmentpkg.MimeFromDataURL(bundle.Base64)
		}
		decoded, err := attachmentpkg.DecodeBase64(bundle.Base64, media.MaxAssetBytes)
		if err != nil {
			h.logger.Warn("ws decode data url failed", slog.Any("error", err))
			return attachmentpkg.Bundle{}, false
		}
		asset, err := h.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     mimeType,
			Reader:   decoded,
			MaxBytes: media.MaxAssetBytes,
		})
		if err != nil {
			h.logger.Warn("ws ingest data url failed", slog.Any("error", err))
			return attachmentpkg.Bundle{}, false
		}
		return bundle.WithAsset(botID, asset), true
	}

	return attachmentpkg.Bundle{}, false
}

// wsSynthesizeSpeech handles speech_delta events by synthesizing audio and
// injecting attachment_delta events with the resulting voice attachments.
func (h *LocalChannelHandler) wsSynthesizeSpeech(ctx context.Context, botID string, original json.RawMessage) []json.RawMessage {
	if h.speechService == nil || h.speechModelResolver == nil {
		h.logger.Warn("speech_delta received but TTS service not configured")
		return nil
	}

	modelID, err := h.speechModelResolver.ResolveSpeechModelID(ctx, botID)
	if err != nil || strings.TrimSpace(modelID) == "" {
		h.logger.Warn("speech_delta: bot has no TTS model configured", slog.String("bot_id", botID))
		return nil
	}

	var event struct {
		Speeches []struct {
			Text string `json:"text"`
		} `json:"speeches"`
	}
	if err := json.Unmarshal(original, &event); err != nil || len(event.Speeches) == 0 {
		return nil
	}

	var results []json.RawMessage
	for _, speech := range event.Speeches {
		text := strings.TrimSpace(speech.Text)
		if text == "" {
			continue
		}

		audioData, contentType, synthErr := h.speechService.Synthesize(ctx, modelID, text, nil)
		if synthErr != nil {
			h.logger.Warn("speech synthesis failed", slog.String("bot_id", botID), slog.Any("error", synthErr))
			continue
		}

		att := h.buildTtsAttachment(ctx, botID, contentType, audioData)
		attachmentEvent, _ := json.Marshal(map[string]any{
			"type":        "attachment_delta",
			"attachments": []any{att},
		})
		results = append(results, attachmentEvent)
	}
	return results
}

func (h *LocalChannelHandler) buildTtsAttachment(ctx context.Context, botID, contentType string, audioData []byte) map[string]any {
	bundle := attachmentpkg.Bundle{
		Type: "voice",
		Mime: contentType,
		Size: int64(len(audioData)),
	}

	mimeType := attachmentpkg.NormalizeMime(contentType)
	if h.mediaService != nil {
		asset, err := h.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     mimeType,
			Reader:   bytes.NewReader(audioData),
			MaxBytes: media.MaxAssetBytes,
		})
		if err == nil {
			return bundle.WithAsset(botID, asset).ToMap()
		}
		h.logger.Warn("ws tts ingest failed", slog.Any("error", err))
	}

	bundle.Base64 = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(audioData)
	return bundle.Normalize().ToMap()
}

// extractAssetRefsFromProcessedEvent parses a processed attachment_delta
// event to collect asset refs for post-persist linking.
func extractAssetRefsFromProcessedEvent(event json.RawMessage) []messagepkg.AssetRef {
	var envelope struct {
		Type        string           `json:"type"`
		ToolCallID  string           `json:"toolCallId"`
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal(event, &envelope); err != nil || envelope.Type != "attachment_delta" {
		return nil
	}
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	var refs []messagepkg.AssetRef
	for i, att := range envelope.Attachments {
		bundle := attachmentpkg.BundleFromMap(att)
		ch := strings.TrimSpace(bundle.ContentHash)
		if ch == "" {
			continue
		}
		name := strings.TrimSpace(bundle.Name)
		if name == "" && bundle.Metadata != nil {
			name, _ = bundle.Metadata["name"].(string)
		}
		metadata := bundle.Metadata
		if toolCallID != "" {
			metadata = maps.Clone(metadata)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["tool_call_id"] = toolCallID
		}
		ref := messagepkg.AssetRef{
			ContentHash: ch,
			Role:        "attachment",
			Ordinal:     i,
			Name:        name,
			Mime:        strings.TrimSpace(bundle.Mime),
			SizeBytes:   bundle.Size,
			Metadata:    metadata,
		}
		ref.StorageKey = attachmentpkg.MetadataString(bundle.Metadata, attachmentpkg.MetadataKeyStorageKey)
		refs = append(refs, ref)
	}
	return refs
}

func applyBundleToItemMap(item map[string]any, bundle attachmentpkg.Bundle) map[string]any {
	return bundle.MergeIntoMap(item)
}

// ingestWSInboundAttachments persists inbound user attachments (data URLs or
// container paths) into the media store so each carries a content_hash. The
// gateway can inline a raw base64 payload for the model to see, but only
// attachments with a content_hash are linked to the persisted user message —
// so without this step the file shows up in the live turn yet disappears from
// history after the session refreshes. Mirrors the channel inbound path's
// ingestInboundAttachments behaviour for the WebSocket transport.
func (h *LocalChannelHandler) ingestWSInboundAttachments(ctx context.Context, botID string, attachments []conversation.ChatAttachment) []conversation.ChatAttachment {
	if len(attachments) == 0 || h.mediaService == nil || strings.TrimSpace(botID) == "" {
		return attachments
	}
	result := make([]conversation.ChatAttachment, 0, len(attachments))
	for _, att := range attachments {
		if strings.TrimSpace(att.ContentHash) != "" {
			result = append(result, att)
			continue
		}
		bundle := conversation.BundleFromChatAttachment(att)
		if strings.TrimSpace(bundle.Base64) == "" && strings.TrimSpace(bundle.Path) == "" {
			result = append(result, att)
			continue
		}
		ingested, ok := h.ingestSingleAttachment(ctx, botID, bundle)
		if !ok {
			// Keep the original so the model can still see the inlined payload
			// even if persistence failed.
			result = append(result, att)
			continue
		}
		result = append(result, conversation.ChatAttachmentFromBundle(ingested))
	}
	return result
}

func parseWSClientAttachments(rawAttachments []json.RawMessage) ([]conversation.ChatAttachment, error) {
	if len(rawAttachments) == 0 {
		return nil, nil
	}
	attachments := make([]conversation.ChatAttachment, 0, len(rawAttachments))
	for _, rawAtt := range rawAttachments {
		var decoded any
		if err := json.Unmarshal(rawAtt, &decoded); err != nil {
			continue
		}
		bundles, ok := attachmentpkg.ParseToolInputBundles(decoded)
		if !ok {
			continue
		}
		for _, bundle := range bundles {
			attachment := conversation.ChatAttachmentFromBundle(bundle)
			if err := slash.RejectReservedSkillMetadataValue(attachment.Metadata); err != nil {
				return nil, err
			}
			attachments = append(attachments, attachment)
		}
	}
	return attachments, nil
}
