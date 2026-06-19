package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/session"
)

// SessionHandler handles bot session CRUD endpoints.
type SessionHandler struct {
	sessionService *session.Service
	acpPool        acpSessionCloser
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

type acpSessionCloser interface {
	CloseSession(sessionID string) error
	BindRuntime(botID, runtimeID, sessionID, agentID, projectPath string) error
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(log *slog.Logger, sessionService *session.Service, acpPool acpSessionCloser, botService *bots.Service, accountService *accounts.Service) *SessionHandler {
	return &SessionHandler{
		sessionService: sessionService,
		acpPool:        acpPool,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "session")),
	}
}

// Register registers session routes.
func (h *SessionHandler) Register(e *echo.Echo) {
	g := e.Group("/bots/:bot_id/sessions")
	g.POST("", h.CreateSession)
	g.GET("", h.ListSessions)
	g.GET("/:session_id", h.GetSession)
	g.PATCH("/:session_id", h.UpdateSession)
	g.DELETE("/:session_id", h.DeleteSession)
}

type createSessionRequest struct {
	Type        string         `json:"type,omitempty"`
	Title       string         `json:"title"`
	ChannelType string         `json:"channel_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	// ACPRuntimeID optionally binds a warm pre-session runtime (created via
	// POST /bots/{bot_id}/acp-runtimes) to the new ACP session. It is a
	// transient in-memory handle reference, never persisted in metadata.
	ACPRuntimeID string `json:"acp_runtime_id,omitempty"`
}

type updateSessionRequest struct {
	Title    *string        `json:"title,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateSession godoc
// @Summary Create a new chat session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param body body createSessionRequest true "Session data"
// @Success 201 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions [post].
func (h *SessionHandler) CreateSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	var req createSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sessionType := strings.TrimSpace(req.Type)
	if sessionType == "" {
		sessionType = session.TypeChat
	}
	if !session.IsKnownType(sessionType) {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown session type")
	}
	bot, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, requiredPermissionForSessionType(sessionType))
	if err != nil {
		return err
	}
	if sessionType == session.TypeACPAgent {
		req.Metadata = session.ApplyACPMetadataDefaults(req.Metadata)
		if err := validateACPCreate(bot, req.Metadata); err != nil {
			return err
		}
	}
	sess, err := h.sessionService.Create(c.Request().Context(), session.CreateInput{
		BotID:           bot.ID,
		ChannelType:     req.ChannelType,
		Type:            sessionType,
		Title:           req.Title,
		Metadata:        req.Metadata,
		CreatedByUserID: channelIdentityID,
	})
	if err != nil {
		return sessionServiceError(err)
	}
	// Best-effort bind of a warm pre-session runtime: the session lives in
	// the database and the runtime in memory, so this is sequenced (bind only
	// after a successful create), not transactional. A failed bind keeps the
	// session — the first prompt simply cold starts a runtime.
	if runtimeID := strings.TrimSpace(req.ACPRuntimeID); runtimeID != "" && sessionType == session.TypeACPAgent && h.acpPool != nil {
		if bindErr := h.acpPool.BindRuntime(
			bot.ID,
			runtimeID,
			sess.ID,
			sessionMetadataString(sess.Metadata, "acp_agent_id"),
			sessionMetadataString(sess.Metadata, "project_path"),
		); bindErr != nil {
			h.logger.Warn("failed to bind ACP runtime to new session; first prompt will cold start",
				slog.String("session_id", sess.ID),
				slog.String("runtime_id", runtimeID),
				slog.Any("error", bindErr),
			)
		}
	}
	return c.JSON(http.StatusCreated, sess)
}

// ListSessions godoc
// @Summary List bot sessions
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param types query string false "Comma-separated session types to include. Defaults to user-facing types (chat,discuss,acp_agent)."
// @Param limit query int false "Page size (1..200). Defaults to 50."
// @Param cursor query string false "Opaque cursor returned as next_cursor on a previous page."
// @Success 200 {object} listSessionsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions [get].
func (h *SessionHandler) ListSessions(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	bot, perms, err := h.authorizeBotSessionAccess(c, channelIdentityID, botID)
	if err != nil {
		return err
	}
	types, err := parseSessionTypesParam(c.QueryParam("types"))
	if err != nil {
		return err
	}
	limit, err := parseSessionLimitParam(c.QueryParam("limit"))
	if err != nil {
		return err
	}
	cursor, err := decodeSessionCursor(c.QueryParam("cursor"))
	if err != nil {
		return err
	}

	var (
		sessions     []session.Session
		nextCursor   session.SessionCursor
		hasMorePages bool
	)
	if bots.HasPermission(perms, bots.PermissionManage) {
		sessions, err = h.sessionService.ListByBotPaged(c.Request().Context(), bot.ID, types, cursor, limit)
		if err == nil && len(sessions) == int(limit) {
			hasMorePages = true
			last := sessions[len(sessions)-1]
			nextCursor = session.SessionCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
		}
	} else {
		var preFilter []session.Session
		preFilter, err = h.sessionService.ListByBotAndCreatedByUserPaged(c.Request().Context(), bot.ID, channelIdentityID, types, cursor, limit)
		if err == nil {
			// next_cursor must reflect the DB position to resume from, not the
			// filter survivorship — otherwise a page whose rows were all
			// dropped by the permission filter would terminate pagination
			// while older accessible rows still exist on disk.
			if len(preFilter) == int(limit) {
				hasMorePages = true
				last := preFilter[len(preFilter)-1]
				nextCursor = session.SessionCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
			}
			sessions = filterSessionsForPermissions(preFilter, channelIdentityID, perms)
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	encoded := ""
	if hasMorePages {
		encoded = encodeSessionCursor(nextCursor)
	}
	return c.JSON(http.StatusOK, listSessionsResponse{Items: sessions, NextCursor: encoded})
}

// listSessionsResponse is the shape returned by ListSessions.
type listSessionsResponse struct {
	Items      []session.Session `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

const (
	sessionListDefaultLimit = 50
	sessionListMaxLimit     = 200
)

func parseSessionTypesParam(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		out := make([]string, len(session.UserFacingSessionTypes))
		copy(out, session.UserFacingSessionTypes)
		return out, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if !session.IsKnownType(token) {
			return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unknown session type %q", token))
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	if len(out) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "types must contain at least one session type")
	}
	return out, nil
}

func parseSessionLimitParam(raw string) (int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sessionListDefaultLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "limit must be an integer")
	}
	if value < 1 || value > sessionListMaxLimit {
		return 0, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("limit must be between 1 and %d", sessionListMaxLimit))
	}
	//nolint:gosec // bounded above by sessionListMaxLimit (200), safe for int32.
	return int32(value), nil
}

// encodeSessionCursor packs the keyset cursor as base64(updated_at|id) where
// the timestamp is RFC3339Nano. The full nanosecond precision is preserved on
// the wire and on Postgres, but the SQLite backend compares updated_at at
// second precision because CURRENT_TIMESTAMP stores it that way — that
// truncation is intentional and only matters when two rows share the same
// second, where the id tiebreak in the SQL handles uniqueness.
func encodeSessionCursor(c session.SessionCursor) string {
	raw := c.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeSessionCursor(raw string) (session.SessionCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return session.SessionCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return session.SessionCursor{}, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return session.SessionCursor{}, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return session.SessionCursor{}, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor")
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return session.SessionCursor{}, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor")
	}
	return session.SessionCursor{UpdatedAt: updatedAt, ID: parts[1]}, nil
}

// GetSession godoc
// @Summary Get a session by ID
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [get].
func (h *SessionHandler) GetSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	_, _, sess, err := h.authorizeSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, sess)
}

// UpdateSession godoc
// @Summary Update a session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Param body body updateSessionRequest true "Fields to update"
// @Success 200 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [patch].
func (h *SessionHandler) UpdateSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}

	bot, perms, existing, err := h.authorizeSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}

	var req updateSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result := existing
	if req.Type != nil || req.Metadata != nil {
		targetType := existing.Type
		if req.Type != nil {
			targetType = strings.TrimSpace(*req.Type)
			if targetType == "" {
				targetType = session.TypeChat
			}
		}
		if !session.IsKnownType(targetType) {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown session type")
		}
		if !bots.HasPermission(perms, requiredPermissionForSessionType(targetType)) {
			return echo.NewHTTPError(http.StatusForbidden, "bot access denied")
		}
		targetMetadata := cloneSessionMetadata(existing.Metadata)
		if req.Metadata != nil {
			targetMetadata = cloneSessionMetadata(req.Metadata)
		}
		if targetType == session.TypeACPAgent {
			targetMetadata = session.ApplyACPMetadataDefaults(targetMetadata)
		}
		agentChanged := sessionAgentConfigChanged(existing.Type, existing.Metadata, targetType, targetMetadata)
		if agentChanged {
			count, err := h.sessionService.MessageCount(c.Request().Context(), sessionID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if count > 0 {
				return echo.NewHTTPError(http.StatusConflict, "session agent cannot be changed after messages are sent")
			}
		}
		if targetType == session.TypeACPAgent {
			if err := validateACPCreate(bot, targetMetadata); err != nil {
				return err
			}
		} else if existing.Type == session.TypeACPAgent || req.Type != nil {
			targetMetadata = stripACPMetadata(targetMetadata)
		}
		if targetType != existing.Type || req.Metadata != nil {
			result, err = h.sessionService.UpdateTypeAndMetadata(c.Request().Context(), sessionID, targetType, targetMetadata)
			if err != nil {
				return sessionServiceError(err)
			}
			if agentChanged && existing.Type == session.TypeACPAgent && h.acpPool != nil {
				if closeErr := h.acpPool.CloseSession(sessionID); closeErr != nil {
					h.logger.Warn("failed to close ACP runtime after session update", slog.String("session_id", sessionID), slog.Any("error", closeErr))
				}
			}
		}
	}
	if req.Title != nil {
		result, err = h.sessionService.UpdateTitle(c.Request().Context(), sessionID, *req.Title)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if req.Title == nil && req.Metadata == nil && req.Type == nil {
		result = existing
	}
	return c.JSON(http.StatusOK, result)
}

// DeleteSession godoc
// @Summary Soft-delete a session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [delete].
func (h *SessionHandler) DeleteSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	_, _, existing, err := h.authorizeSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	if existing.Type == session.TypeACPAgent && h.acpPool != nil {
		if closeErr := h.acpPool.CloseSession(sessionID); closeErr != nil {
			h.logger.Warn("failed to close ACP runtime before session delete", slog.String("session_id", sessionID), slog.Any("error", closeErr))
		}
	}
	if err := h.sessionService.SoftDelete(c.Request().Context(), sessionID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SessionHandler) authorizeBotSessionAccess(c echo.Context, channelIdentityID, botID string) (bots.Bot, []string, error) {
	bot, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionChat)
	if err != nil {
		bot, err = AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
		if err != nil {
			return bots.Bot{}, nil, err
		}
	}
	perms, err := h.resolveCurrentUserPermissions(c, channelIdentityID, bot.ID)
	if err != nil {
		return bots.Bot{}, nil, err
	}
	return bot, perms, nil
}

func (h *SessionHandler) authorizeSession(c echo.Context, channelIdentityID, botID, sessionID string) (bots.Bot, []string, session.Session, error) {
	bot, perms, err := h.authorizeBotSessionAccess(c, channelIdentityID, botID)
	if err != nil {
		return bots.Bot{}, nil, session.Session{}, err
	}
	sess, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil || sess.BotID != bot.ID {
		return bots.Bot{}, nil, session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if !canAccessSession(sess, channelIdentityID, perms) {
		return bots.Bot{}, nil, session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return bot, perms, sess, nil
}

func (h *SessionHandler) resolveCurrentUserPermissions(c echo.Context, channelIdentityID, botID string) ([]string, error) {
	if h.botService == nil || h.accountService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(c.Request().Context(), channelIdentityID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	perms, err := h.botService.ResolveUserPermissions(c.Request().Context(), botID, channelIdentityID, isAdmin)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return perms, nil
}

func requiredPermissionForSessionType(sessionType string) string {
	switch strings.TrimSpace(sessionType) {
	case session.TypeChat:
		return bots.PermissionChat
	case session.TypeACPAgent:
		return bots.PermissionWorkspaceExec
	default:
		return bots.PermissionManage
	}
}

func canAccessSession(sess session.Session, userID string, perms []string) bool {
	if bots.HasPermission(perms, bots.PermissionManage) {
		return true
	}
	if strings.TrimSpace(sess.CreatedByUserID) == "" || sess.CreatedByUserID != strings.TrimSpace(userID) {
		return false
	}
	return bots.HasPermission(perms, requiredPermissionForSessionType(sess.Type))
}

func filterSessionsForPermissions(items []session.Session, userID string, perms []string) []session.Session {
	if bots.HasPermission(perms, bots.PermissionManage) {
		return items
	}
	out := make([]session.Session, 0, len(items))
	for _, item := range items {
		if canAccessSession(item, userID, perms) {
			out = append(out, item)
		}
	}
	return out
}

func validateACPCreate(bot bots.Bot, metadata map[string]any) error {
	agentID := sessionMetadataString(metadata, "acp_agent_id")
	if agentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, session.ErrACPAgentIDRequired.Error())
	}
	if sessionMetadataString(metadata, "project_path") == "" {
		return echo.NewHTTPError(http.StatusBadRequest, session.ErrACPProjectPathMissing.Error())
	}
	if _, ok := acpprofile.Lookup(agentID); !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown ACP agent")
	}
	if !acpprofile.MetadataAgentEnabled(bot.Metadata, agentID) {
		return echo.NewHTTPError(http.StatusForbidden, "ACP agent is not enabled for this bot")
	}
	return nil
}

func sessionServiceError(err error) error {
	switch {
	case errors.Is(err, session.ErrACPAgentIDRequired),
		errors.Is(err, session.ErrACPProjectPathMissing),
		errors.Is(err, session.ErrACPUnknownAgent):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrACPAgentNotEnabled):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}

func sessionAgentConfigChanged(existingType string, existingMetadata map[string]any, targetType string, targetMetadata map[string]any) bool {
	if strings.TrimSpace(existingType) != strings.TrimSpace(targetType) {
		return true
	}
	if targetType != session.TypeACPAgent {
		return false
	}
	for _, key := range []string{"acp_agent_id", "project_path", "acp_project_mode"} {
		if sessionMetadataString(existingMetadata, key) != sessionMetadataString(targetMetadata, key) {
			return true
		}
	}
	return false
}

func stripACPMetadata(metadata map[string]any) map[string]any {
	out := cloneSessionMetadata(metadata)
	for key := range out {
		if strings.HasPrefix(key, "acp_") || key == "project_path" {
			delete(out, key)
		}
	}
	return out
}

func cloneSessionMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func sessionMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
