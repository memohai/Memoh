package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/providers"
)

type ProviderOAuthHandler struct {
	service *providers.Service
}

func NewProviderOAuthHandler(service *providers.Service) *ProviderOAuthHandler {
	return &ProviderOAuthHandler{service: service}
}

func (h *ProviderOAuthHandler) Register(e *echo.Echo) {
	e.GET("/providers/:id/oauth/authorize", h.Authorize)
	e.GET("/providers/:id/oauth/status", h.Status)
	e.DELETE("/providers/:id/oauth/token", h.Revoke)
	e.GET("/auth/callback", h.Callback)
	e.GET("/providers/oauth/callback", h.Callback)
}

// Authorize godoc
// @Summary Start OAuth2 authorization for an LLM provider
// @Tags providers-oauth
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /providers/{id}/oauth/authorize [get].
func (h *ProviderOAuthHandler) Authorize(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	authURL, err := h.service.StartOAuthAuthorization(c.Request().Context(), providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"auth_url": authURL})
}

// Status godoc
// @Summary Get OAuth2 status for an LLM provider
// @Tags providers-oauth
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.OAuthStatus
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /providers/{id}/oauth/status [get].
func (h *ProviderOAuthHandler) Status(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	status, err := h.service.GetOAuthStatus(c.Request().Context(), providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, status)
}

// Revoke godoc
// @Summary Revoke stored OAuth2 tokens for an LLM provider
// @Tags providers-oauth
// @Param id path string true "Provider ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /providers/{id}/oauth/token [delete].
func (h *ProviderOAuthHandler) Revoke(c echo.Context) error {
	providerID := strings.TrimSpace(c.Param("id"))
	if providerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if err := h.service.RevokeOAuthToken(c.Request().Context(), providerID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// Callback godoc
// @Summary OAuth2 callback for LLM providers
// @Tags providers-oauth
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 200 {string} string "HTML success page"
// @Failure 400 {object} ErrorResponse
// @Router /providers/oauth/callback [get].
func (h *ProviderOAuthHandler) Callback(c echo.Context) error {
	code := strings.TrimSpace(c.QueryParam("code"))
	state := strings.TrimSpace(c.QueryParam("state"))
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	if state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "state is required")
	}
	providerID, err := h.service.HandleOAuthCallback(c.Request().Context(), state, code)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	page := template.Must(template.New("oauth-success").Parse(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>OpenAI OAuth Connected</title>
  </head>
  <body style="font-family: sans-serif; padding: 24px;">
    <h2>OpenAI OAuth connected</h2>
    <p>You can close this window and return to Memoh.</p>
    <script>
      window.opener?.postMessage({ type: "memoh-provider-oauth-success", providerId: "{{.ProviderID}}" }, "*");
      window.close();
    </script>
  </body>
</html>`))
	return c.HTML(http.StatusOK, executeHTMLTemplate(page, map[string]string{"ProviderID": providerID}))
}

func executeHTMLTemplate(tpl *template.Template, data any) string {
	var b strings.Builder
	_ = tpl.Execute(&b, data)
	return b.String()
}
