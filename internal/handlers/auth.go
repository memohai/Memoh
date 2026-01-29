package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type AuthHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
	expiresIn time.Duration
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

func NewAuthHandler(db *pgxpool.Pool, jwtSecret string, expiresIn time.Duration) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
		expiresIn: expiresIn,
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
	if h.db == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "db not configured")
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

	user, err := fetchUserByIdentity(c.Request().Context(), h.db, req.Username)
	if err != nil {
		if err == pgx.ErrNoRows {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !user.IsActive {
		return echo.NewHTTPError(http.StatusUnauthorized, "user is inactive")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	userID, err := formatUserID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	token, expiresAt, err := auth.GenerateToken(userID, h.jwtSecret, h.expiresIn)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_ = h.touchLastLogin(c.Request().Context(), user.ID)

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		UserID:      userID,
		Username:    user.Username,
		Role:        fmt.Sprintf("%v", user.Role),
		DisplayName: user.DisplayName.String,
	})
}

func fetchUserByIdentity(ctx context.Context, db *pgxpool.Pool, identity string) (sqlc.User, error) {
	query := `
		SELECT id, username, email, password_hash, role, display_name, avatar_url, is_active, created_at, updated_at, last_login_at
		FROM users
		WHERE username = $1 OR email = $1
	`
	row := db.QueryRow(ctx, query, identity)
	var user sqlc.User
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.DisplayName,
		&user.AvatarUrl,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	return user, err
}

func formatUserID(id pgtype.UUID) (string, error) {
	if !id.Valid {
		return "", fmt.Errorf("user id is invalid")
	}
	parsed, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func (h *AuthHandler) touchLastLogin(ctx context.Context, id pgtype.UUID) error {
	if !id.Valid {
		return fmt.Errorf("user id is invalid")
	}
	_, err := h.db.Exec(ctx, "UPDATE users SET last_login_at = now() WHERE id = $1", id)
	return err
}
