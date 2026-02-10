package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/users"
)

type AuthHandler struct {
	userService *users.Service
	jwtSecret   string
	expiresIn   time.Duration
	logger      *slog.Logger
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
}

func NewAuthHandler(log *slog.Logger, userService *users.Service, runtimeConfig *boot.RuntimeConfig) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		jwtSecret:   runtimeConfig.JwtSecret,
		expiresIn:   runtimeConfig.JwtExpiresIn,
		logger:      log.With(slog.String("handler", "auth")),
	}
}

func (h *AuthHandler) Register(e *echo.Echo) {
	e.POST("/auth/login", h.Login)
}

// Login godoc
// @Summary Login
// @Description Validate user credentials and issue a JWT
// @Tags auth
// @Param payload body LoginRequest true "Login request"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	if h.userService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "user service not configured")
	}
	if strings.TrimSpace(h.jwtSecret) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt secret not configured")
	}
	if h.expiresIn <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt expiry not configured")
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || strings.TrimSpace(req.Password) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
	}

	user, err := h.userService.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, users.ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		if errors.Is(err, users.ErrInactiveUser) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user is inactive")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	token, expiresAt, err := auth.GenerateToken(user.ID, h.jwtSecret, h.expiresIn)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		UserID:      user.ID,
		Username:    user.Username,
		Role:        user.Role,
		DisplayName: user.DisplayName,
	})
}
