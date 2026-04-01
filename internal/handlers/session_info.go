package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

type SessionInfoHandler struct {
	queries         *sqlc.Queries
	botService      *bots.Service
	accountService  *accounts.Service
	modelsService   *models.Service
	settingsService *settings.Service
	logger          *slog.Logger
}

func NewSessionInfoHandler(log *slog.Logger, queries *sqlc.Queries, botService *bots.Service, accountService *accounts.Service, modelsService *models.Service, settingsService *settings.Service) *SessionInfoHandler {
	return &SessionInfoHandler{
		queries:         queries,
		botService:      botService,
		accountService:  accountService,
		modelsService:   modelsService,
		settingsService: settingsService,
		logger:          log.With(slog.String("handler", "session_info")),
	}
}

func (h *SessionInfoHandler) Register(e *echo.Echo) {
	e.GET("/bots/:bot_id/sessions/:session_id/info", h.GetSessionInfo)
}

type SessionInfoResponse struct {
	MessageCount int64        `json:"message_count"`
	ContextUsage ContextUsage `json:"context_usage"`
	CacheStats   CacheStats   `json:"cache_stats"`
	Skills       []string     `json:"skills"`
}

type ContextUsage struct {
	UsedTokens    int64  `json:"used_tokens"`
	ContextWindow *int64 `json:"context_window,omitempty"`
}

type CacheStats struct {
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalInputTokens int64   `json:"total_input_tokens"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
}

// GetSessionInfo godoc
// @Summary Get session info
// @Description Get aggregated info for a chat session including message count, context usage, cache stats, and used skills
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Param model_id query string false "Optional model UUID override for context window"
// @Success 200 {object} SessionInfoResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/info [get].
func (h *SessionInfoHandler) GetSessionInfo(c echo.Context) error {
	userID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}

	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid session id")
	}

	ctx := c.Request().Context()

	messageCount, err := h.queries.CountMessagesBySession(ctx, pgSessionID)
	if err != nil {
		h.logger.Error("count messages failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count messages")
	}

	var usedTokens int64
	latestUsage, err := h.queries.GetLatestAssistantUsage(ctx, pgSessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.logger.Error("get latest usage failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get latest usage")
	}
	if err == nil {
		usedTokens = latestUsage
	}

	contextWindow := h.resolveContextWindow(c, botID)

	cacheRow, err := h.queries.GetSessionCacheStats(ctx, pgSessionID)
	if err != nil {
		h.logger.Error("get cache stats failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get cache stats")
	}

	var cacheHitRate float64
	if cacheRow.TotalInputTokens > 0 {
		cacheHitRate = float64(cacheRow.CacheReadTokens) / float64(cacheRow.TotalInputTokens) * 100
	}

	skills, err := h.queries.GetSessionUsedSkills(ctx, pgSessionID)
	if err != nil {
		h.logger.Error("get used skills failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get used skills")
	}
	if skills == nil {
		skills = []string{}
	}

	resp := SessionInfoResponse{
		MessageCount: messageCount,
		ContextUsage: ContextUsage{
			UsedTokens:    usedTokens,
			ContextWindow: contextWindow,
		},
		CacheStats: CacheStats{
			CacheReadTokens:  cacheRow.CacheReadTokens,
			CacheWriteTokens: cacheRow.CacheWriteTokens,
			TotalInputTokens: cacheRow.TotalInputTokens,
			CacheHitRate:     cacheHitRate,
		},
		Skills: skills,
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *SessionInfoHandler) resolveContextWindow(c echo.Context, botID string) *int64 {
	modelIDStr := strings.TrimSpace(c.QueryParam("model_id"))

	if modelIDStr == "" && h.settingsService != nil {
		s, err := h.settingsService.GetBot(c.Request().Context(), botID)
		if err == nil && s.ChatModelID != "" {
			modelIDStr = s.ChatModelID
		}
	}

	if modelIDStr == "" || h.modelsService == nil {
		return nil
	}

	m, err := h.modelsService.GetByID(c.Request().Context(), modelIDStr)
	if err != nil {
		return nil
	}
	if m.Config.ContextWindow == nil {
		return nil
	}
	cw := int64(*m.Config.ContextWindow)
	return &cw
}
