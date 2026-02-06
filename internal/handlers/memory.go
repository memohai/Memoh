package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/users"
)

type MemoryHandler struct {
	service     *memory.Service
	botService  *bots.Service
	userService *users.Service
	logger      *slog.Logger
}

type memoryAddPayload struct {
	Message          string           `json:"message,omitempty"`
	Messages         []memory.Message `json:"messages,omitempty"`
	RunID            string           `json:"run_id,omitempty"`
	Metadata         map[string]any   `json:"metadata,omitempty"`
	Filters          map[string]any   `json:"filters,omitempty"`
	Infer            *bool            `json:"infer,omitempty"`
	EmbeddingEnabled *bool            `json:"embedding_enabled,omitempty"`
}

type memorySearchPayload struct {
	Query            string         `json:"query"`
	RunID            string         `json:"run_id,omitempty"`
	Limit            int            `json:"limit,omitempty"`
	Filters          map[string]any `json:"filters,omitempty"`
	Sources          []string       `json:"sources,omitempty"`
	EmbeddingEnabled *bool          `json:"embedding_enabled,omitempty"`
}

type memoryEmbedUpsertPayload struct {
	Type     string            `json:"type"`
	Provider string            `json:"provider,omitempty"`
	Model    string            `json:"model,omitempty"`
	Input    memory.EmbedInput `json:"input"`
	Source   string            `json:"source,omitempty"`
	RunID    string            `json:"run_id,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Filters  map[string]any    `json:"filters,omitempty"`
}

type memoryDeleteAllPayload struct {
	RunID string `json:"run_id,omitempty"`
}

func NewMemoryHandler(log *slog.Logger, service *memory.Service, botService *bots.Service, userService *users.Service) *MemoryHandler {
	return &MemoryHandler{
		service:     service,
		botService:  botService,
		userService: userService,
		logger:      log.With(slog.String("handler", "memory")),
	}
}

func (h *MemoryHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/memory")
	group.POST("/add", h.Add)
	group.POST("/embed", h.EmbedUpsert)
	group.POST("/search", h.Search)
	group.POST("/update", h.Update)
	group.GET("/memories/:memoryId", h.Get)
	group.GET("/memories", h.GetAll)
	group.DELETE("/memories/:memoryId", h.Delete)
	group.DELETE("/memories", h.DeleteAll)
}

func (h *MemoryHandler) checkService() error {
	if h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory service not available")
	}
	return nil
}

// EmbedUpsert godoc
// @Summary Embed and upsert memory
// @Description Embed text or multimodal input and upsert into memory store. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param payload body memoryEmbedUpsertPayload true "Embed upsert request"
// @Success 200 {object} memory.EmbedUpsertResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/embed [post]
func (h *MemoryHandler) EmbedUpsert(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}

	var payload memoryEmbedUpsertPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := memory.EmbedUpsertRequest{
		Type:      payload.Type,
		Provider:  payload.Provider,
		Model:     payload.Model,
		Input:     payload.Input,
		Source:    payload.Source,
		BotID:     botID,
		SessionID: sessionID,
		RunID:     payload.RunID,
		Metadata:  payload.Metadata,
		Filters:   payload.Filters,
	}

	resp, err := h.service.EmbedUpsert(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Add godoc
// @Summary Add memory
// @Description Add memory for a user via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param payload body memoryAddPayload true "Add request"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/add [post]
func (h *MemoryHandler) Add(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}

	var payload memoryAddPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := memory.AddRequest{
		Message:          payload.Message,
		Messages:         payload.Messages,
		BotID:            botID,
		SessionID:        sessionID,
		RunID:            payload.RunID,
		Metadata:         payload.Metadata,
		Filters:          payload.Filters,
		Infer:            payload.Infer,
		EmbeddingEnabled: payload.EmbeddingEnabled,
	}

	resp, err := h.service.Add(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Search godoc
// @Summary Search memories
// @Description Search memories for a user via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param payload body memorySearchPayload true "Search request"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/search [post]
func (h *MemoryHandler) Search(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}

	var payload memorySearchPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := memory.SearchRequest{
		Query:            payload.Query,
		BotID:            botID,
		SessionID:        sessionID,
		RunID:            payload.RunID,
		Limit:            payload.Limit,
		Filters:          payload.Filters,
		Sources:          payload.Sources,
		EmbeddingEnabled: payload.EmbeddingEnabled,
	}

	resp, err := h.service.Search(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update memory
// @Description Update a memory by ID via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param payload body memory.UpdateRequest true "Update request"
// @Success 200 {object} memory.MemoryItem
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/update [post]
func (h *MemoryHandler) Update(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	var req memory.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.MemoryID != "" {
		existing, err := h.service.Get(c.Request().Context(), req.MemoryID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if existing.BotID != "" && existing.BotID != botID {
			return echo.NewHTTPError(http.StatusForbidden, "bot mismatch")
		}
	}

	resp, err := h.service.Update(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Get godoc
// @Summary Get memory
// @Description Get a memory by ID via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param memoryId path string true "Memory ID"
// @Success 200 {object} memory.MemoryItem
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/memories/{memoryId} [get]
func (h *MemoryHandler) Get(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	memoryID := c.Param("memoryId")
	if memoryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "memory ID required")
	}

	resp, err := h.service.Get(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if resp.BotID != "" && resp.BotID != botID {
		return echo.NewHTTPError(http.StatusForbidden, "bot mismatch")
	}
	return c.JSON(http.StatusOK, resp)
}

// GetAll godoc
// @Summary List memories
// @Description List memories for a user via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param run_id query string false "Run ID"
// @Param limit query int false "Limit"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/memories [get]
func (h *MemoryHandler) GetAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}

	req := memory.GetAllRequest{
		BotID:     botID,
		SessionID: sessionID,
		AgentID:   c.QueryParam("agent_id"),
		RunID:     c.QueryParam("run_id"),
	}
	if limit := c.QueryParam("limit"); limit != "" {
		var parsed int
		if _, err := fmt.Sscanf(limit, "%d", &parsed); err == nil {
			req.Limit = parsed
		}
	}

	resp, err := h.service.GetAll(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete memory
// @Description Delete a memory by ID via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param memoryId path string true "Memory ID"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/memories/{memoryId} [delete]
func (h *MemoryHandler) Delete(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	memoryID := c.Param("memoryId")
	if memoryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "memory ID required")
	}

	existing, err := h.service.Get(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if existing.BotID != "" && existing.BotID != botID {
		return echo.NewHTTPError(http.StatusForbidden, "bot mismatch")
	}

	resp, err := h.service.Delete(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteAll godoc
// @Summary Delete memories
// @Description Delete all memories for a user via memory. Auth: Bearer JWT determines user_id (sub or user_id).
// @Tags memory
// @Param payload body memoryDeleteAllPayload true "Delete all request"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/memories [delete]
func (h *MemoryHandler) DeleteAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}

	var payload memoryDeleteAllPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := memory.DeleteAllRequest{
		BotID:     botID,
		SessionID: sessionID,
		RunID:     payload.RunID,
	}

	resp, err := h.service.DeleteAll(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *MemoryHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

func (h *MemoryHandler) authorizeBotAccess(ctx context.Context, actorID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.userService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.userService.IsAdmin(ctx, actorID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, actorID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: false})
	if err != nil {
		if errors.Is(err, bots.ErrBotNotFound) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		if errors.Is(err, bots.ErrBotAccessDenied) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusForbidden, "bot access denied")
		}
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return bot, nil
}
