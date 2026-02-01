package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/subagent"
)

type SubagentHandler struct {
	service *subagent.Service
	logger  *slog.Logger
}

func NewSubagentHandler(log *slog.Logger, service *subagent.Service) *SubagentHandler {
	return &SubagentHandler{
		service: service,
		logger:  log,
	}
}

func (h *SubagentHandler) Register(e *echo.Echo) {
	group := e.Group("/subagents")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
	group.GET("/:id/context", h.GetContext)
	group.PUT("/:id/context", h.UpdateContext)
	group.GET("/:id/skills", h.GetSkills)
	group.PUT("/:id/skills", h.UpdateSkills)
	group.POST("/:id/skills", h.AddSkills)
}

// Create godoc
// @Summary Create subagent
// @Description Create a subagent for current user
// @Tags subagent
// @Param payload body subagent.CreateRequest true "Subagent payload"
// @Success 201 {object} subagent.Subagent
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents [post]
func (h *SubagentHandler) Create(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req subagent.CreateRequest
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
// @Summary List subagents
// @Description List subagents for current user
// @Tags subagent
// @Success 200 {object} subagent.ListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents [get]
func (h *SubagentHandler) List(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.service.List(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, subagent.ListResponse{Items: items})
}

// Get godoc
// @Summary Get subagent
// @Description Get a subagent by ID
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Success 200 {object} subagent.Subagent
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id} [get]
func (h *SubagentHandler) Get(c echo.Context) error {
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
// @Summary Update subagent
// @Description Update a subagent by ID
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Param payload body subagent.UpdateRequest true "Subagent payload"
// @Success 200 {object} subagent.Subagent
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id} [put]
func (h *SubagentHandler) Update(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req subagent.UpdateRequest
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
// @Summary Delete subagent
// @Description Delete a subagent by ID
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id} [delete]
func (h *SubagentHandler) Delete(c echo.Context) error {
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

// GetContext godoc
// @Summary Get subagent context
// @Description Get a subagent's message context
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Success 200 {object} subagent.ContextResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id}/context [get]
func (h *SubagentHandler) GetContext(c echo.Context) error {
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
	return c.JSON(http.StatusOK, subagent.ContextResponse{Messages: item.Messages})
}

// UpdateContext godoc
// @Summary Update subagent context
// @Description Update a subagent's message context
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Param payload body subagent.UpdateContextRequest true "Context payload"
// @Success 200 {object} subagent.ContextResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id}/context [put]
func (h *SubagentHandler) UpdateContext(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req subagent.UpdateContextRequest
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
	updated, err := h.service.UpdateContext(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, subagent.ContextResponse{Messages: updated.Messages})
}

// GetSkills godoc
// @Summary Get subagent skills
// @Description Get a subagent's skills
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Success 200 {object} subagent.SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id}/skills [get]
func (h *SubagentHandler) GetSkills(c echo.Context) error {
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
	return c.JSON(http.StatusOK, subagent.SkillsResponse{Skills: item.Skills})
}

// UpdateSkills godoc
// @Summary Update subagent skills
// @Description Replace a subagent's skills
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Param payload body subagent.UpdateSkillsRequest true "Skills payload"
// @Success 200 {object} subagent.SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id}/skills [put]
func (h *SubagentHandler) UpdateSkills(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req subagent.UpdateSkillsRequest
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
	updated, err := h.service.UpdateSkills(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, subagent.SkillsResponse{Skills: updated.Skills})
}

// AddSkills godoc
// @Summary Add subagent skills
// @Description Add skills to a subagent
// @Tags subagent
// @Param id path string true "Subagent ID"
// @Param payload body subagent.AddSkillsRequest true "Skills payload"
// @Success 200 {object} subagent.SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subagents/{id}/skills [post]
func (h *SubagentHandler) AddSkills(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req subagent.AddSkillsRequest
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
	updated, err := h.service.AddSkills(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, subagent.SkillsResponse{Skills: updated.Skills})
}

func (h *SubagentHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

