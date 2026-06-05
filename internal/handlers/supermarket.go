package handlers

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type SupermarketHandler struct {
	baseURL        string
	httpClient     *http.Client
	pluginService  *pluginspkg.Service
	containers     bridge.Provider
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewSupermarketHandler(
	log *slog.Logger,
	cfg config.Config,
	pluginService *pluginspkg.Service,
	containers bridge.Provider,
	botService *bots.Service,
	accountService *accounts.Service,
) *SupermarketHandler {
	return &SupermarketHandler{
		baseURL:        cfg.Supermarket.GetBaseURL(),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		pluginService:  pluginService,
		containers:     containers,
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
	g.GET("/skills/:id", h.GetSkill)
	g.GET("/tags", h.ListTags)

	ig := e.Group("/bots/:bot_id/supermarket")
	ig.POST("/install-plugin", h.InstallPlugin)
	ig.POST("/install-skill", h.InstallSkill)
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
	url := h.baseURL + upstreamPath
	if qs := c.QueryString(); qs != "" {
		url += "?" + qs
	}

	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, url, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Error("supermarket proxy failed", slog.String("url", url), slog.Any("error", err))
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
// @Success 200 {object} plugins.Manifest
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/plugins/{id} [get].
func (h *SupermarketHandler) GetPlugin(c echo.Context) error {
	id := c.Param("id")
	return h.proxy(c, "/api/plugins/"+id)
}

// ListSkills godoc
// @Summary List skills from supermarket
// @Tags supermarket
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {object} SupermarketSkillListResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/skills [get].
func (h *SupermarketHandler) ListSkills(c echo.Context) error {
	return h.proxy(c, "/api/skills")
}

// GetSkill godoc
// @Summary Get skill detail from supermarket
// @Tags supermarket
// @Param id path string true "Skill ID"
// @Success 200 {object} SupermarketSkillEntry
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/skills/{id} [get].
func (h *SupermarketHandler) GetSkill(c echo.Context) error {
	id := c.Param("id")
	return h.proxy(c, "/api/skills/"+id)
}

// ListTags godoc
// @Summary List all tags from supermarket
// @Tags supermarket
// @Success 200 {object} SupermarketTagsResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/tags [get].
func (h *SupermarketHandler) ListTags(c echo.Context) error {
	return h.proxy(c, "/api/tags")
}

// --- Install endpoints ---

// InstallPluginRequest is the request body for installing a plugin from supermarket.
type InstallPluginRequest struct {
	PluginID  string            `json:"plugin_id"`
	Variables map[string]string `json:"variables,omitempty"`
}

// InstallSkillRequest is the request body for installing a skill from supermarket.
type InstallSkillRequest struct {
	SkillID string `json:"skill_id"`
}

// InstallPlugin godoc
// @Summary Install plugin from supermarket to bot
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallPluginRequest true "Install plugin request"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
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
	if strings.TrimSpace(req.PluginID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin_id is required")
	}

	manifest, err := h.fetchPluginEntry(c, req.PluginID)
	if err != nil {
		return err
	}

	installation, err := h.pluginService.Install(c.Request().Context(), botID, pluginspkg.InstallRequest{
		Manifest:  manifest,
		Variables: req.Variables,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, installation)
}

// InstallSkill godoc
// @Summary Install skill from supermarket to bot container
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallSkillRequest true "Install skill request"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /bots/{bot_id}/supermarket/install-skill [post].
func (h *SupermarketHandler) InstallSkill(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallSkillRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	skillID := strings.TrimSpace(req.SkillID)
	if skillID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "skill_id is required")
	}
	if strings.Contains(skillID, "..") || strings.Contains(skillID, "/") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill_id")
	}

	ctx := c.Request().Context()
	client, err := h.containers.MCPClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	downloadURL := h.baseURL + "/api/skills/" + skillID + "/download"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	resp, err := h.httpClient.Do(httpReq) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Error("supermarket skill download failed", slog.String("url", downloadURL), slog.Any("error", err))
		return echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("skill %q not found in supermarket", skillID))
	}
	if resp.StatusCode != http.StatusOK {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned status %d", resp.StatusCode))
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "invalid gzip response from supermarket")
	}
	defer func() { _ = gz.Close() }()

	skillDir := path.Join(skillset.ManagedDir(), skillID)
	if err := client.Mkdir(ctx, skillDir); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("mkdir failed: %v", err))
	}

	tr := tar.NewReader(gz)
	filesWritten := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("invalid tar: %v", err))
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		relativePath := strings.TrimPrefix(hdr.Name, skillID+"/")
		if relativePath == "" || strings.Contains(relativePath, "..") {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("read tar entry failed: %v", err))
		}

		filePath := path.Join(skillDir, relativePath)
		dir := path.Dir(filePath)
		if dir != skillDir {
			_ = client.Mkdir(ctx, dir)
		}

		if err := client.WriteFile(ctx, filePath, content); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("write file %s failed: %v", relativePath, err))
		}
		filesWritten++
	}

	if filesWritten == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "skill archive was empty")
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "files_written": filesWritten})
}

// --- Supermarket upstream types (for swagger) ---

type SupermarketAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SupermarketPluginListResponse struct {
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
	Data  []pluginspkg.Manifest `json:"data"`
}

type SupermarketSkillMetadata struct {
	Author   SupermarketAuthor `json:"author"`
	Tags     []string          `json:"tags,omitempty"`
	Homepage string            `json:"homepage,omitempty"`
}

type SupermarketSkillEntry struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Metadata    SupermarketSkillMetadata `json:"metadata"`
	Content     string                   `json:"content"`
	Files       []string                 `json:"files"`
}

type SupermarketSkillListResponse struct {
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
	Data  []SupermarketSkillEntry `json:"data"`
}

type SupermarketTagsResponse struct {
	Tags []string `json:"tags"`
}

// --- Internal helpers ---

func (h *SupermarketHandler) fetchPluginEntry(c echo.Context, pluginID string) (pluginspkg.Manifest, error) {
	url := h.baseURL + "/api/plugins/" + pluginID
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, url, nil)
	if err != nil {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Error("supermarket plugin fetch failed", slog.String("url", url), slog.Any("error", err))
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("plugin %q not found in supermarket", pluginID))
	}
	if resp.StatusCode != http.StatusOK {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned status %d", resp.StatusCode))
	}

	var manifest pluginspkg.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusBadGateway, "invalid JSON from supermarket")
	}
	return manifest, nil
}
