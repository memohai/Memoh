package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type TokenUsageHandler struct {
	queries        *sqlc.Queries
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewTokenUsageHandler(log *slog.Logger, queries *sqlc.Queries, botService *bots.Service, accountService *accounts.Service) *TokenUsageHandler {
	return &TokenUsageHandler{
		queries:        queries,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "token_usage")),
	}
}

func (h *TokenUsageHandler) Register(e *echo.Echo) {
	e.GET("/bots/:bot_id/token-usage", h.GetTokenUsage)
}

// DailyTokenUsage represents aggregated token usage for a single day.
type DailyTokenUsage struct {
	Day              string `json:"day"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
}

// ModelTokenUsage represents aggregated token usage for a single model.
type ModelTokenUsage struct {
	ModelID      string `json:"model_id"`
	ModelSlug    string `json:"model_slug"`
	ModelName    string `json:"model_name"`
	ProviderName string `json:"provider_name"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// TokenUsageResponse is the response body for GET /bots/:bot_id/token-usage.
type TokenUsageResponse struct {
	Chat      []DailyTokenUsage `json:"chat"`
	Heartbeat []DailyTokenUsage `json:"heartbeat"`
	Schedule  []DailyTokenUsage `json:"schedule"`
	ByModel   []ModelTokenUsage `json:"by_model"`
}

// GetTokenUsage godoc
// @Summary Get token usage statistics
// @Description Get daily aggregated token usage for a bot, split by chat, heartbeat, and schedule session types, with optional model filter and per-model breakdown
// @Tags token-usage
// @Param bot_id path string true "Bot ID"
// @Param from query string true "Start date (YYYY-MM-DD)"
// @Param to query string true "End date exclusive (YYYY-MM-DD)"
// @Param model_id query string false "Optional model UUID to filter by"
// @Success 200 {object} TokenUsageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/token-usage [get].
func (h *TokenUsageHandler) GetTokenUsage(c echo.Context) error {
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

	fromStr := strings.TrimSpace(c.QueryParam("from"))
	toStr := strings.TrimSpace(c.QueryParam("to"))
	if fromStr == "" || toStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "from and to query parameters are required (YYYY-MM-DD)")
	}
	fromDate, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid from date format, expected YYYY-MM-DD")
	}
	toDate, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid to date format, expected YYYY-MM-DD")
	}
	if !toDate.After(fromDate) {
		return echo.NewHTTPError(http.StatusBadRequest, "to must be after from")
	}

	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid bot id")
	}

	var pgModelID pgtype.UUID
	if modelIDStr := strings.TrimSpace(c.QueryParam("model_id")); modelIDStr != "" {
		pgModelID, err = db.ParseUUID(modelIDStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid model_id")
		}
	}

	fromTS := pgtype.Timestamptz{Time: fromDate, Valid: true}
	toTS := pgtype.Timestamptz{Time: toDate, Valid: true}

	ctx := c.Request().Context()

	chat, heartbeat, schedule, err := h.fetchUsageByDay(ctx, pgBotID, fromTS, toTS, pgModelID)
	if err != nil {
		h.logger.Error("fetch token usage failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch token usage")
	}

	byModel, err := h.fetchUsageByModel(ctx, pgBotID, fromTS, toTS)
	if err != nil {
		h.logger.Error("fetch token usage by model failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to fetch token usage by model")
	}

	resp := TokenUsageResponse{
		Chat:      chat,
		Heartbeat: heartbeat,
		Schedule:  schedule,
		ByModel:   byModel,
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *TokenUsageHandler) fetchUsageByDay(ctx context.Context, botID pgtype.UUID, from, to pgtype.Timestamptz, modelID pgtype.UUID) (chat, heartbeat, schedule []DailyTokenUsage, err error) {
	rows, err := h.queries.GetTokenUsageByDayAndType(ctx, sqlc.GetTokenUsageByDayAndTypeParams{
		BotID:    botID,
		FromTime: from,
		ToTime:   to,
		ModelID:  modelID,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	for _, r := range rows {
		d := DailyTokenUsage{
			Day:              formatPgDate(r.Day),
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheReadTokens:  r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens,
			ReasoningTokens:  r.ReasoningTokens,
		}
		switch r.SessionType {
		case "heartbeat":
			heartbeat = append(heartbeat, d)
		case "schedule":
			schedule = append(schedule, d)
		default:
			chat = append(chat, d)
		}
	}
	return chat, heartbeat, schedule, nil
}

func (h *TokenUsageHandler) fetchUsageByModel(ctx context.Context, botID pgtype.UUID, from, to pgtype.Timestamptz) ([]ModelTokenUsage, error) {
	rows, err := h.queries.GetTokenUsageByModel(ctx, sqlc.GetTokenUsageByModelParams{
		BotID:    botID,
		FromTime: from,
		ToTime:   to,
	})
	if err != nil {
		return nil, err
	}

	result := make([]ModelTokenUsage, 0, len(rows))
	for _, r := range rows {
		result = append(result, ModelTokenUsage{
			ModelID:      formatOptionalUUID(r.ModelID),
			ModelSlug:    r.ModelSlug,
			ModelName:    r.ModelName,
			ProviderName: r.ProviderName,
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
		})
	}
	return result, nil
}

func formatPgDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func formatOptionalUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}
