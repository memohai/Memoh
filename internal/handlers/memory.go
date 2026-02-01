package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/memory"
)

type MemoryHandler struct {
	service *memory.Service
	logger  *slog.Logger
}

func NewMemoryHandler(log *slog.Logger, service *memory.Service) *MemoryHandler {
	return &MemoryHandler{
		service: service,
		logger:  log.With(slog.String("handler", "memory")),
	}
}

func (h *MemoryHandler) Register(e *echo.Echo) {
	group := e.Group("/memory")
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
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory service not available: no embedding models configured")
	}
	return nil
}

// EmbedUpsert godoc
// @Summary Embed and upsert memory
// @Description Embed text or multimodal input and upsert into memory store
// @Tags memory
// @Param payload body memory.EmbedUpsertRequest true "Embed upsert request"
// @Success 200 {object} memory.EmbedUpsertResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/embed [post]
func (h *MemoryHandler) EmbedUpsert(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req memory.EmbedUpsertRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.UserID != "" && req.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	req.UserID = userID

	resp, err := h.service.EmbedUpsert(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Add godoc
// @Summary Add memory
// @Description Add memory for a user via memory
// @Tags memory
// @Param payload body memory.AddRequest true "Add request"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/add [post]
func (h *MemoryHandler) Add(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req memory.AddRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.UserID != "" && req.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	req.UserID = userID

	resp, err := h.service.Add(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Search godoc
// @Summary Search memories
// @Description Search memories for a user via memory
// @Tags memory
// @Param payload body memory.SearchRequest true "Search request"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/search [post]
func (h *MemoryHandler) Search(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req memory.SearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.UserID != "" && req.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	req.UserID = userID

	resp, err := h.service.Search(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update memory
// @Description Update a memory by ID via memory
// @Tags memory
// @Param payload body memory.UpdateRequest true "Update request"
// @Success 200 {object} memory.MemoryItem
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/update [post]
func (h *MemoryHandler) Update(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
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
		if existing.UserID != "" && existing.UserID != userID {
			return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
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
// @Description Get a memory by ID via memory
// @Tags memory
// @Param memoryId path string true "Memory ID"
// @Success 200 {object} memory.MemoryItem
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/memories/{memoryId} [get]
func (h *MemoryHandler) Get(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
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
	if resp.UserID != "" && resp.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	return c.JSON(http.StatusOK, resp)
}

// GetAll godoc
// @Summary List memories
// @Description List memories for a user via memory
// @Tags memory
// @Param user_id query string false "User ID"
// @Param agent_id query string false "Agent ID"
// @Param run_id query string false "Run ID"
// @Param limit query int false "Limit"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/memories [get]
func (h *MemoryHandler) GetAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	if queryUserID := c.QueryParam("user_id"); queryUserID != "" && queryUserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	req := memory.GetAllRequest{
		UserID:  userID,
		AgentID: c.QueryParam("agent_id"),
		RunID:   c.QueryParam("run_id"),
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
// @Description Delete a memory by ID via memory
// @Tags memory
// @Param memoryId path string true "Memory ID"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/memories/{memoryId} [delete]
func (h *MemoryHandler) Delete(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
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
	if existing.UserID != "" && existing.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}

	resp, err := h.service.Delete(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteAll godoc
// @Summary Delete memories
// @Description Delete all memories for a user via memory
// @Tags memory
// @Param payload body memory.DeleteAllRequest true "Delete all request"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /memory/memories [delete]
func (h *MemoryHandler) DeleteAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}

	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req memory.DeleteAllRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.UserID != "" && req.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	req.UserID = userID

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
