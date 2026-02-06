package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/users"
)

type UsersHandler struct {
	service        *users.Service
	botService     *bots.Service
	channelService *channel.Service
	channelManager *channel.Manager
	logger         *slog.Logger
}

func NewUsersHandler(log *slog.Logger, service *users.Service, botService *bots.Service, channelService *channel.Service, channelManager *channel.Manager) *UsersHandler {
	if log == nil {
		log = slog.Default()
	}
	return &UsersHandler{
		service:        service,
		botService:     botService,
		channelService: channelService,
		channelManager: channelManager,
		logger:         log.With(slog.String("handler", "users")),
	}
}

func (h *UsersHandler) Register(e *echo.Echo) {
	userGroup := e.Group("/users")
	userGroup.GET("/me", h.GetMe)
	userGroup.PUT("/me", h.UpdateMe)
	userGroup.PUT("/me/password", h.UpdateMyPassword)
	userGroup.GET("", h.ListUsers)
	userGroup.GET("/:id", h.GetUser)
	userGroup.PUT("/:id", h.UpdateUser)
	userGroup.PUT("/:id/password", h.ResetUserPassword)
	userGroup.POST("", h.CreateUser)

	botGroup := e.Group("/bots")
	botGroup.POST("", h.CreateBot)
	botGroup.GET("", h.ListBots)
	botGroup.GET("/:id", h.GetBot)
	botGroup.PUT("/:id", h.UpdateBot)
	botGroup.PUT("/:id/owner", h.TransferBotOwner)
	botGroup.DELETE("/:id", h.DeleteBot)
	botGroup.GET("/:id/members", h.ListBotMembers)
	botGroup.PUT("/:id/members", h.UpsertBotMember)
	botGroup.DELETE("/:id/members/:user_id", h.DeleteBotMember)
	botGroup.GET("/:id/channel/:platform", h.GetBotChannelConfig)
	botGroup.PUT("/:id/channel/:platform", h.UpsertBotChannelConfig)
	botGroup.POST("/:id/channel/:platform/send", h.SendBotMessage)
	botGroup.POST("/:id/channel/:platform/send_session", h.SendBotMessageSession)
}

// GetMe godoc
// @Summary Get current user
// @Description Get current user profile
// @Tags users
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/me [get]
func (h *UsersHandler) GetMe(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	resp, err := h.service.Get(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateMe godoc
// @Summary Update current user profile
// @Description Update current user display name or avatar
// @Tags users
// @Param payload body users.UpdateProfileRequest true "Profile payload"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/me [put]
func (h *UsersHandler) UpdateMe(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req users.UpdateProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateProfile(c.Request().Context(), userID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateMyPassword godoc
// @Summary Update current user password
// @Description Update current user password with current password check
// @Tags users
// @Param payload body users.UpdatePasswordRequest true "Password payload"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/me/password [put]
func (h *UsersHandler) UpdateMyPassword(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req users.UpdatePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.service.UpdatePassword(c.Request().Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, users.ErrInvalidPassword) {
			return echo.NewHTTPError(http.StatusBadRequest, "current password mismatch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ListUsers godoc
// @Summary List users (admin only)
// @Description List users
// @Tags users
// @Success 200 {object} users.ListUsersResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users [get]
func (h *UsersHandler) ListUsers(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	if strings.TrimSpace(c.QueryParam("user_type")) != "" || strings.TrimSpace(c.QueryParam("owner_id")) != "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_type and owner_id are not supported")
	}
	items, err := h.service.ListUsers(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, users.ListUsersResponse{Items: items})
}

// GetUser godoc
// @Summary Get user by ID
// @Description Get user details (self or admin only)
// @Tags users
// @Param id path string true "User ID"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id} [get]
func (h *UsersHandler) GetUser(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	targetID := strings.TrimSpace(c.Param("id"))
	if targetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user id is required")
	}
	if targetID != actorID {
		isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !isAdmin {
			return echo.NewHTTPError(http.StatusForbidden, "user access denied")
		}
	}
	user, err := h.service.Get(c.Request().Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, user)
}

// UpdateUser godoc
// @Summary Update user (admin only)
// @Description Update user profile and status
// @Tags users
// @Param id path string true "User ID"
// @Param payload body users.UpdateUserRequest true "User update payload"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id} [put]
func (h *UsersHandler) UpdateUser(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	targetID := strings.TrimSpace(c.Param("id"))
	if targetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user id is required")
	}
	_, err = h.service.Get(c.Request().Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	var req users.UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateUserAdmin(c.Request().Context(), targetID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// ResetUserPassword godoc
// @Summary Reset user password (admin only)
// @Description Reset a user password
// @Tags users
// @Param id path string true "User ID"
// @Param payload body users.ResetPasswordRequest true "Password payload"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id}/password [put]
func (h *UsersHandler) ResetUserPassword(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	targetID := strings.TrimSpace(c.Param("id"))
	if targetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user id is required")
	}
	if _, err := h.service.Get(c.Request().Context(), targetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	var req users.ResetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.service.ResetPassword(c.Request().Context(), targetID, req.NewPassword); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// CreateUser godoc
// @Summary Create human user (admin only)
// @Description Create a new human user account
// @Tags users
// @Param payload body users.CreateUserRequest true "User payload"
// @Success 201 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users [post]
func (h *UsersHandler) CreateUser(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	var req users.CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.CreateHuman(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// CreateBot godoc
// @Summary Create bot user
// @Description Create a bot user owned by current user (or admin-specified owner)
// @Tags bots
// @Param payload body bots.CreateBotRequest true "Bot payload"
// @Success 201 {object} bots.Bot
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots [post]
func (h *UsersHandler) CreateBot(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req bots.CreateBotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ownerID := actorID
	if raw := strings.TrimSpace(c.QueryParam("owner_id")); raw != "" {
		isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !isAdmin {
			return echo.NewHTTPError(http.StatusForbidden, "admin role required for owner override")
		}
		ownerID = raw
	}
	resp, err := h.botService.Create(c.Request().Context(), ownerID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// ListBots godoc
// @Summary List bots
// @Description List bots accessible to current user (admin can specify owner_id)
// @Tags bots
// @Param owner_id query string false "Owner user ID (admin only)"
// @Success 200 {object} bots.ListBotsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots [get]
func (h *UsersHandler) ListBots(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	ownerID := strings.TrimSpace(c.QueryParam("owner_id"))
	if ownerID != "" {
		isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !isAdmin {
			return echo.NewHTTPError(http.StatusForbidden, "admin role required for owner filter")
		}
		items, err := h.botService.ListByOwner(c.Request().Context(), ownerID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, bots.ListBotsResponse{Items: items})
	}
	items, err := h.botService.ListAccessible(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, bots.ListBotsResponse{Items: items})
}

// GetBot godoc
// @Summary Get bot details
// @Description Get a bot by ID (owner/admin only)
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} bots.Bot
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id} [get]
func (h *UsersHandler) GetBot(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	bot, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, bot)
}

// UpdateBot godoc
// @Summary Update bot details
// @Description Update bot profile (owner/admin only)
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body bots.UpdateBotRequest true "Bot update payload"
// @Success 200 {object} bots.Bot
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id} [put]
func (h *UsersHandler) UpdateBot(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	var req bots.UpdateBotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.botService.Update(c.Request().Context(), botID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// TransferBotOwner godoc
// @Summary Transfer bot owner (admin only)
// @Description Transfer bot ownership to another human user
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body bots.TransferBotRequest true "Transfer payload"
// @Success 200 {object} bots.Bot
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/owner [put]
func (h *UsersHandler) TransferBotOwner(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	isAdmin, err := h.service.IsAdmin(c.Request().Context(), actorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	var req bots.TransferBotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.botService.TransferOwner(c.Request().Context(), botID, req.OwnerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteBot godoc
// @Summary Delete bot
// @Description Delete a bot user (owner/admin only)
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id} [delete]
func (h *UsersHandler) DeleteBot(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	if err := h.botService.Delete(c.Request().Context(), botID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ListBotMembers godoc
// @Summary List bot members
// @Description List members for a bot
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} bots.ListMembersResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/members [get]
func (h *UsersHandler) ListBotMembers(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	items, err := h.botService.ListMembers(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, bots.ListMembersResponse{Items: items})
}

// UpsertBotMember godoc
// @Summary Upsert bot member
// @Description Add or update bot member role
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body bots.UpsertMemberRequest true "Member payload"
// @Success 200 {object} bots.BotMember
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/members [put]
func (h *UsersHandler) UpsertBotMember(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	var req bots.UpsertMemberRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id is required")
	}
	resp, err := h.botService.UpsertMember(c.Request().Context(), botID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteBotMember godoc
// @Summary Delete bot member
// @Description Remove a member from a bot
// @Tags bots
// @Param id path string true "Bot ID"
// @Param user_id path string true "User ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/members/{user_id} [delete]
func (h *UsersHandler) DeleteBotMember(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	if err := h.botService.DeleteMember(c.Request().Context(), botID, userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// GetBotChannelConfig godoc
// @Summary Get bot channel config
// @Description Get bot channel configuration
// @Tags bots
// @Param id path string true "Bot ID"
// @Param platform path string true "Channel platform"
// @Success 200 {object} channel.ChannelConfig
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/{platform} [get]
func (h *UsersHandler) GetBotChannelConfig(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	channelType, err := channel.ParseChannelType(c.Param("platform"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.channelService.ResolveEffectiveConfig(c.Request().Context(), botID, channelType)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpsertBotChannelConfig godoc
// @Summary Update bot channel config
// @Description Update bot channel configuration
// @Tags bots
// @Param id path string true "Bot ID"
// @Param platform path string true "Channel platform"
// @Param payload body channel.UpsertConfigRequest true "Channel config payload"
// @Success 200 {object} channel.ChannelConfig
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/{platform} [put]
func (h *UsersHandler) UpsertBotChannelConfig(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	channelType, err := channel.ParseChannelType(c.Param("platform"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var req channel.UpsertConfigRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Credentials == nil {
		req.Credentials = map[string]any{}
	}
	resp, err := h.channelService.UpsertConfig(c.Request().Context(), botID, channelType, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// SendBotMessage godoc
// @Summary Send message via bot channel
// @Description Send a message using bot channel configuration
// @Tags bots
// @Param id path string true "Bot ID"
// @Param platform path string true "Channel platform"
// @Param payload body channel.SendRequest true "Send payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/{platform}/send [post]
func (h *UsersHandler) SendBotMessage(c echo.Context) error {
	actorID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), actorID, botID); err != nil {
		return err
	}
	if h.channelManager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel manager not configured")
	}
	channelType, err := channel.ParseChannelType(c.Param("platform"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var req channel.SendRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Message.IsEmpty() {
		return echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	if err := h.channelManager.Send(c.Request().Context(), botID, channelType, req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SendBotMessageSession godoc
// @Summary Send message via bot channel session token
// @Description Send a message using a session-scoped token (reply only)
// @Tags bots
// @Param id path string true "Bot ID"
// @Param platform path string true "Channel platform"
// @Param payload body channel.SendRequest true "Send payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/{platform}/send_session [post]
func (h *UsersHandler) SendBotMessageSession(c echo.Context) error {
	sessionToken, err := auth.SessionTokenFromContext(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	channelType, err := channel.ParseChannelType(c.Param("platform"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if sessionToken.BotID != botID || sessionToken.Platform != channelType.String() {
		return echo.NewHTTPError(http.StatusForbidden, "session token mismatch")
	}
	if h.channelManager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel manager not configured")
	}
	var req channel.SendRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Message.IsEmpty() {
		return echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	if strings.TrimSpace(sessionToken.ReplyTarget) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "reply target missing")
	}
	if err := h.channelManager.Send(c.Request().Context(), botID, channelType, channel.SendRequest{
		Target:  sessionToken.ReplyTarget,
		Message: req.Message,
	}); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *UsersHandler) authorizeBotAccess(ctx context.Context, actorID, botID string) (bots.Bot, error) {
	isAdmin, err := h.service.IsAdmin(ctx, actorID)
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

func (h *UsersHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}
