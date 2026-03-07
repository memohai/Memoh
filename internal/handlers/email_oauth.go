package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/email"
	emailgmail "github.com/memohai/memoh/internal/email/adapters/gmail"
)

// EmailOAuthHandler handles the OAuth2 authorization flow for Gmail providers.
type EmailOAuthHandler struct {
	service     *email.Service
	tokenStore  email.OAuthTokenStore
	callbackURL string
	logger      *slog.Logger
}

type emailOAuthStatusResponse struct {
	Provider     string     `json:"provider"`
	Configured   bool       `json:"configured"`
	HasToken     bool       `json:"has_token"`
	Expired      bool       `json:"expired"`
	EmailAddress string     `json:"email_address,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func NewEmailOAuthHandler(log *slog.Logger, service *email.Service, tokenStore email.OAuthTokenStore, callbackURL string) *EmailOAuthHandler {
	return &EmailOAuthHandler{
		service:     service,
		tokenStore:  tokenStore,
		callbackURL: callbackURL,
		logger:      log.With(slog.String("handler", "email_oauth")),
	}
}

func (h *EmailOAuthHandler) Register(e *echo.Echo) {
	e.GET("/email-providers/:id/oauth/authorize", h.Authorize)
	e.GET("/email-providers/:id/oauth/status", h.Status)
	e.DELETE("/email-providers/:id/oauth/token", h.Revoke)
	e.GET("/email/oauth/callback", h.Callback)
}

// Authorize godoc
// @Summary Start OAuth2 authorization for an email provider
// @Description Returns the authorization URL to redirect the user to
// @Tags email-oauth
// @Param id path string true "Email provider ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /email-providers/{id}/oauth/authorize [get].
func (h *EmailOAuthHandler) Authorize(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	provider, err := h.service.GetProvider(c.Request().Context(), providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}

	state, err := generateState()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate state")
	}

	if err := h.tokenStore.SetPendingState(c.Request().Context(), providerID, state); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to store state")
	}

	var authURL string
	if email.ProviderName(provider.Provider) == emailgmail.ProviderName {
		clientID, _ := provider.Config["client_id"].(string)
		if strings.TrimSpace(clientID) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "client_id is not configured for this provider")
		}
		adapter := emailgmail.New(h.logger, h.tokenStore)
		authURL = adapter.AuthorizeURL(clientID, h.callbackURL, state)
	}
	if authURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider does not support OAuth2")
	}

	return c.JSON(http.StatusOK, map[string]string{"auth_url": authURL})
}

// Callback godoc
// @Summary OAuth2 callback for email providers
// @Description Handles the OAuth2 callback, exchanges the code for tokens
// @Tags email-oauth
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /email/oauth/callback [get].
func (h *EmailOAuthHandler) Callback(c echo.Context) error {
	code := strings.TrimSpace(c.QueryParam("code"))
	state := strings.TrimSpace(c.QueryParam("state"))

	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	if state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "state is required")
	}

	ctx := c.Request().Context()

	stored, err := h.tokenStore.GetByState(ctx, state)
	if err != nil {
		h.logger.Error("oauth callback: state not found", slog.String("state", state), slog.Any("error", err))
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired state")
	}

	provider, err := h.service.GetProvider(ctx, stored.ProviderID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "provider not found")
	}

	if email.ProviderName(provider.Provider) != emailgmail.ProviderName {
		return echo.NewHTTPError(http.StatusBadRequest, "provider does not support OAuth2")
	}
	adapter := emailgmail.New(h.logger, h.tokenStore)
	if err := adapter.ExchangeCode(ctx, provider.Config, stored.ProviderID, code, h.callbackURL); err != nil {
		h.logger.Error("gmail code exchange failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "token exchange failed")
	}

	h.logger.Info("email oauth authorized", slog.String("provider_id", stored.ProviderID), slog.String("provider", provider.Provider))
	return c.JSON(http.StatusOK, map[string]string{"status": "authorized"})
}

// Status godoc
// @Summary Get OAuth2 status for an email provider
// @Tags email-oauth
// @Param id path string true "Email provider ID"
// @Success 200 {object} emailOAuthStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /email-providers/{id}/oauth/status [get].
func (h *EmailOAuthHandler) Status(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	ctx := c.Request().Context()
	provider, err := h.service.GetProvider(ctx, providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}
	if !supportsEmailOAuth(email.ProviderName(provider.Provider)) {
		return echo.NewHTTPError(http.StatusBadRequest, "provider does not support OAuth2")
	}

	resp := emailOAuthStatusResponse{
		Provider:   provider.Provider,
		Configured: isProviderConfigured(provider),
	}

	token, err := h.tokenStore.Get(ctx, providerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusOK, resp)
		}
		h.logger.Error("email oauth status failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load oauth status")
	}

	resp.HasToken = token.AccessToken != ""
	resp.EmailAddress = token.EmailAddress
	if !token.ExpiresAt.IsZero() {
		expiresAt := token.ExpiresAt
		resp.ExpiresAt = &expiresAt
		resp.Expired = time.Now().After(token.ExpiresAt)
	}

	return c.JSON(http.StatusOK, resp)
}

// Revoke godoc
// @Summary Revoke stored OAuth2 tokens for an email provider
// @Tags email-oauth
// @Param id path string true "Email provider ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /email-providers/{id}/oauth/token [delete].
func (h *EmailOAuthHandler) Revoke(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	ctx := c.Request().Context()
	provider, err := h.service.GetProvider(ctx, providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}
	if !supportsEmailOAuth(email.ProviderName(provider.Provider)) {
		return echo.NewHTTPError(http.StatusBadRequest, "provider does not support OAuth2")
	}

	if err := h.tokenStore.Delete(ctx, providerID); err != nil {
		h.logger.Error("email oauth revoke failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to revoke oauth token")
	}

	return c.NoContent(http.StatusNoContent)
}

func supportsEmailOAuth(name email.ProviderName) bool {
	return name == emailgmail.ProviderName
}

func isProviderConfigured(provider email.ProviderResponse) bool {
	config := provider.Config
	if config == nil {
		config = map[string]any{}
	}
	if email.ProviderName(provider.Provider) != emailgmail.ProviderName {
		return false
	}
	clientID, _ := config["client_id"].(string)
	return strings.TrimSpace(clientID) != ""
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
