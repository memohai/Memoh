package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/providers"
)

const (
	acpCodexOAuthStateTTL    = 10 * time.Minute
	acpCodexOAuthStatePrefix = "acp_codex_"
)

type acpCodexOAuthProvider interface {
	StartOpenAICodexACPAuthorization(ctx context.Context, redirectURI, state string) (*providers.OAuthAuthorizeResponse, string, error)
	ExchangeOpenAICodexACPCode(ctx context.Context, redirectURI, code, codeVerifier string) (providers.OpenAICodexOAuthCredentials, error)
}

type ACPCodexOAuthAuthorizeResponse struct {
	AuthURL string `json:"auth_url"`
}

type ACPCodexOAuthStatus struct {
	Configured  bool   `json:"configured"`
	HasToken    bool   `json:"has_token"`
	CallbackURL string `json:"callback_url"`
	AccountID   string `json:"account_id,omitempty"`
}

type ACPCodexOAuthHandler struct {
	provider       acpCodexOAuthProvider
	botService     *bots.Service
	accountService *accounts.Service
	acpWorkspace   acpWorkspaceConfigProvider
	callbackURL    string

	mu     sync.Mutex
	states map[string]acpCodexOAuthState
}

type acpCodexOAuthState struct {
	BotID             string
	ChannelIdentityID string
	CodeVerifier      string
	ExpiresAt         time.Time
}

func NewACPCodexOAuthHandler(provider *providers.Service, botService *bots.Service, accountService *accounts.Service, acpWorkspace acpWorkspaceConfigProvider, callbackURL string) *ACPCodexOAuthHandler {
	return &ACPCodexOAuthHandler{
		provider:       provider,
		botService:     botService,
		accountService: accountService,
		acpWorkspace:   acpWorkspace,
		callbackURL:    strings.TrimSpace(callbackURL),
		states:         map[string]acpCodexOAuthState{},
	}
}

func (h *ACPCodexOAuthHandler) Register(e *echo.Echo) {
	e.GET("/bots/:bot_id/acp/codex/oauth/authorize", h.Authorize)
	e.GET("/bots/:bot_id/acp/codex/oauth/status", h.Status)
}

func (*ACPCodexOAuthHandler) HandlesCallbackState(state string) bool {
	return strings.HasPrefix(strings.TrimSpace(state), acpCodexOAuthStatePrefix)
}

func (h *ACPCodexOAuthHandler) Authorize(c echo.Context) error {
	botID, channelIdentityID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.provider == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "openai codex oauth provider is not configured")
	}
	if h.acpWorkspace == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "workspace manager is not configured")
	}
	if err := h.ensureManagedWorkspace(c.Request().Context(), botID); err != nil {
		return err
	}

	state, err := generateACPCodexOAuthState()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	resp, codeVerifier, err := h.provider.StartOpenAICodexACPAuthorization(c.Request().Context(), h.callbackURL, state)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if resp == nil || strings.TrimSpace(resp.AuthURL) == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "openai codex oauth authorize URL is empty")
	}

	h.mu.Lock()
	h.pruneExpiredLocked(time.Now())
	h.states[state] = acpCodexOAuthState{
		BotID:             botID,
		ChannelIdentityID: channelIdentityID,
		CodeVerifier:      codeVerifier,
		ExpiresAt:         time.Now().Add(acpCodexOAuthStateTTL),
	}
	h.mu.Unlock()

	return c.JSON(http.StatusOK, ACPCodexOAuthAuthorizeResponse{AuthURL: resp.AuthURL})
}

func (h *ACPCodexOAuthHandler) Status(c echo.Context) error {
	botID, _, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	status := ACPCodexOAuthStatus{
		Configured:  h.provider != nil && h.acpWorkspace != nil,
		CallbackURL: h.callbackURL,
	}
	if !status.Configured {
		return c.JSON(http.StatusOK, status)
	}
	if err := h.ensureManagedWorkspace(c.Request().Context(), botID); err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusBadRequest {
			status.Configured = false
			return c.JSON(http.StatusOK, status)
		}
		return err
	}

	client, err := h.acpWorkspace.MCPClient(c.Request().Context(), botID)
	if err != nil {
		return c.JSON(http.StatusOK, status)
	}
	resp, err := client.ReadFile(c.Request().Context(), path.Join(acpclient.CodexManagedConfigDir, "auth.json"), 0, 0)
	if err != nil {
		return c.JSON(http.StatusOK, status)
	}
	auth := parseCodexOAuthAuth(resp.GetContent())
	status.HasToken = auth.Valid
	status.AccountID = auth.AccountID
	return c.JSON(http.StatusOK, status)
}

func (h *ACPCodexOAuthHandler) Callback(c echo.Context) error {
	code := strings.TrimSpace(c.QueryParam("code"))
	state := strings.TrimSpace(c.QueryParam("state"))
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	if state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "state is required")
	}
	if h.provider == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "openai codex oauth provider is not configured")
	}

	oauthState, err := h.takeState(state)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if _, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, oauthState.ChannelIdentityID, oauthState.BotID, bots.PermissionWorkspaceExec); err != nil {
		return err
	}
	creds, err := h.provider.ExchangeOpenAICodexACPCode(c.Request().Context(), h.callbackURL, code, oauthState.CodeVerifier)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.writeCodexOAuthAuth(c.Request().Context(), oauthState.BotID, creds); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	page := template.Must(template.New("acp-codex-oauth-success").Parse(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Codex Connected</title>
  </head>
  <body style="font-family: sans-serif; padding: 24px;">
    <h2>Codex connected</h2>
    <p>Your ChatGPT account is now connected to this bot workspace.</p>
    <script>
      window.opener?.postMessage({ type: "memoh-acp-codex-oauth-success", botId: "{{.BotID}}" }, "*");
      window.close();
    </script>
  </body>
</html>`))
	return c.HTML(http.StatusOK, executeHTMLTemplate(page, map[string]string{"BotID": oauthState.BotID}))
}

func (h *ACPCodexOAuthHandler) requireBotAccess(c echo.Context) (string, string, error) {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", "", err
	}
	bot, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
	if err != nil {
		return "", "", err
	}
	return bot.ID, channelIdentityID, nil
}

func (h *ACPCodexOAuthHandler) ensureManagedWorkspace(ctx context.Context, botID string) error {
	// Both container and local (desktop BYOK) workspaces can host managed Codex
	// auth. For local, the credentials are written under the bot-scoped
	// CODEX_HOME (workspace .codex dir) rather than the user's real ~/.codex.
	if _, err := h.acpWorkspace.WorkspaceInfo(ctx, botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

func (h *ACPCodexOAuthHandler) writeCodexOAuthAuth(ctx context.Context, botID string, creds providers.OpenAICodexOAuthCredentials) error {
	if h.acpWorkspace == nil {
		return errors.New("workspace manager is not configured")
	}
	if err := h.ensureManagedWorkspace(ctx, botID); err != nil {
		return err
	}
	client, err := h.acpWorkspace.MCPClient(ctx, botID)
	if err != nil {
		return err
	}
	return acpclient.WriteCodexManagedConfigWithAuth(ctx, client, acpclient.CodexManagedConfig{
		Mode: acpclient.SetupModeOAuth,
		OAuth: &acpclient.CodexOAuthCredentials{
			AccessToken:  creds.AccessToken,
			IDToken:      creds.IDToken,
			RefreshToken: creds.RefreshToken,
			AccountID:    creds.AccountID,
			BaseURL:      creds.BaseURL,
			ExpiresAt:    creds.ExpiresAt,
			LastRefresh:  creds.LastRefresh,
		},
	})
}

func (h *ACPCodexOAuthHandler) takeState(state string) (acpCodexOAuthState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneExpiredLocked(time.Now())
	oauthState, ok := h.states[state]
	if !ok {
		return acpCodexOAuthState{}, errors.New("oauth state is invalid or expired")
	}
	delete(h.states, state)
	return oauthState, nil
}

func (h *ACPCodexOAuthHandler) pruneExpiredLocked(now time.Time) {
	for state, value := range h.states {
		if !value.ExpiresAt.IsZero() && now.After(value.ExpiresAt) {
			delete(h.states, state)
		}
	}
}

type codexOAuthAuthStatus struct {
	Valid     bool
	AccountID string
}

func parseCodexOAuthAuth(content string) codexOAuthAuthStatus {
	var auth struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`  //nolint:gosec // Codex auth status parses existing runtime token material.
			RefreshToken string `json:"refresh_token"` //nolint:gosec // Codex auth status parses existing runtime token material.
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(content), &auth); err != nil {
		return codexOAuthAuthStatus{}
	}
	idToken := strings.TrimSpace(auth.Tokens.IDToken)
	accessToken := strings.TrimSpace(auth.Tokens.AccessToken)
	refreshToken := strings.TrimSpace(auth.Tokens.RefreshToken)
	accountID := strings.TrimSpace(auth.Tokens.AccountID)
	return codexOAuthAuthStatus{
		Valid: strings.EqualFold(strings.TrimSpace(auth.AuthMode), "chatgpt") &&
			idToken != "" &&
			accessToken != "" &&
			refreshToken != "" &&
			accountID != "" &&
			idToken != accessToken,
		AccountID: accountID,
	}
}

func generateACPCodexOAuthState() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return acpCodexOAuthStatePrefix + hex.EncodeToString(raw[:]), nil
}
