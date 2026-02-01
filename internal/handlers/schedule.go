package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/schedule"
)

type ScheduleHandler struct {
	service *schedule.Service
	logger  *slog.Logger
}

func NewScheduleHandler(service *schedule.Service, log *slog.Logger) *ScheduleHandler {
	return &ScheduleHandler{
		service: service,
		logger:  log,
	}
}

func (h *ScheduleHandler) Register(e *echo.Echo) {
	group := e.Group("/schedule")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary Create schedule
// @Description Create a schedule for current user
// @Tags schedule
// @Param payload body schedule.CreateRequest true "Schedule payload"
// @Success 201 {object} schedule.Schedule
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /schedule [post]
func (h *ScheduleHandler) Create(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req schedule.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Create(c.Request().Context(), userID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List schedules
// @Description List schedules for current user
// @Tags schedule
// @Success 200 {object} schedule.ListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /schedule [get]
func (h *ScheduleHandler) List(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.service.List(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, schedule.ListResponse{Items: items})
}

// Get godoc
// @Summary Get schedule
// @Description Get a schedule by ID
// @Tags schedule
// @Param id path string true "Schedule ID"
// @Success 200 {object} schedule.Schedule
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /schedule/{id} [get]
func (h *ScheduleHandler) Get(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	item, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if item.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	return c.JSON(http.StatusOK, item)
}

// Update godoc
// @Summary Update schedule
// @Description Update a schedule by ID
// @Tags schedule
// @Param id path string true "Schedule ID"
// @Param payload body schedule.UpdateRequest true "Schedule payload"
// @Success 200 {object} schedule.Schedule
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /schedule/{id} [put]
func (h *ScheduleHandler) Update(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req schedule.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	item, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if item.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	resp, err := h.service.Update(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete schedule
// @Description Delete a schedule by ID
// @Tags schedule
// @Param id path string true "Schedule ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /schedule/{id} [delete]
func (h *ScheduleHandler) Delete(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	item, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if item.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "user mismatch")
	}
	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ScheduleHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

