package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/history"
	"github.com/memohai/memoh/internal/identity"
)

type HistoryHandler struct {
	service *history.Service
	logger  *slog.Logger
}

func NewHistoryHandler(log *slog.Logger, service *history.Service) *HistoryHandler {
	return &HistoryHandler{
		service: service,
		logger:  log,
	}
}

func (h *HistoryHandler) Register(e *echo.Echo) {
	group := e.Group("/history")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.DELETE("/:id", h.Delete)
	group.DELETE("", h.DeleteAll)
}

// Create godoc
// @Summary Create history record
// @Description Create a history record for current user
// @Tags history
// @Param payload body history.CreateRequest true "History payload"
// @Success 201 {object} history.Record
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /history [post]
func (h *HistoryHandler) Create(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req history.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Create(c.Request().Context(), userID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// Get godoc
// @Summary Get history record
// @Description Get a history record by ID (must belong to current user)
// @Tags history
// @Param id path string true "History ID"
// @Success 200 {object} history.Record
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /history/{id} [get]
func (h *HistoryHandler) Get(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	record, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if record.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	return c.JSON(http.StatusOK, record)
}

// List godoc
// @Summary List history records
// @Description List history records for current user
// @Tags history
// @Param limit query int false "Limit"
// @Success 200 {object} history.ListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /history [get]
func (h *HistoryHandler) List(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	limit := 0
	if raw := c.QueryParam("limit"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &limit); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit")
		}
	}
	items, err := h.service.List(c.Request().Context(), userID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, history.ListResponse{Items: items})
}

// Delete godoc
// @Summary Delete history record
// @Description Delete a history record by ID (must belong to current user)
// @Tags history
// @Param id path string true "History ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /history/{id} [delete]
func (h *HistoryHandler) Delete(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	record, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if record.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteAll godoc
// @Summary Delete all history records
// @Description Delete all history records for current user
// @Tags history
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /history [delete]
func (h *HistoryHandler) DeleteAll(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.service.DeleteByUser(c.Request().Context(), userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *HistoryHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

