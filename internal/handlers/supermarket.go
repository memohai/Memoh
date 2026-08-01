package handlers

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type SupermarketHandler struct {
	baseURL        string
	httpClient     *http.Client
	pluginService  pluginInstaller
	containers     bridge.Provider
	workspaces     *workspace.Manager
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

type pluginInstaller interface {
	botMutationCoordinator
	Install(ctx context.Context, botID string, req pluginspkg.InstallRequest) (pluginspkg.Installation, error)
	GarbageCollectRegistrySkills(ctx context.Context, botID string, references []pluginspkg.SkillReference)
}

type pluginBundleWriter interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Mkdir(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	WriteFile(ctx context.Context, path string, content []byte) error
}

type pluginInstallScriptExecutor interface {
	ExecWithEnv(ctx context.Context, command, workDir string, timeout int32, env []string) (*bridge.ExecResult, error)
}

type pluginAssetInstallResult struct {
	OK           bool   `json:"ok"`
	FilesWritten int    `json:"files_written"`
	Error        string `json:"error,omitempty"`
}

type pluginBundleInstallResult struct {
	Hooks   pluginAssetInstallResult
	Scripts pluginAssetInstallResult
}

type pluginInstallScriptsResult struct {
	OK          bool                         `json:"ok"`
	CommandsRun int                          `json:"commands_run"`
	Results     []pluginInstallCommandResult `json:"results,omitempty"`
	Error       string                       `json:"error,omitempty"`
}

type pluginSkillsInstallResult struct {
	OK     bool                       `json:"ok"`
	Skills []pluginSkillInstallResult `json:"skills,omitempty"`
	Error  string                     `json:"error,omitempty"`
}

type pluginSkillInstallResult struct {
	RegistryID     string `json:"registry_id"`
	PackageID      string `json:"package_id"`
	SkillID        string `json:"skill_id"`
	InstallID      string `json:"install_id,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
	FilesWritten   int    `json:"files_written,omitempty"`
	Error          string `json:"error,omitempty"`
}

type pluginInstallCommandResult struct {
	Command  string `json:"command"`
	ExitCode int32  `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

const (
	pluginInstallScriptTimeoutSeconds int32 = 10 * 60
	pluginInstallScriptOutputLimit          = 64 * 1024
	maxPluginBundleCompressedBytes          = 25 * 1024 * 1024
	maxPluginBundleUncompressedBytes        = 10 * 1024 * 1024
	maxPluginBundleFileBytes                = 2 * 1024 * 1024
	maxPluginBundleFiles                    = 1_000
	maxPluginBundleEntries                  = 2_000
)

func NewSupermarketHandler(
	log *slog.Logger,
	cfg config.Config,
	pluginService *pluginspkg.Service,
	containers bridge.Provider,
	workspaces *workspace.Manager,
	botService *bots.Service,
	accountService *accounts.Service,
) *SupermarketHandler {
	return &SupermarketHandler{
		baseURL:        cfg.Supermarket.GetBaseURL(),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		pluginService:  pluginService,
		containers:     containers,
		workspaces:     workspaces,
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
	g.GET("/tags", h.ListTags)
	g.GET("/registries", h.ListRegistries)
	g.GET("/registries/:registry_id/categories", h.ListRegistryCategories)
	g.GET("/registries/:registry_id/packages/:package_id/skills/:skill_id", h.GetRegistrySkill)
	g.GET("/artifacts/icon/:digest", h.GetRegistrySkillIcon)

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
	id := strings.TrimSpace(c.Param("id"))
	if !skillset.IsValidName(id) {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin id is invalid")
	}
	return h.proxy(c, "/api/plugins/"+url.PathEscape(id))
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
	RegistryID        string `json:"registry_id"`
	PackageID         string `json:"package_id"`
	SkillID           string `json:"skill_id"`
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
	if !skillset.IsValidName(req.PluginID) {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin_id is invalid")
	}

	manifest, err := h.fetchPluginEntry(c, req.PluginID)
	if err != nil {
		return err
	}
	manifest = pluginspkg.NormalizeManifest(manifest)
	if manifest.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin id is required")
	}
	if err := pluginspkg.ValidateSkillReferences(manifest.Skills); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	target, err := h.resolvePluginInstallTarget(ctx, botID)
	if err != nil {
		return err
	}
	var (
		installation  pluginspkg.Installation
		skillsResult  pluginSkillsInstallResult
		bundleResult  pluginBundleInstallResult
		scriptsResult pluginInstallScriptsResult
	)
	if err := withBotMutation(ctx, botID, h.pluginService, func(mutationCtx context.Context) error {
		var (
			skillArtifacts map[string]pluginspkg.SkillArtifactMetadata
			installErr     error
		)
		skillsResult, skillArtifacts, installErr = h.installPluginSkills(
			mutationCtx,
			target.Client,
			target.Info.OS,
			manifest.Skills,
		)
		if installErr != nil {
			h.cleanupFailedPluginSkills(mutationCtx, botID, manifest.Skills)
			return installErr
		}
		bundleResult, installErr = h.installPluginBundle(
			mutationCtx, target.Client, req.PluginID, manifest.ID, manifest.Skills,
		)
		if installErr != nil {
			h.cleanupFailedPluginSkills(mutationCtx, botID, manifest.Skills)
			return echo.NewHTTPError(http.StatusBadGateway, installErr.Error())
		}
		scriptsResult, installErr = runPluginInstallScripts(
			mutationCtx, target.Client, botID, manifest.ID, manifest.Install,
		)
		if installErr != nil {
			h.cleanupFailedPluginSkills(mutationCtx, botID, manifest.Skills)
			return echo.NewHTTPError(http.StatusBadGateway, installErr.Error())
		}
		installation, installErr = h.pluginService.Install(mutationCtx, botID, pluginspkg.InstallRequest{
			Manifest:       manifest,
			Variables:      req.Variables,
			SkillArtifacts: skillArtifacts,
		})
		if installErr != nil {
			h.cleanupFailedPluginSkills(mutationCtx, botID, manifest.Skills)
			return echo.NewHTTPError(http.StatusBadRequest, installErr.Error())
		}
		return nil
	}); err != nil {
		return err
	}
	installation = withPluginBundleInstallMetadata(installation, bundleResult, nil)
	installation = withPluginInstallScriptsMetadata(installation, scriptsResult, nil)
	installation = withPluginSkillsInstallMetadata(installation, skillsResult, nil)
	return c.JSON(http.StatusOK, installation)
}

// InstallSkill godoc
// @Summary Install skill from supermarket to bot workspace
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallSkillRequest true "Install skill request"
// @Success 200 {object} InstallRegistrySkillResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} apperror.Problem
// @Failure 409 {object} apperror.Problem
// @Failure 500 {object} apperror.Problem
// @Failure 502 {object} apperror.Problem
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
	var result InstallRegistrySkillResponse
	if err := withBotMutation(
		c.Request().Context(),
		botID,
		h.pluginService,
		func(mutationCtx context.Context) error {
			var installErr error
			result, installErr = h.installRegistrySkill(mutationCtx, botID, req)
			return installErr
		},
	); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
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

type SupermarketTagsResponse struct {
	Tags []string `json:"tags"`
}

// --- Internal helpers ---

func (h *SupermarketHandler) fetchPluginEntry(c echo.Context, pluginID string) (pluginspkg.Manifest, error) {
	pluginID = strings.TrimSpace(pluginID)
	if !skillset.IsValidName(pluginID) {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusBadRequest, "plugin id is invalid")
	}
	endpoint := h.baseURL + "/api/plugins/" + url.PathEscape(pluginID)
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return pluginspkg.Manifest{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Error("supermarket plugin fetch failed", slog.String("url", endpoint), slog.Any("error", err))
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

func (h *SupermarketHandler) resolvePluginInstallTarget(ctx context.Context, botID string) (workspace.ResolvedWorkspaceTarget, error) {
	if h.workspaces != nil {
		target, err := h.workspaces.ResolveWorkspaceTarget(ctx, botID, "")
		if err != nil {
			return workspace.ResolvedWorkspaceTarget{}, workspaceTargetHTTPError(h.logger, err)
		}
		return target, nil
	}
	if h.containers == nil {
		return workspace.ResolvedWorkspaceTarget{}, errors.New("workspace runtime provider is not configured")
	}
	client, err := h.containers.MCPClient(ctx, botID)
	if err != nil {
		return workspace.ResolvedWorkspaceTarget{}, fmt.Errorf("workspace is not reachable: %w", err)
	}
	return workspace.ResolvedWorkspaceTarget{Client: client}, nil
}

func (h *SupermarketHandler) installPluginSkills(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS string,
	references []pluginspkg.SkillReference,
) (pluginSkillsInstallResult, map[string]pluginspkg.SkillArtifactMetadata, error) {
	result := pluginSkillsInstallResult{OK: true, Skills: make([]pluginSkillInstallResult, 0, len(references))}
	artifacts := make(map[string]pluginspkg.SkillArtifactMetadata, len(references))
	for _, reference := range references {
		item := pluginSkillInstallResult{
			RegistryID: reference.RegistryID,
			PackageID:  reference.PackageID,
			SkillID:    reference.SkillID,
		}
		installed, err := h.installRegistrySkillArtifact(
			ctx,
			client,
			workspaceOS,
			false,
			reference.RegistryID,
			reference.PackageID,
			reference.SkillID,
		)
		if err != nil {
			item.Error = err.Error()
			result.OK = false
			result.Error = err.Error()
			result.Skills = append(result.Skills, item)
			return result, nil, err
		}
		item.InstallID = installed.Skill.InstallID
		item.ArtifactDigest = installed.Skill.Artifact.Digest
		item.FilesWritten = installed.FilesWritten
		result.Skills = append(result.Skills, item)
		artifacts[pluginspkg.SkillReferenceIdentity(reference)] = pluginspkg.SkillArtifactMetadata{
			InstallID:      item.InstallID,
			ArtifactDigest: item.ArtifactDigest,
			FilesWritten:   item.FilesWritten,
		}
	}
	return result, artifacts, nil
}

func (h *SupermarketHandler) cleanupFailedPluginSkills(ctx context.Context, botID string, references []pluginspkg.SkillReference) {
	if h.pluginService == nil || len(references) == 0 {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	h.pluginService.GarbageCollectRegistrySkills(cleanupCtx, botID, references)
}

func (h *SupermarketHandler) installPluginBundle(
	ctx context.Context,
	client pluginBundleWriter,
	downloadPluginID, targetPluginID string,
	expectedSkills []pluginspkg.SkillReference,
) (pluginBundleInstallResult, error) {
	if client == nil {
		return pluginBundleInstallResult{}, errors.New("workspace is not reachable")
	}

	downloadURL := h.baseURL + "/api/plugins/" + url.PathEscape(strings.TrimSpace(downloadPluginID)) + "/download"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}

	resp, err := h.httpClient.Do(httpReq) //nolint:gosec // URL constructed from trusted config
	if err != nil {
		h.logger.Warn("supermarket plugin bundle download failed", slog.String("url", downloadURL), slog.Any("error", err))
		return pluginBundleInstallResult{}, fmt.Errorf("supermarket unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return pluginBundleInstallResult{}, errors.New("plugin bundle was not found in supermarket")
	}
	if resp.StatusCode != http.StatusOK {
		return pluginBundleInstallResult{}, fmt.Errorf("supermarket returned status %d", resp.StatusCode)
	}

	if resp.ContentLength > maxPluginBundleCompressedBytes {
		return pluginBundleInstallResult{}, errors.New("plugin bundle exceeds the compressed size limit")
	}
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxPluginBundleCompressedBytes+1))
	if err != nil {
		return pluginBundleInstallResult{}, fmt.Errorf("invalid gzip response from supermarket: %w", err)
	}
	defer func() { _ = gz.Close() }()

	archive, err := readPluginBundleArchive(downloadPluginID, targetPluginID, gz)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	if !samePluginSkillReferences(archive.skillReferences, expectedSkills) {
		return pluginBundleInstallResult{}, errors.New("plugin bundle Skill references do not match the catalog manifest")
	}
	return publishPluginBundleArchive(ctx, client, targetPluginID, archive)
}

func runPluginInstallScripts(ctx context.Context, client *bridge.Client, botID, pluginID string, commands pluginspkg.InstallCommands) (pluginInstallScriptsResult, error) {
	result := newPluginInstallScriptsResult()
	if len(commands) == 0 {
		return result, nil
	}
	if client == nil {
		return result, errors.New("workspace is not reachable")
	}
	return runPluginInstallCommands(ctx, client, botID, pluginID, []string(commands))
}

func runPluginInstallCommands(ctx context.Context, executor pluginInstallScriptExecutor, botID, pluginID string, commands []string) (pluginInstallScriptsResult, error) {
	result := newPluginInstallScriptsResult()
	if executor == nil {
		return result, errors.New("plugin install script executor is not configured")
	}
	pluginRoot, err := skillset.PluginDirForID(pluginID)
	if err != nil {
		return result, err
	}
	env := []string{
		"MEMOH_PLUGIN_ID=" + pluginID,
		"MEMOH_PLUGIN_DIR=" + pluginRoot,
		"MEMOH_BOT_ID=" + botID,
	}
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		commandResult := pluginInstallCommandResult{Command: command}
		execResult, execErr := executor.ExecWithEnv(ctx, command, pluginRoot, pluginInstallScriptTimeoutSeconds, env)
		result.CommandsRun++
		if execResult != nil {
			commandResult.ExitCode = execResult.ExitCode
			commandResult.Stdout = truncatePluginInstallOutput(execResult.Stdout)
			commandResult.Stderr = truncatePluginInstallOutput(execResult.Stderr)
		}
		if execErr != nil {
			commandResult.Error = execErr.Error()
			result.Results = append(result.Results, commandResult)
			result.OK = false
			result.Error = execErr.Error()
			return result, fmt.Errorf("plugin install command %q failed: %w", command, execErr)
		}
		if execResult != nil && execResult.ExitCode != 0 {
			commandResult.Error = fmt.Sprintf("command exited with code %d", execResult.ExitCode)
			result.Results = append(result.Results, commandResult)
			result.OK = false
			result.Error = commandResult.Error
			return result, fmt.Errorf("plugin install command %q exited with code %d", command, execResult.ExitCode)
		}
		result.Results = append(result.Results, commandResult)
	}
	return result, nil
}

const (
	pluginArchiveKindManifest = "manifest"
	pluginArchiveKindHooks    = "hooks"
	pluginArchiveKindScripts  = "scripts"
)

type pluginArchiveEntry struct {
	kind         string
	root         string
	relativePath string
}

type pluginBundleArchiveFile struct {
	entry   pluginArchiveEntry
	content []byte
}

type pluginBundleArchive struct {
	files           []pluginBundleArchiveFile
	skillReferences []pluginspkg.SkillReference
}

func extractPluginBundleArchive(ctx context.Context, client pluginBundleWriter, archivePluginID, targetPluginID string, r io.Reader) (pluginBundleInstallResult, error) {
	archive, err := readPluginBundleArchive(archivePluginID, targetPluginID, r)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	return publishPluginBundleArchive(ctx, client, targetPluginID, archive)
}

func readPluginBundleArchive(archivePluginID, targetPluginID string, r io.Reader) (pluginBundleArchive, error) {
	if !skillset.IsValidName(strings.TrimSpace(archivePluginID)) {
		return pluginBundleArchive{}, errors.New("plugin bundle archive id is invalid")
	}
	if _, err := skillset.PluginDirForID(targetPluginID); err != nil {
		return pluginBundleArchive{}, errors.New("plugin bundle target id is invalid")
	}

	archive := pluginBundleArchive{files: make([]pluginBundleArchiveFile, 0)}
	seen := make(map[string]bool)
	var totalSize int64
	totalEntries := 0
	regularFiles := 0
	hasManifest := false
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return pluginBundleArchive{}, fmt.Errorf("invalid plugin bundle tar: %w", err)
		}
		totalEntries++
		if totalEntries > maxPluginBundleEntries {
			return pluginBundleArchive{}, errors.New("plugin bundle contains too many entries")
		}

		name, err := normalizePluginBundleArchivePath(archivePluginID, hdr.Name)
		if err != nil {
			return pluginBundleArchive{}, err
		}
		if name == "" {
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
			return pluginBundleArchive{}, errors.New("plugin bundle archive root must be a directory")
		}
		isFile := hdr.Typeflag == tar.TypeReg || hdr.Typeflag == 0
		if err := recordPluginBundleArchivePath(seen, name, isFile); err != nil {
			return pluginBundleArchive{}, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, 0:
		case tar.TypeSymlink, tar.TypeLink:
			return pluginBundleArchive{}, fmt.Errorf("plugin bundle contains a link at %q", hdr.Name)
		default:
			return pluginBundleArchive{}, fmt.Errorf("plugin bundle contains unsupported entry %q", hdr.Name)
		}
		regularFiles++
		if regularFiles > maxPluginBundleFiles {
			return pluginBundleArchive{}, errors.New("plugin bundle exceeds the file limit")
		}
		if hdr.Size < 0 || hdr.Size > maxPluginBundleFileBytes {
			return pluginBundleArchive{}, fmt.Errorf("plugin bundle file %q exceeds the file size limit", hdr.Name)
		}
		if hdr.Size > maxPluginBundleUncompressedBytes-totalSize {
			return pluginBundleArchive{}, errors.New("plugin bundle exceeds the uncompressed size limit")
		}
		content, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil || int64(len(content)) != hdr.Size {
			return pluginBundleArchive{}, fmt.Errorf("plugin bundle file %q is truncated", hdr.Name)
		}
		totalSize += int64(len(content))

		entry, ok, err := pluginBundleArchiveEntry(archivePluginID, targetPluginID, hdr.Name)
		if err != nil {
			return pluginBundleArchive{}, err
		}
		if !ok {
			continue
		}
		if entry.kind == pluginArchiveKindManifest {
			references, err := validatePluginBundleManifest(content, targetPluginID)
			if err != nil {
				return pluginBundleArchive{}, err
			}
			archive.skillReferences = references
			hasManifest = true
			continue
		}
		archive.files = append(archive.files, pluginBundleArchiveFile{entry: entry, content: content})
	}
	if !hasManifest {
		return pluginBundleArchive{}, errors.New("plugin bundle does not contain a root plugin.yaml")
	}
	return archive, nil
}

func publishPluginBundleArchive(
	ctx context.Context,
	client pluginBundleWriter,
	targetPluginID string,
	archive pluginBundleArchive,
) (pluginBundleInstallResult, error) {
	result := newPluginBundleInstallResult()
	if client == nil {
		return pluginBundleInstallResult{}, errors.New("workspace is not reachable")
	}
	pluginRoot, err := skillset.PluginDirForID(targetPluginID)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	stagingRoot := path.Join(skillset.PluginDirPath, ".staging")
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	tempDir := path.Join(stagingRoot, "install-"+targetPluginID+"-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+targetPluginID+"-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() { _ = client.DeleteFile(context.WithoutCancel(ctx), tempDir, true) }()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return pluginBundleInstallResult{}, fmt.Errorf("create plugin staging root: %w", err)
	}
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return pluginBundleInstallResult{}, fmt.Errorf("create temporary plugin directory: %w", err)
	}

	for _, file := range archive.files {
		relativePath := file.entry.relativePath
		if file.entry.kind == pluginArchiveKindScripts {
			relativePath = path.Join("scripts", relativePath)
		}
		filePath := path.Clean(path.Join(tempDir, relativePath))
		if filePath == tempDir || !strings.HasPrefix(filePath, tempDir+"/") {
			return pluginBundleInstallResult{}, fmt.Errorf("plugin bundle path escapes staging root: %s", relativePath)
		}
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return pluginBundleInstallResult{}, fmt.Errorf("create plugin bundle directory %s: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return pluginBundleInstallResult{}, fmt.Errorf("write plugin bundle file %s: %w", relativePath, err)
		}
		switch file.entry.kind {
		case pluginArchiveKindHooks:
			result.Hooks.FilesWritten++
		case pluginArchiveKindScripts:
			result.Scripts.FilesWritten++
		}
	}

	targetExists := true
	if err := client.Rename(ctx, pluginRoot, backupDir); err != nil {
		if errors.Is(err, bridge.ErrNotFound) {
			targetExists = false
		} else {
			return pluginBundleInstallResult{}, fmt.Errorf("prepare existing plugin bundle for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, pluginRoot); err != nil {
		if targetExists {
			if rollbackErr := client.Rename(context.WithoutCancel(ctx), backupDir, pluginRoot); rollbackErr != nil {
				return pluginBundleInstallResult{}, fmt.Errorf(
					"publish plugin bundle: %w; restore previous bundle from %q: %w",
					err, backupDir, rollbackErr,
				)
			}
		}
		return pluginBundleInstallResult{}, fmt.Errorf("publish plugin bundle: %w", err)
	}
	if targetExists {
		_ = client.DeleteFile(ctx, backupDir, true)
	}
	return result, nil
}

func pluginBundleArchiveEntry(archivePluginID, targetPluginID, rawName string) (pluginArchiveEntry, bool, error) {
	name, err := normalizePluginBundleArchivePath(archivePluginID, rawName)
	if err != nil {
		return pluginArchiveEntry{}, false, err
	}
	if name == "" {
		return pluginArchiveEntry{}, false, nil
	}
	segments := strings.Split(name, "/")

	switch segments[0] {
	case "plugin.yaml":
		if len(segments) == 1 {
			return pluginArchiveEntry{kind: pluginArchiveKindManifest, relativePath: "plugin.yaml"}, true, nil
		}
		return pluginArchiveEntry{}, false, errors.New("plugin bundle contains a path below plugin.yaml")
	case "hooks.json":
		if len(segments) != 1 {
			return pluginArchiveEntry{}, false, nil
		}
		root, err := skillset.PluginDirForID(targetPluginID)
		if err != nil {
			return pluginArchiveEntry{}, false, err
		}
		return pluginArchiveEntry{kind: pluginArchiveKindHooks, root: root, relativePath: "hooks.json"}, true, nil
	case "skills":
		return pluginArchiveEntry{}, false, errors.New("plugin bundle must reference Registry Skills instead of embedding skills/**")
	case "scripts":
		if len(segments) < 2 {
			return pluginArchiveEntry{}, false, nil
		}
		root, err := skillset.PluginScriptsDirForID(targetPluginID)
		if err != nil {
			return pluginArchiveEntry{}, false, err
		}
		return pluginArchiveEntry{kind: pluginArchiveKindScripts, root: root, relativePath: strings.Join(segments[1:], "/")}, true, nil
	}
	return pluginArchiveEntry{}, false, nil
}

func normalizePluginBundleArchivePath(archivePluginID, rawName string) (string, error) {
	if rawName == "" || rawName != strings.TrimSpace(rawName) || path.IsAbs(rawName) || strings.Contains(rawName, "\\") {
		return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
	}
	name := strings.TrimSuffix(rawName, "/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	pluginPrefix := strings.TrimSpace(archivePluginID)
	if !skillset.IsValidName(pluginPrefix) {
		return "", errors.New("plugin bundle archive id is invalid")
	}
	if name == pluginPrefix {
		return "", nil
	}
	name = strings.TrimPrefix(name, pluginPrefix+"/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
		}
	}
	return name, nil
}

func recordPluginBundleArchivePath(seen map[string]bool, name string, isFile bool) error {
	if _, exists := seen[name]; exists {
		return fmt.Errorf("plugin bundle contains duplicate path %q", name)
	}
	for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if seen[parent] {
			return fmt.Errorf("plugin bundle path %q is nested below file %q", name, parent)
		}
	}
	if isFile {
		for candidate := range seen {
			if strings.HasPrefix(candidate, name+"/") {
				return fmt.Errorf("plugin bundle file %q conflicts with child path %q", name, candidate)
			}
		}
	}
	seen[name] = isFile
	return nil
}

func validatePluginBundleManifest(content []byte, targetPluginID string) ([]pluginspkg.SkillReference, error) {
	var manifest struct {
		ID     string `yaml:"id"`
		Skills []struct {
			RegistryID string `yaml:"registry_id"`
			PackageID  string `yaml:"package_id"`
			SkillID    string `yaml:"skill_id"`
		} `yaml:"skills"`
	}
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("plugin bundle contains an invalid plugin.yaml: %w", err)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	if manifest.ID != targetPluginID {
		return nil, fmt.Errorf("plugin bundle manifest id %q does not match %q", manifest.ID, targetPluginID)
	}
	references := make([]pluginspkg.SkillReference, 0, len(manifest.Skills))
	for _, reference := range manifest.Skills {
		references = append(references, pluginspkg.SkillReference{
			RegistryID: strings.TrimSpace(reference.RegistryID),
			PackageID:  strings.TrimSpace(reference.PackageID),
			SkillID:    strings.TrimSpace(reference.SkillID),
		})
	}
	if err := pluginspkg.ValidateSkillReferences(references); err != nil {
		return nil, fmt.Errorf("plugin bundle manifest contains invalid Skill references: %w", err)
	}
	return references, nil
}

func samePluginSkillReferences(left, right []pluginspkg.SkillReference) bool {
	if len(left) != len(right) {
		return false
	}
	identities := make(map[string]struct{}, len(left))
	for _, reference := range left {
		identities[pluginspkg.SkillReferenceIdentity(reference)] = struct{}{}
	}
	for _, reference := range right {
		if _, ok := identities[pluginspkg.SkillReferenceIdentity(reference)]; !ok {
			return false
		}
	}
	return true
}

func newPluginBundleInstallResult() pluginBundleInstallResult {
	return pluginBundleInstallResult{
		Hooks:   pluginAssetInstallResult{OK: true},
		Scripts: pluginAssetInstallResult{OK: true},
	}
}

func newPluginInstallScriptsResult() pluginInstallScriptsResult {
	return pluginInstallScriptsResult{OK: true}
}

func withPluginBundleInstallMetadata(installation pluginspkg.Installation, result pluginBundleInstallResult, err error) pluginspkg.Installation {
	metadata := maps.Clone(installation.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if err != nil {
		failed := pluginAssetInstallResult{OK: false, Error: err.Error()}
		result = pluginBundleInstallResult{Hooks: failed, Scripts: failed}
	}
	metadata["hooks_install"] = result.Hooks
	metadata["scripts_install"] = result.Scripts
	installation.Metadata = metadata
	return installation
}

func withPluginInstallScriptsMetadata(installation pluginspkg.Installation, result pluginInstallScriptsResult, err error) pluginspkg.Installation {
	metadata := maps.Clone(installation.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	metadata["install_scripts"] = result
	installation.Metadata = metadata
	return installation
}

func withPluginSkillsInstallMetadata(installation pluginspkg.Installation, result pluginSkillsInstallResult, err error) pluginspkg.Installation {
	metadata := maps.Clone(installation.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	metadata["skills_install"] = result
	installation.Metadata = metadata
	return installation
}

func truncatePluginInstallOutput(output string) string {
	if len(output) <= pluginInstallScriptOutputLimit {
		return output
	}
	return output[:pluginInstallScriptOutputLimit]
}
