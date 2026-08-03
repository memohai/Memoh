package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	supermarketclient "github.com/memohai/memoh/internal/supermarket"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type SupermarketHandler struct {
	upstream       *supermarketclient.Client
	installer      *supermarketclient.Installer
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewSupermarketHandler(
	log *slog.Logger,
	cfg config.Config,
	pluginService *pluginspkg.Service,
	containers bridge.Provider,
	workspaces *workspace.Manager,
	botService *bots.Service,
	accountService *accounts.Service,
) *SupermarketHandler {
	upstream := supermarketclient.NewClient(cfg.Supermarket.GetBaseURL(), nil)
	return &SupermarketHandler{
		upstream:       upstream,
		installer:      supermarketclient.NewInstaller(upstream, pluginService, containers, workspaces, log),
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "supermarket")),
	}
}

func (h *SupermarketHandler) Register(e *echo.Echo) {
	g := e.Group("/supermarket")
	g.GET("/plugins", h.ListPlugins)
	g.GET("/plugins/:id", h.GetPlugin)
	g.GET("/skills", h.ListSkills)
	g.GET("/packages", h.ListPackages)
	g.GET("/registries", h.ListRegistries)
	g.GET("/registries/:registry_id/categories", h.ListRegistryCategories)
	g.GET("/registries/:registry_id/packages", h.ListRegistryPackages)
	g.GET("/registries/:registry_id/packages/:package_id", h.GetRegistryPackage)
	g.GET("/registries/:registry_id/packages/:package_id/skills/:skill_id", h.GetRegistrySkill)
	g.GET("/artifacts/icon/:digest", h.GetRegistrySkillIcon)

	ig := e.Group("/bots/:bot_id/supermarket")
	ig.POST("/install-plugin", h.InstallPlugin)
	ig.POST("/install-package", h.InstallPackage)
}

func (h *SupermarketHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := c.Param("bot_id")
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

// proxy forwards a GET request to the supermarket and streams the JSON response back.
func (h *SupermarketHandler) proxy(c echo.Context, upstreamPath string) error {
	requestPath := upstreamPath
	if qs := c.QueryString(); qs != "" {
		requestPath += "?" + qs
	}
	resp, err := h.upstream.Get(c.Request().Context(), requestPath, "application/json")
	if err != nil {
		h.logger.Error("supermarket proxy failed", slog.String("path", requestPath), slog.Any("error", err))
		return echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c.Response().WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Response(), resp.Body)
	return nil
}

// ListPlugins godoc
// @Summary List plugins from supermarket
// @Tags supermarket
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {object} SupermarketPluginListResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/plugins [get].
func (h *SupermarketHandler) ListPlugins(c echo.Context) error {
	return h.proxy(c, "/api/plugins")
}

// GetPlugin godoc
// @Summary Get plugin detail from supermarket
// @Tags supermarket
// @Param id path string true "Plugin ID"
// @Success 200 {object} SupermarketPluginEntry
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/plugins/{id} [get].
func (h *SupermarketHandler) GetPlugin(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if !skillset.IsValidName(id) {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin id is invalid")
	}
	return h.proxy(c, "/api/plugins/"+url.PathEscape(id))
}

// --- Install endpoints ---

// InstallPluginRequest is the request body for installing a plugin from supermarket.
type InstallPluginRequest struct {
	PluginID                      string            `json:"plugin_id" validate:"required"`
	ReleaseRevision               string            `json:"release_revision" validate:"required"`
	ExpectedInstalledRevision     *string           `json:"expected_installed_revision" validate:"required" extensions:"x-nullable"`
	ExpectedInstallationUpdatedAt *time.Time        `json:"expected_installation_updated_at" validate:"required" extensions:"x-nullable"`
	Variables                     map[string]string `json:"variables,omitempty"`

	expectedInstalledRevisionSet     bool
	expectedInstallationUpdatedAtSet bool
}

func (r *InstallPluginRequest) UnmarshalJSON(data []byte) error {
	type request InstallPluginRequest
	var decoded request
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = InstallPluginRequest(decoded)
	_, r.expectedInstalledRevisionSet = fields["expected_installed_revision"]
	_, r.expectedInstallationUpdatedAtSet = fields["expected_installation_updated_at"]
	return nil
}

// InstallPackageRequest installs one immutable Package revision.
type InstallPackageRequest struct {
	RegistryID        string `json:"registry_id" validate:"required"`
	PackageID         string `json:"package_id" validate:"required"`
	Revision          string `json:"revision" validate:"required"`
	WorkspaceTargetID string `json:"workspace_target_id,omitempty"`
}

// InstallPlugin godoc
// @Summary Install plugin from supermarket to bot
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallPluginRequest true "Install plugin request"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /bots/{bot_id}/supermarket/install-plugin [post].
func (h *SupermarketHandler) InstallPlugin(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallPluginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.PluginID = strings.TrimSpace(req.PluginID)
	if req.PluginID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin_id is required")
	}
	if !skillset.IsValidPluginID(req.PluginID) {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin_id is invalid")
	}
	req.ReleaseRevision = strings.TrimSpace(req.ReleaseRevision)
	if !supermarketclient.IsCanonicalSHA256(req.ReleaseRevision) {
		return echo.NewHTTPError(http.StatusBadRequest, "release_revision is invalid")
	}
	if !req.expectedInstalledRevisionSet {
		return echo.NewHTTPError(http.StatusBadRequest, "expected_installed_revision is required")
	}
	if !req.expectedInstallationUpdatedAtSet {
		return echo.NewHTTPError(http.StatusBadRequest, "expected_installation_updated_at is required")
	}
	if req.ExpectedInstalledRevision != nil {
		expectedRevision := strings.TrimSpace(*req.ExpectedInstalledRevision)
		if !supermarketclient.IsCanonicalSHA256(expectedRevision) {
			return echo.NewHTTPError(http.StatusBadRequest, "expected_installed_revision is invalid")
		}
		req.ExpectedInstalledRevision = &expectedRevision
	}
	if (req.ExpectedInstalledRevision == nil) != (req.ExpectedInstallationUpdatedAt == nil) {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"expected Plugin revision and installation timestamp must both be null or both be set",
		)
	}

	if h.installer == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "supermarket installer is not configured")
	}
	installation, err := h.installer.InstallPlugin(c.Request().Context(), botID, supermarketclient.InstallPluginRequest{
		PluginID: req.PluginID, ReleaseRevision: req.ReleaseRevision,
		ExpectedInstalledRevision: req.ExpectedInstalledRevision,
		ExpectedInstallationTime:  req.ExpectedInstallationUpdatedAt,
		Variables:                 req.Variables,
	})
	if err != nil {
		return h.installerHTTPError(err)
	}
	return c.JSON(http.StatusOK, installation)
}

// InstallPackage godoc
// @Summary Install an immutable Skill Package release to a bot workspace
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallPackageRequest true "Install Package request"
// @Success 200 {object} InstallRegistryPackageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} apperror.Problem
// @Failure 409 {object} apperror.Problem
// @Failure 500 {object} apperror.Problem
// @Failure 502 {object} apperror.Problem
// @Router /bots/{bot_id}/supermarket/install-package [post].
func (h *SupermarketHandler) InstallPackage(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallPackageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.installer == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "supermarket installer is not configured")
	}
	result, err := h.installer.InstallPackage(c.Request().Context(), botID, supermarketclient.InstallPackageRequest{
		RegistryID: req.RegistryID, PackageID: req.PackageID, Revision: req.Revision,
		WorkspaceTargetID: req.WorkspaceTargetID,
	})
	if err != nil {
		return h.installerHTTPError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *SupermarketHandler) installerHTTPError(err error) error {
	var targetErr *supermarketclient.WorkspaceTargetError
	if errors.As(err, &targetErr) {
		return workspaceTargetHTTPError(h.logger, targetErr.Err)
	}
	var statusErr *supermarketclient.StatusError
	if errors.As(err, &statusErr) {
		return echo.NewHTTPError(statusErr.Status, statusErr.Error())
	}
	return err
}

// --- Supermarket upstream types (for swagger) ---

type SupermarketAuthor = supermarketclient.Author

type SupermarketPluginArtifact = supermarketclient.PluginArtifact

type SupermarketPluginResolvedPackage = supermarketclient.PluginResolvedPackage

type SupermarketPluginRelease = supermarketclient.PluginRelease

type SupermarketImmutablePluginRelease = supermarketclient.ImmutablePluginRelease

type SupermarketPluginEntry = supermarketclient.PluginEntry

type SupermarketPluginListResponse = supermarketclient.PluginListResponse
