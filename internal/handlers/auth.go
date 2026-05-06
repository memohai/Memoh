package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	iamaccounts "github.com/memohai/memoh/internal/iam/accounts"
	iamauth "github.com/memohai/memoh/internal/iam/auth"
	"github.com/memohai/memoh/internal/iam/sso"
)

type AuthHandler struct {
	accountService AuthAccountService
	sessionStore   AuthSessionStore
	ssoService     AuthSSOService
	jwtSecret      string
	expiresIn      time.Duration
	logger         *slog.Logger
}

type AuthAccountService interface {
	Login(ctx context.Context, identity string, password string) (accounts.Account, error)
}

type AuthSessionStore interface {
	CreateSession(ctx context.Context, input AuthSessionInput) (AuthSession, error)
	ValidateSession(ctx context.Context, userID string, sessionID string) error
	RevokeSession(ctx context.Context, sessionID string) error
}

type AuthSessionInput struct {
	UserID     string
	IdentityID string
	ExpiresAt  time.Time
	IPAddress  string
	UserAgent  string
}

type AuthSession struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

type AuthSSOService interface {
	ListEnabledProviders(ctx context.Context) ([]sso.Provider, error)
	GetProvider(ctx context.Context, providerID string) (sso.Provider, error)
	BuildOIDCAuthRedirect(ctx context.Context, provider sso.Provider, state string, nonce string, codeVerifier string) (sso.OIDCAuthRedirect, error)
	CompleteOIDCCallback(ctx context.Context, provider sso.Provider, code string, state sso.OIDCState) (sso.LoginCode, error)
	BuildSAMLAuthRedirect(ctx context.Context, provider sso.Provider) (sso.SAMLAuthRedirect, error)
	CompleteSAMLACS(ctx context.Context, provider sso.Provider, r *http.Request, state sso.SAMLState) (sso.LoginCode, error)
	BuildSAMLMetadata(ctx context.Context, provider sso.Provider) (string, error)
	ExchangeLoginCode(ctx context.Context, code string) (sso.LoginCode, error)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` //nolint:gosec // intentional: JSON request field carrying a user-supplied credential
}

type LoginResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // intentional: JWT is the purpose of this response field
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
	UserID      string `json:"user_id"`
	SessionID   string `json:"session_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Timezone    string `json:"timezone,omitempty"`
}

func NewAuthHandler(log *slog.Logger, accountService AuthAccountService, jwtSecret string, expiresIn time.Duration) *AuthHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AuthHandler{
		accountService: accountService,
		jwtSecret:      jwtSecret,
		expiresIn:      expiresIn,
		logger:         log.With(slog.String("handler", "auth")),
	}
}

type IAMAccountServiceAdapter struct {
	service *iamaccounts.Service
}

func NewIAMAccountServiceAdapter(service *iamaccounts.Service) *IAMAccountServiceAdapter {
	return &IAMAccountServiceAdapter{service: service}
}

func (a *IAMAccountServiceAdapter) Login(ctx context.Context, identity string, password string) (accounts.Account, error) {
	account, err := a.service.Login(ctx, identity, password)
	if err != nil {
		return accounts.Account{}, err
	}
	return accounts.Account{
		ID:          account.ID,
		Username:    account.Username,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		AvatarURL:   account.AvatarURL,
		Timezone:    account.Timezone,
		IsActive:    account.IsActive,
		CreatedAt:   account.CreatedAt,
		UpdatedAt:   account.UpdatedAt,
		LastLoginAt: account.LastLoginAt,
	}, nil
}

func (h *AuthHandler) SetSessionStore(store AuthSessionStore) {
	h.sessionStore = store
}

func (h *AuthHandler) SetSSOService(service AuthSSOService) {
	h.ssoService = service
}

func (h *AuthHandler) Register(e *echo.Echo) {
	e.POST("/auth/login", h.Login)
	e.POST("/auth/logout", h.Logout)
	e.POST("/auth/refresh", h.Refresh)
	e.GET("/auth/sso/providers", h.ListSSOProviders)
	e.GET("/auth/sso/:provider_id/start", h.StartOIDC)
	e.GET("/auth/sso/:provider_id/callback", h.OIDCCallback)
	e.GET("/auth/sso/:provider_id/saml/start", h.StartSAML)
	e.POST("/auth/sso/:provider_id/saml/acs", h.SAMLACS)
	e.GET("/auth/sso/:provider_id/saml/metadata", h.SAMLMetadata)
	e.POST("/auth/sso/exchange", h.ExchangeSSOCode)
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
// @Router /auth/login [post].
func (h *AuthHandler) Login(c echo.Context) error {
	if h.accountService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "user service not configured")
	}
	if strings.TrimSpace(h.jwtSecret) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt secret not configured")
	}
	if h.expiresIn <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt expiry not configured")
	}
	if h.sessionStore == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session store not configured")
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || strings.TrimSpace(req.Password) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
	}

	account, err := h.accountService.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		if isInvalidCredentials(err) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		if isInactiveAccount(err) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user is inactive")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	session, err := h.sessionStore.CreateSession(c.Request().Context(), AuthSessionInput{
		UserID:    account.ID,
		ExpiresAt: time.Now().UTC().Add(h.expiresIn),
		IPAddress: c.RealIP(),
		UserAgent: c.Request().UserAgent(),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	token, expiresAt, err := iamauth.GenerateToken(account.ID, session.ID, h.jwtSecret, h.expiresIn)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		UserID:      account.ID,
		SessionID:   session.ID,
		Username:    account.Username,
		DisplayName: account.DisplayName,
		Timezone:    account.Timezone,
	})
}

func isInvalidCredentials(err error) bool {
	return errors.Is(err, accounts.ErrInvalidCredentials) || errors.Is(err, iamaccounts.ErrInvalidCredentials)
}

func isInactiveAccount(err error) bool {
	return errors.Is(err, accounts.ErrInactiveAccount) || errors.Is(err, iamaccounts.ErrInactiveAccount)
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // intentional: JWT is the purpose of this response field
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
}

type ExchangeSSOCodeRequest struct {
	Code string `json:"code"`
}

type SSOProviderResponse struct {
	ID      string           `json:"id"`
	Type    sso.ProviderType `json:"type"`
	Key     string           `json:"key"`
	Name    string           `json:"name"`
	Enabled bool             `json:"enabled"`
}

type ListSSOProvidersResponse struct {
	Items []SSOProviderResponse `json:"items"`
}

type OIDCStartResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// Refresh godoc
// @Summary Refresh Token
// @Description Issue a new JWT after validating the active IAM session
// @Tags auth
// @Security BearerAuth
// @Success 200 {object} RefreshResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/refresh [post].
func (h *AuthHandler) Refresh(c echo.Context) error {
	if strings.TrimSpace(h.jwtSecret) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt secret not configured")
	}
	if h.expiresIn <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt expiry not configured")
	}
	if h.sessionStore == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session store not configured")
	}

	userID, err := iamauth.UserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	sessionID, err := iamauth.SessionIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	if err := h.sessionStore.ValidateSession(c.Request().Context(), userID, sessionID); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}
	token, expiresAt, err := iamauth.GenerateToken(userID, sessionID, h.jwtSecret, h.expiresIn)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, RefreshResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	if h.sessionStore == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session store not configured")
	}
	sessionID, err := iamauth.SessionIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	if err := h.sessionStore.RevokeSession(c.Request().Context(), sessionID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ListSSOProviders godoc
// @Summary List SSO providers
// @Description List enabled OIDC and SAML SSO providers available for login
// @Tags auth
// @Success 200 {object} ListSSOProvidersResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/sso/providers [get].
func (h *AuthHandler) ListSSOProviders(c echo.Context) error {
	if h.ssoService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "sso service not configured")
	}
	providers, err := h.ssoService.ListEnabledProviders(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]SSOProviderResponse, 0, len(providers))
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		items = append(items, SSOProviderResponse{
			ID:      provider.ID,
			Type:    provider.Type,
			Key:     provider.Key,
			Name:    provider.Name,
			Enabled: provider.Enabled,
		})
	}
	return c.JSON(http.StatusOK, ListSSOProvidersResponse{Items: items})
}

// StartOIDC godoc
// @Summary Start OIDC login
// @Description Build the provider authorization URL and set the transient OIDC state cookie
// @Tags auth
// @Param provider_id path string true "SSO provider ID or key"
// @Success 200 {object} OIDCStartResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/sso/{provider_id}/start [get].
func (h *AuthHandler) StartOIDC(c echo.Context) error {
	provider, err := h.ssoProvider(c)
	if err != nil {
		return err
	}
	if provider.Type != sso.ProviderTypeOIDC {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is not oidc")
	}
	state, err := randomURLToken(32)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	codeVerifier, err := randomURLToken(64)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	redirect, err := h.ssoService.BuildOIDCAuthRedirect(c.Request().Context(), provider, state, nonce, codeVerifier)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	now := time.Now().UTC()
	if err := sso.SetOIDCStateCookie(c.Response(), sso.OIDCState{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		CreatedAt:    now,
	}, now); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, OIDCStartResponse{RedirectURL: redirect.URL})
}

// OIDCCallback godoc
// @Summary Complete OIDC login
// @Description Exchange the provider code, create an IAM session, and redirect to the web login callback with a short-lived login code
// @Tags auth
// @Param provider_id path string true "SSO provider ID or key"
// @Param code query string true "OIDC authorization code"
// @Param state query string true "OIDC state"
// @Success 302
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /auth/sso/{provider_id}/callback [get].
func (h *AuthHandler) OIDCCallback(c echo.Context) error {
	provider, err := h.ssoProvider(c)
	if err != nil {
		return err
	}
	if provider.Type != sso.ProviderTypeOIDC {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is not oidc")
	}
	state, err := sso.ReadAndClearOIDCStateCookie(c.Response(), c.Request(), time.Now().UTC())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid oidc state")
	}
	if c.QueryParam("state") != state.State {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid oidc state")
	}
	code := strings.TrimSpace(c.QueryParam("code"))
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	loginCode, err := h.ssoService.CompleteOIDCCallback(c.Request().Context(), provider, code, state)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	return h.redirectWithLoginCode(c, loginCode)
}

// StartSAML godoc
// @Summary Start SAML login
// @Description Create a SAML AuthnRequest, set the transient SAML state cookie, and redirect to the IdP
// @Tags auth
// @Param provider_id path string true "SSO provider ID or key"
// @Success 302
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/sso/{provider_id}/saml/start [get].
func (h *AuthHandler) StartSAML(c echo.Context) error {
	provider, err := h.ssoProvider(c)
	if err != nil {
		return err
	}
	if provider.Type != sso.ProviderTypeSAML {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is not saml")
	}
	redirect, err := h.ssoService.BuildSAMLAuthRedirect(c.Request().Context(), provider)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	now := time.Now().UTC()
	if err := sso.SetSAMLStateCookie(c.Response(), sso.SAMLState{
		RelayState: redirect.RelayState,
		RequestID:  redirect.RequestID,
		CreatedAt:  now,
	}, now); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.Redirect(http.StatusFound, redirect.URL)
}

// SAMLACS godoc
// @Summary Complete SAML login
// @Description Validate the SAML response, create an IAM session, and redirect to the web login callback with a short-lived login code
// @Tags auth
// @Param provider_id path string true "SSO provider ID or key"
// @Success 302
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /auth/sso/{provider_id}/saml/acs [post].
func (h *AuthHandler) SAMLACS(c echo.Context) error {
	provider, err := h.ssoProvider(c)
	if err != nil {
		return err
	}
	if provider.Type != sso.ProviderTypeSAML {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is not saml")
	}
	state, err := sso.ReadAndClearSAMLStateCookie(c.Response(), c.Request(), time.Now().UTC())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid saml state")
	}
	loginCode, err := h.ssoService.CompleteSAMLACS(c.Request().Context(), provider, c.Request(), state)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	return h.redirectWithLoginCode(c, loginCode)
}

// SAMLMetadata godoc
// @Summary Get SAML SP metadata
// @Description Return the SAML service provider metadata XML for an SSO provider
// @Tags auth
// @Param provider_id path string true "SSO provider ID or key"
// @Success 200 {string} string
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/sso/{provider_id}/saml/metadata [get].
func (h *AuthHandler) SAMLMetadata(c echo.Context) error {
	provider, err := h.ssoProvider(c)
	if err != nil {
		return err
	}
	if provider.Type != sso.ProviderTypeSAML {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is not saml")
	}
	metadata, err := h.ssoService.BuildSAMLMetadata(c.Request().Context(), provider)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.Blob(http.StatusOK, "application/samlmetadata+xml", []byte(metadata))
}

// ExchangeSSOCode godoc
// @Summary Exchange SSO login code
// @Description Exchange a one-time SSO login code for a session JWT
// @Tags auth
// @Param payload body ExchangeSSOCodeRequest true "SSO exchange request"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/sso/exchange [post].
func (h *AuthHandler) ExchangeSSOCode(c echo.Context) error {
	if strings.TrimSpace(h.jwtSecret) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt secret not configured")
	}
	if h.expiresIn <= 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "jwt expiry not configured")
	}
	if h.ssoService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "sso service not configured")
	}
	var req ExchangeSSOCodeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	loginCode, err := h.ssoService.ExchangeLoginCode(c.Request().Context(), code)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	return h.issueLoginCodeToken(c, loginCode)
}

func (h *AuthHandler) ssoProvider(c echo.Context) (sso.Provider, error) {
	if h.ssoService == nil {
		return sso.Provider{}, echo.NewHTTPError(http.StatusInternalServerError, "sso service not configured")
	}
	providerID := strings.TrimSpace(c.Param("provider_id"))
	if providerID == "" {
		return sso.Provider{}, echo.NewHTTPError(http.StatusBadRequest, "provider_id is required")
	}
	provider, err := h.ssoService.GetProvider(c.Request().Context(), providerID)
	if err != nil {
		return sso.Provider{}, echo.NewHTTPError(http.StatusNotFound, "sso provider not found")
	}
	if !provider.Enabled {
		return sso.Provider{}, echo.NewHTTPError(http.StatusNotFound, "sso provider not found")
	}
	return provider, nil
}

func (*AuthHandler) redirectWithLoginCode(c echo.Context, loginCode sso.LoginCode) error {
	if strings.TrimSpace(loginCode.Code) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "sso login code missing")
	}
	target := strings.TrimSpace(loginCode.RedirectURL)
	if target == "" {
		target = "/login/sso/callback"
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	values := parsed.Query()
	values.Set("code", loginCode.Code)
	parsed.RawQuery = values.Encode()
	return c.Redirect(http.StatusFound, parsed.String())
}

func (h *AuthHandler) issueLoginCodeToken(c echo.Context, loginCode sso.LoginCode) error {
	userID := strings.TrimSpace(loginCode.UserID)
	sessionID := strings.TrimSpace(loginCode.SessionID)
	if userID == "" || sessionID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid login code")
	}
	if h.sessionStore != nil {
		if err := h.sessionStore.ValidateSession(c.Request().Context(), userID, sessionID); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
		}
	}
	token, expiresAt, err := iamauth.GenerateToken(userID, sessionID, h.jwtSecret, h.expiresIn)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		UserID:      userID,
		SessionID:   sessionID,
	})
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
