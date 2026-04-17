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
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
)

type SupermarketHandler struct {
	baseURL        string
	httpClient     *http.Client
	mcpService     *mcp.ConnectionService
	manager        *workspace.Manager
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewSupermarketHandler(
	log *slog.Logger,
	cfg config.Config,
	mcpService *mcp.ConnectionService,
	manager *workspace.Manager,
	botService *bots.Service,
	accountService *accounts.Service,
) *SupermarketHandler {
	return &SupermarketHandler{
		baseURL:        cfg.Supermarket.GetBaseURL(),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		mcpService:     mcpService,
		manager:        manager,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "supermarket")),
	}
}

func (h *SupermarketHandler) Register(e *echo.Echo) {
	g := e.Group("/supermarket")
	g.GET("/mcps", h.ListMcps)
	g.GET("/mcps/:id", h.GetMcp)
	g.GET("/skills", h.ListSkills)
	g.GET("/skills/:id", h.GetSkill)
	g.GET("/tags", h.ListTags)

	ig := e.Group("/bots/:bot_id/supermarket")
	ig.POST("/install-mcp", h.InstallMcp)
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

// ListMcps godoc
// @Summary List MCPs from supermarket
// @Tags supermarket
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param transport query string false "Filter by transport type"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {object} SupermarketMcpListResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/mcps [get].
func (h *SupermarketHandler) ListMcps(c echo.Context) error {
	return h.proxy(c, "/api/mcps")
}

// GetMcp godoc
// @Summary Get MCP detail from supermarket
// @Tags supermarket
// @Param id path string true "MCP ID"
// @Success 200 {object} SupermarketMcpEntry
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/mcps/{id} [get].
func (h *SupermarketHandler) GetMcp(c echo.Context) error {
	id := c.Param("id")
	return h.proxy(c, "/api/mcps/"+id)
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

// InstallMcpRequest is the request body for installing an MCP from supermarket.
type InstallMcpRequest struct {
	McpID string            `json:"mcp_id"`
	Env   map[string]string `json:"env,omitempty"`
}

// InstallSkillRequest is the request body for installing a skill from supermarket.
type InstallSkillRequest struct {
	SkillID string `json:"skill_id"`
}

// InstallMcp godoc
// @Summary Install MCP from supermarket to bot
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallMcpRequest true "Install MCP request"
// @Success 200 {object} mcp.Connection
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /bots/{bot_id}/supermarket/install-mcp [post].
func (h *SupermarketHandler) InstallMcp(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallMcpRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.McpID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "mcp_id is required")
	}

	entry, err := h.fetchMcpEntry(c, req.McpID)
	if err != nil {
		return err
	}

	upsert := h.mcpEntryToUpsert(entry, req.Env)
	conn, err := h.mcpService.Create(c.Request().Context(), botID, upsert)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, conn)
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
	client, err := h.manager.MCPClient(ctx, botID)
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

type SupermarketConfigVar struct {
	Key          string `json:"key"`
	Description  string `json:"description"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

type SupermarketMcpEntry struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Author      SupermarketAuthor      `json:"author"`
	Transport   string                 `json:"transport"`
	Icon        string                 `json:"icon,omitempty"`
	Homepage    string                 `json:"homepage,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Command     string                 `json:"command,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Headers     []SupermarketConfigVar `json:"headers,omitempty"`
	Env         []SupermarketConfigVar `json:"env,omitempty"`
}

type SupermarketMcpListResponse struct {
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
	Data  []SupermarketMcpEntry `json:"data"`
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

func (h *SupermarketHandler) fetchMcpEntry(c echo.Context, mcpID string) (SupermarketMcpEntry, error) {
	url := h.baseURL + "/api/mcps/" + mcpID
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, url, nil)
	if err != nil {
		return SupermarketMcpEntry{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Error("supermarket fetch failed", slog.String("url", url), slog.Any("error", err))
		return SupermarketMcpEntry{}, echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return SupermarketMcpEntry{}, echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("MCP %q not found in supermarket", mcpID))
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketMcpEntry{}, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned status %d", resp.StatusCode))
	}

	var entry SupermarketMcpEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return SupermarketMcpEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid JSON from supermarket")
	}
	return entry, nil
}

func (*SupermarketHandler) mcpEntryToUpsert(entry SupermarketMcpEntry, envOverrides map[string]string) mcp.UpsertRequest {
	headers := make(map[string]string, len(entry.Headers))
	for _, hdr := range entry.Headers {
		headers[hdr.Key] = hdr.DefaultValue
	}

	env := make(map[string]string, len(entry.Env))
	for _, e := range entry.Env {
		if override, ok := envOverrides[e.Key]; ok {
			env[e.Key] = override
		} else {
			env[e.Key] = e.DefaultValue
		}
	}

	return mcp.UpsertRequest{
		Name:      entry.Name,
		Command:   entry.Command,
		Args:      entry.Args,
		URL:       entry.URL,
		Headers:   headers,
		Env:       env,
		Transport: entry.Transport,
	}
}
