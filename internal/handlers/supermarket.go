package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/memohai/memoh/internal/apperror"
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
	List(ctx context.Context, botID string) ([]pluginspkg.Installation, error)
	InstalledPluginState(ctx context.Context, botID, pluginID string) (pluginspkg.InstalledPluginState, bool, error)
	CheckSkillArtifactConflicts(
		ctx context.Context,
		botID, targetPluginID, workspaceTargetID string,
		expected map[string]string,
	) error
}

type pluginBundleWriter interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	ExecWithEnv(ctx context.Context, command, workDir string, timeout int32, env []string) (*bridge.ExecResult, error)
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
	pluginInstallScriptTimeoutSeconds        int32 = 10 * 60
	pluginInstallScriptOutputLimit                 = 64 * 1024
	maxPluginBundleCompressedBytes                 = 25 * 1024 * 1024
	maxPluginBundleUncompressedBytes               = 10 * 1024 * 1024
	maxPluginBundleStreamBytes                     = 16 * 1024 * 1024
	maxPluginBundleFileBytes                       = 2 * 1024 * 1024
	maxPluginBundleFiles                           = 1_000
	maxPluginBundleEntries                         = 2_000
	maxPluginMetadataBytes                         = 2 * 1024 * 1024
	maxPluginReleasePackages                       = 128
	maxPluginSkillArtifactsCompressedBytes         = 128 * 1024 * 1024
	maxPluginSkillArtifactsUncompressedBytes       = 128 * 1024 * 1024
	maxPluginSkillArtifactsArchiveBytes            = 128 * 1024 * 1024
	maxPluginSkillArtifactFiles                    = 10_000
	pluginPublicationCleanupTimeout                = 30 * time.Second
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
	url := h.baseURL + upstreamPath
	if qs := c.QueryString(); qs != "" {
		url += "?" + qs
	}

	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, url, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.doSupermarketRequest(req)
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
	if !isCanonicalSHA256(req.ReleaseRevision) {
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
		if !isCanonicalSHA256(expectedRevision) {
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

	entry, err := h.fetchPluginEntry(c, req.PluginID)
	if err != nil {
		return err
	}
	if entry.Release.Revision != req.ReleaseRevision {
		return echo.NewHTTPError(http.StatusConflict, "Plugin release changed; refresh before installing")
	}
	manifest := pluginspkg.NormalizeManifest(entry.Manifest)
	if manifest.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin id is required")
	}
	if err := pluginspkg.ValidatePackageReferences(manifest.Packages); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := validateSupermarketPluginEntry(entry, req.PluginID, manifest); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("invalid Plugin release from supermarket: %v", err))
	}

	ctx := c.Request().Context()
	target, err := h.resolvePluginInstallTarget(ctx, botID)
	if err != nil {
		return err
	}
	packageDescriptors, skillsResult, err := h.resolvePluginPackages(ctx, entry.Release.Packages)
	if err != nil {
		return err
	}
	if len(packageDescriptors) > 0 {
		releasePreparation, err := acquireRegistryPackagePreparation(ctx)
		if err != nil {
			return err
		}
		defer releasePreparation()
	}
	bundleArchive, err := h.preparePluginBundle(
		ctx, req.PluginID, manifest.ID, entry.Release.Artifact, manifest.Packages,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	preparedPackages, skillsResult, err := h.preparePluginPackages(
		ctx, target.Info.OS, entry.Release.Packages, packageDescriptors, skillsResult,
	)
	if err != nil {
		return err
	}
	var (
		installation  pluginspkg.Installation
		bundleResult  pluginBundleInstallResult
		scriptsResult pluginInstallScriptsResult
	)
	if err := withBotMutation(ctx, botID, h.pluginService, func(mutationCtx context.Context) error {
		installedState, installed, currentErr := h.pluginService.InstalledPluginState(
			mutationCtx, botID, manifest.ID,
		)
		if currentErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, currentErr.Error())
		}
		if !matchesExpectedPluginInstallation(
			req.ExpectedInstalledRevision,
			req.ExpectedInstallationUpdatedAt,
			installedState,
			installed,
		) {
			return echo.NewHTTPError(http.StatusConflict, "installed Plugin changed; refresh before installing")
		}
		expectedArtifacts := expectedPluginPackageArtifacts(preparedPackages)
		if conflictErr := h.pluginService.CheckSkillArtifactConflicts(
			mutationCtx, botID, manifest.ID, target.TargetID, expectedArtifacts,
		); conflictErr != nil {
			return echo.NewHTTPError(http.StatusConflict, conflictErr.Error())
		}
		for _, prepared := range preparedPackages {
			if err := h.checkPluginPackageMembers(
				mutationCtx, botID, target.TargetID, manifest.ID,
				prepared.Descriptor.RegistryID, prepared.Descriptor.PackageID, prepared.ExpectedArtifacts,
			); err != nil {
				return err
			}
		}
		var (
			skillArtifacts      map[string]pluginspkg.SkillArtifactMetadata
			installedSkills     []pluginspkg.InstalledSkill
			packagePublications []*skillset.PackagePublication
			installErr          error
		)
		skillsResult, installedSkills, skillArtifacts, packagePublications, installErr = publishPreparedPluginPackages(
			mutationCtx, target.Client, target.TargetID, preparedPackages, skillsResult,
		)
		if installErr != nil {
			return installErr
		}
		bundlePublication, installErr := publishPluginBundleArchiveTransaction(
			mutationCtx, target.Client, target.Info.OS, manifest.ID, bundleArchive,
		)
		if installErr != nil {
			return rollbackPluginWorkspace(
				mutationCtx, echo.NewHTTPError(http.StatusBadGateway, installErr.Error()), nil, packagePublications,
			)
		}
		bundleResult = bundlePublication.result
		scriptsResult, installErr = runPluginInstallScripts(
			mutationCtx, target.Client, botID, manifest.ID, manifest.Install,
		)
		if installErr != nil {
			return rollbackPluginWorkspace(
				mutationCtx, echo.NewHTTPError(http.StatusBadGateway, installErr.Error()), bundlePublication, packagePublications,
			)
		}
		installation, installErr = h.pluginService.Install(mutationCtx, botID, pluginspkg.InstallRequest{
			Manifest:        manifest,
			Variables:       req.Variables,
			InstalledSkills: installedSkills,
			SkillArtifacts:  skillArtifacts,
			Release: pluginspkg.ReleaseMetadata{
				Revision:       entry.Release.Revision,
				ArtifactDigest: entry.Release.Artifact.Digest,
			},
			WorkspaceTargetID: target.TargetID,
		})
		if installErr != nil {
			return rollbackPluginWorkspace(
				mutationCtx, echo.NewHTTPError(http.StatusBadRequest, installErr.Error()), bundlePublication, packagePublications,
			)
		}
		if err := bundlePublication.commit(mutationCtx); err != nil && h.logger != nil {
			h.logger.Warn("cleanup Plugin bundle backup failed", slog.Any("error", err))
		}
		for _, publication := range packagePublications {
			if err := publication.Commit(mutationCtx); err != nil && h.logger != nil {
				h.logger.Warn("cleanup Plugin Package backup failed", slog.Any("error", err))
			}
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

func matchesExpectedPluginInstallation(
	expectedRevision *string,
	expectedUpdatedAt *time.Time,
	actual pluginspkg.InstalledPluginState,
	installed bool,
) bool {
	if expectedRevision == nil || expectedUpdatedAt == nil {
		return !installed
	}
	return installed && actual.ReleaseRevision == *expectedRevision && actual.UpdatedAt.Equal(*expectedUpdatedAt)
}

// InstallPackage godoc
// @Summary Install every Skill in a supermarket Package to a bot workspace
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
	result, err := h.installRegistryPackage(c.Request().Context(), botID, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// --- Supermarket upstream types (for swagger) ---

type SupermarketAuthor struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required"`
}

type SupermarketPluginArtifact struct {
	Format      string `json:"format" validate:"required"`
	Digest      string `json:"digest" validate:"required"`
	Size        int64  `json:"size" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	DownloadURL string `json:"download_url" validate:"required"`
}

type SupermarketPluginResolvedPackage struct {
	RegistryID string `json:"registry_id" validate:"required"`
	PackageID  string `json:"package_id" validate:"required"`
	Revision   string `json:"revision" validate:"required"`
}

type SupermarketPluginRelease struct {
	Revision    string                             `json:"revision" validate:"required"`
	PublishedAt string                             `json:"published_at" validate:"required"`
	Artifact    SupermarketPluginArtifact          `json:"artifact" validate:"required"`
	Packages    []SupermarketPluginResolvedPackage `json:"packages" validate:"required"`
}

type SupermarketImmutablePluginRelease struct {
	SchemaVersion string                             `json:"schema_version"`
	Plugin        pluginspkg.Manifest                `json:"plugin"`
	Artifact      SupermarketPluginArtifact          `json:"artifact"`
	Packages      []SupermarketPluginResolvedPackage `json:"packages"`
}

type SupermarketPluginEntry struct {
	pluginspkg.Manifest
	Release SupermarketPluginRelease `json:"release" validate:"required"`
}

type SupermarketPluginListResponse struct {
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Limit int                      `json:"limit"`
	Data  []SupermarketPluginEntry `json:"data"`
}

// --- Internal helpers ---

func (h *SupermarketHandler) fetchPluginEntry(c echo.Context, pluginID string) (SupermarketPluginEntry, error) {
	pluginID = strings.TrimSpace(pluginID)
	if !skillset.IsValidName(pluginID) {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadRequest, "plugin id is invalid")
	}
	endpoint := strings.TrimRight(h.baseURL, "/") + "/api/plugins/" + url.PathEscape(pluginID)
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		h.logger.Error("supermarket plugin fetch failed", slog.String("url", endpoint), slog.Any("error", err))
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("plugin %q not found in supermarket", pluginID))
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned status %d", resp.StatusCode))
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginMetadataBytes+1))
	if err != nil {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid JSON from supermarket")
	}
	if len(payload) > maxPluginMetadataBytes {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "Plugin response is too large or malformed")
	}
	var entry SupermarketPluginEntry
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&entry); err != nil {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid JSON from supermarket")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "Plugin response is too large or malformed")
	}
	if !isCanonicalSHA256(entry.Release.Revision) {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid Plugin release revision from supermarket")
	}
	if _, err := time.Parse(time.RFC3339, entry.Release.PublishedAt); err != nil {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid Plugin release publication time from supermarket")
	}
	return h.fetchImmutablePluginRelease(
		c.Request().Context(), pluginID, entry.Release.Revision, entry.Release.PublishedAt,
	)
}

func (h *SupermarketHandler) fetchImmutablePluginRelease(
	ctx context.Context,
	pluginID, revision, publishedAt string,
) (SupermarketPluginEntry, error) {
	endpoint := strings.TrimRight(h.baseURL, "/") + "/api/plugins/" + url.PathEscape(pluginID) +
		"/releases/" + url.PathEscape(revision)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		h.logger.Error("supermarket Plugin release fetch failed", slog.String("url", endpoint), slog.Any("error", err))
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "approved Plugin release is missing")
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketPluginEntry{}, echo.NewHTTPError(
			http.StatusBadGateway,
			fmt.Sprintf("supermarket returned status %d for Plugin release", resp.StatusCode),
		)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginMetadataBytes+1))
	if err != nil || len(payload) > maxPluginMetadataBytes {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "Plugin release is too large or malformed")
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != revision {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "Plugin release SHA-256 verification failed")
	}
	var release SupermarketImmutablePluginRelease
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&release); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "invalid immutable Plugin release from supermarket")
	}
	if release.SchemaVersion != "1" {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, "unsupported immutable Plugin release schema")
	}
	release.Artifact.DownloadURL = "/api/artifacts/plugin/" + release.Artifact.Digest
	return SupermarketPluginEntry{
		Manifest: release.Plugin,
		Release: SupermarketPluginRelease{
			Revision:    revision,
			PublishedAt: publishedAt,
			Artifact:    release.Artifact,
			Packages:    release.Packages,
		},
	}, nil
}

var (
	errSupermarketRedirectOrigin = errors.New("supermarket redirect left the configured origin")
	errSupermarketRedirectLimit  = errors.New("supermarket redirect limit exceeded")
)

func (h *SupermarketHandler) doSupermarketRequest(req *http.Request) (*http.Response, error) {
	base, err := url.Parse(h.baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, errors.New("configured Supermarket URL is invalid")
	}
	client := http.Client{Timeout: 30 * time.Second}
	if h.httpClient != nil {
		client = *h.httpClient
	}
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errSupermarketRedirectLimit
		}
		if !sameHTTPOrigin(next.URL, base) {
			return errSupermarketRedirectOrigin
		}
		return nil
	}
	resp, err := client.Do(req) //nolint:gosec // Initial URL is derived from trusted config; redirects remain same-origin.
	if err != nil {
		return nil, err
	}
	if resp.Request == nil || !sameHTTPOrigin(resp.Request.URL, base) {
		_ = resp.Body.Close()
		return nil, errSupermarketRedirectOrigin
	}
	return resp, nil
}

func sameHTTPOrigin(candidate, base *url.URL) bool {
	return candidate != nil && base != nil && candidate.User == nil &&
		candidate.Scheme == base.Scheme && strings.EqualFold(candidate.Host, base.Host)
}

func validateSupermarketPluginEntry(
	entry SupermarketPluginEntry,
	expectedPluginID string,
	manifest pluginspkg.Manifest,
) error {
	if manifest.ID != expectedPluginID {
		return errors.New("plugin identity does not match the request")
	}
	if !isCanonicalSHA256(entry.Release.Revision) {
		return errors.New("plugin release revision is invalid")
	}
	if _, err := time.Parse(time.RFC3339, entry.Release.PublishedAt); err != nil {
		return errors.New("plugin release publication time is invalid")
	}
	artifact := entry.Release.Artifact
	if artifact.Format != "memoh_plugin_v1" || artifact.ContentType != "application/gzip" {
		return errors.New("plugin Artifact format is unsupported")
	}
	if !isCanonicalSHA256(artifact.Digest) || artifact.Size < 1 ||
		artifact.Size > maxPluginBundleCompressedBytes || strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("plugin Artifact descriptor is invalid")
	}
	if len(entry.Release.Packages) != len(manifest.Packages) {
		return errors.New("plugin release does not lock every Package reference")
	}
	if len(entry.Release.Packages) > maxPluginReleasePackages {
		return fmt.Errorf("plugin release exceeds the %d Package limit", maxPluginReleasePackages)
	}
	resolvedReferences := make([]pluginspkg.PackageReference, 0, len(entry.Release.Packages))
	for _, resolved := range entry.Release.Packages {
		reference := pluginspkg.PackageReference{
			RegistryID: resolved.RegistryID,
			PackageID:  resolved.PackageID,
		}
		resolvedReferences = append(resolvedReferences, reference)
		if !isCanonicalSHA256(resolved.Revision) {
			return fmt.Errorf("plugin Package %q revision is invalid", pluginspkg.PackageReferenceIdentity(reference))
		}
	}
	if !samePluginPackageReferences(resolvedReferences, manifest.Packages) {
		return errors.New("plugin release Package locks do not match plugin.yaml")
	}
	return nil
}

func isCanonicalSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
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

func (h *SupermarketHandler) preparePluginPackages(
	ctx context.Context,
	workspaceOS string,
	resolvedPackages []SupermarketPluginResolvedPackage,
	descriptors []SupermarketSkillPackageDescriptor,
	result pluginSkillsInstallResult,
) ([]preparedRegistryPackage, pluginSkillsInstallResult, error) {
	if len(descriptors) != len(resolvedPackages) {
		return nil, result, errors.New("plugin Package descriptor count does not match its locks")
	}
	prepared := make([]preparedRegistryPackage, 0, len(descriptors))
	for index, pkg := range descriptors {
		resolved := resolvedPackages[index]
		item, err := h.prepareRegistryPackage(
			ctx, workspaceOS, pkg, resolved.RegistryID, resolved.PackageID, resolved.Revision,
		)
		if err != nil {
			result.OK = false
			result.Error = err.Error()
			return nil, result, err
		}
		prepared = append(prepared, item)
	}
	return prepared, result, nil
}

func (h *SupermarketHandler) resolvePluginPackages(
	ctx context.Context,
	resolvedPackages []SupermarketPluginResolvedPackage,
) ([]SupermarketSkillPackageDescriptor, pluginSkillsInstallResult, error) {
	result := pluginSkillsInstallResult{OK: true}
	if len(resolvedPackages) > maxPluginReleasePackages {
		return nil, result, fmt.Errorf("plugin release exceeds the %d Package limit", maxPluginReleasePackages)
	}
	descriptors := make([]SupermarketSkillPackageDescriptor, 0, len(resolvedPackages))
	budget := pluginSkillArtifactBudget{}
	for _, resolved := range resolvedPackages {
		pkg, err := h.fetchRegistryPackageRelease(ctx, resolved.RegistryID, resolved.PackageID, resolved.Revision)
		if err != nil {
			return nil, result, err
		}
		if pkg.Revision != resolved.Revision {
			return nil, result, errors.New("plugin Package release revision does not match its lock")
		}
		if err := validateRegistryPackage(pkg, resolved.RegistryID, resolved.PackageID); err != nil {
			return nil, result, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
		}
		for _, skill := range pkg.Skills {
			if err := budget.add(skill.Artifact); err != nil {
				return nil, result, err
			}
		}
		descriptors = append(descriptors, pkg)
	}
	return descriptors, result, nil
}

type pluginSkillArtifactBudget struct {
	compressedBytes   int64
	uncompressedBytes int64
	archiveBytes      int64
	files             int
}

func (b *pluginSkillArtifactBudget) add(artifact SupermarketSkillArtifact) error {
	if artifact.Size < 1 || artifact.Size > maxRegistrySkillArtifactCompressedBytes ||
		artifact.Size > int64(maxPluginSkillArtifactsCompressedBytes)-b.compressedBytes {
		return fmt.Errorf(
			"plugin release Skills exceed the %d byte compressed limit",
			maxPluginSkillArtifactsCompressedBytes,
		)
	}
	if artifact.UncompressedSize < 1 ||
		artifact.UncompressedSize > maxRegistrySkillArtifactUncompressedBytes ||
		artifact.UncompressedSize > int64(maxPluginSkillArtifactsUncompressedBytes)-b.uncompressedBytes {
		return fmt.Errorf(
			"plugin release Skills exceed the %d byte uncompressed limit",
			maxPluginSkillArtifactsUncompressedBytes,
		)
	}
	if artifact.ArchiveSize < 1 || artifact.ArchiveSize > maxRegistrySkillArtifactArchiveBytes ||
		artifact.ArchiveSize > int64(maxPluginSkillArtifactsArchiveBytes)-b.archiveBytes {
		return fmt.Errorf(
			"plugin release Skills exceed the %d byte decompressed archive limit",
			maxPluginSkillArtifactsArchiveBytes,
		)
	}
	if artifact.FileCount < 1 || artifact.FileCount > maxRegistrySkillArtifactFiles ||
		artifact.FileCount > maxPluginSkillArtifactFiles-b.files {
		return fmt.Errorf(
			"plugin release Skills exceed the %d file limit",
			maxPluginSkillArtifactFiles,
		)
	}
	b.compressedBytes += artifact.Size
	b.uncompressedBytes += artifact.UncompressedSize
	b.archiveBytes += artifact.ArchiveSize
	b.files += artifact.FileCount
	return nil
}

func expectedPluginPackageArtifacts(prepared []preparedRegistryPackage) map[string]string {
	expected := make(map[string]string)
	for _, pkg := range prepared {
		for identity, digest := range pkg.ExpectedArtifacts {
			expected[identity] = digest
		}
	}
	return expected
}

func publishPreparedPluginPackages(
	ctx context.Context,
	client *bridge.Client,
	workspaceTargetID string,
	prepared []preparedRegistryPackage,
	result pluginSkillsInstallResult,
) (
	pluginSkillsInstallResult,
	[]pluginspkg.InstalledSkill,
	map[string]pluginspkg.SkillArtifactMetadata,
	[]*skillset.PackagePublication,
	error,
) {
	installedSkills := make([]pluginspkg.InstalledSkill, 0)
	artifacts := make(map[string]pluginspkg.SkillArtifactMetadata)
	publications := make([]*skillset.PackagePublication, 0, len(prepared))
	for _, pkg := range prepared {
		publication, installed, err := publishPreparedRegistryPackage(ctx, client, pkg, workspaceTargetID)
		if err != nil {
			result.OK = false
			result.Error = err.Error()
			if rollbackErr := rollbackRegistryPackages(ctx, publications); rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
			return result, nil, nil, publications, err
		}
		publications = append(publications, publication)
		for _, skill := range installed {
			installedSkill := pluginspkg.InstalledSkill{
				RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID,
			}
			installedSkills = append(installedSkills, installedSkill)
			result.Skills = append(result.Skills, pluginSkillInstallResult{
				RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID,
				InstallID: skill.InstallID, ArtifactDigest: skill.ArtifactDigest, FilesWritten: skill.FilesWritten,
			})
			artifacts[pluginspkg.InstalledSkillIdentity(installedSkill)] = pluginspkg.SkillArtifactMetadata{
				PackageRevision: pkg.Descriptor.Revision,
				InstallID:       skill.InstallID, ArtifactDigest: skill.ArtifactDigest, FilesWritten: skill.FilesWritten,
			}
		}
	}
	return result, installedSkills, artifacts, publications, nil
}

func rollbackRegistryPackages(ctx context.Context, publications []*skillset.PackagePublication) error {
	var errs []error
	for index := len(publications) - 1; index >= 0; index-- {
		if err := publications[index].Rollback(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func rollbackPluginWorkspace(
	ctx context.Context,
	cause error,
	bundle *pluginBundlePublication,
	packages []*skillset.PackagePublication,
) error {
	errorsToJoin := []error{cause}
	if bundle != nil {
		if err := bundle.rollback(ctx); err != nil {
			errorsToJoin = append(errorsToJoin, fmt.Errorf("roll back Plugin bundle: %w", err))
		}
	}
	if len(packages) > 0 {
		if err := rollbackRegistryPackages(ctx, packages); err != nil {
			errorsToJoin = append(errorsToJoin, fmt.Errorf("roll back Plugin Packages: %w", err))
		}
	}
	if len(errorsToJoin) == 1 {
		return cause
	}
	return errors.Join(errorsToJoin...)
}

func (h *SupermarketHandler) installPluginBundle(
	ctx context.Context,
	client pluginBundleWriter,
	workspaceOS string,
	downloadPluginID, targetPluginID string,
	artifact SupermarketPluginArtifact,
	expectedPackages []pluginspkg.PackageReference,
) (pluginBundleInstallResult, error) {
	if client == nil {
		return pluginBundleInstallResult{}, errors.New("workspace is not reachable")
	}
	archive, err := h.preparePluginBundle(ctx, downloadPluginID, targetPluginID, artifact, expectedPackages)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	return publishPluginBundleArchive(ctx, client, workspaceOS, targetPluginID, archive)
}

func (h *SupermarketHandler) preparePluginBundle(
	ctx context.Context,
	downloadPluginID, targetPluginID string,
	artifact SupermarketPluginArtifact,
	expectedPackages []pluginspkg.PackageReference,
) (pluginBundleArchive, error) {
	bundle, err := h.downloadSupermarketArtifact(ctx, supermarketArtifactDownloadDescriptor{
		Digest: artifact.Digest, Size: artifact.Size, DownloadURL: artifact.DownloadURL,
	})
	if err != nil {
		return pluginBundleArchive{}, fmt.Errorf("download Plugin Artifact: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return pluginBundleArchive{}, fmt.Errorf("invalid gzip response from supermarket: %w", err)
	}
	defer func() { _ = gz.Close() }()

	archive, err := readPluginBundleArchive(downloadPluginID, targetPluginID, gz)
	if err != nil {
		return pluginBundleArchive{}, err
	}
	if !samePluginPackageReferences(archive.packageReferences, expectedPackages) {
		return pluginBundleArchive{}, errors.New("plugin bundle Package references do not match the catalog manifest")
	}
	return archive, nil
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
	entry      pluginArchiveEntry
	content    []byte
	executable bool
}

type pluginBundleArchive struct {
	files             []pluginBundleArchiveFile
	packageReferences []pluginspkg.PackageReference
}

func extractPluginBundleArchive(
	ctx context.Context,
	client pluginBundleWriter,
	workspaceOS, archivePluginID, targetPluginID string,
	r io.Reader,
) (pluginBundleInstallResult, error) {
	archive, err := readPluginBundleArchive(archivePluginID, targetPluginID, r)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	return publishPluginBundleArchive(ctx, client, workspaceOS, targetPluginID, archive)
}

func readPluginBundleArchive(archivePluginID, targetPluginID string, r io.Reader) (pluginBundleArchive, error) {
	if !skillset.IsValidPluginID(archivePluginID) {
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
	stream := &io.LimitedReader{R: r, N: maxPluginBundleStreamBytes + 1}
	tr := tar.NewReader(stream)
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
			if !isAllowedPluginBundleDirectory(name) {
				return pluginBundleArchive{}, fmt.Errorf("plugin bundle contains unsupported directory %q", hdr.Name)
			}
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
			return pluginBundleArchive{}, fmt.Errorf("plugin bundle contains unsupported file %q", hdr.Name)
		}
		if entry.kind == pluginArchiveKindManifest {
			references, err := validatePluginBundleManifest(content, targetPluginID)
			if err != nil {
				return pluginBundleArchive{}, err
			}
			archive.packageReferences = references
			hasManifest = true
		}
		archive.files = append(archive.files, pluginBundleArchiveFile{
			entry: entry, content: content, executable: hdr.FileInfo().Mode().Perm()&0o111 != 0,
		})
	}
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return pluginBundleArchive{}, fmt.Errorf("plugin bundle decompression failed: %w", err)
	}
	if stream.N <= 0 {
		return pluginBundleArchive{}, errors.New("plugin bundle exceeds the decompressed stream limit")
	}
	if !hasManifest {
		return pluginBundleArchive{}, errors.New("plugin bundle does not contain a root plugin.yaml")
	}
	return archive, nil
}

func publishPluginBundleArchive(
	ctx context.Context,
	client pluginBundleWriter,
	workspaceOS string,
	targetPluginID string,
	archive pluginBundleArchive,
) (pluginBundleInstallResult, error) {
	publication, err := publishPluginBundleArchiveTransaction(ctx, client, workspaceOS, targetPluginID, archive)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	_ = publication.commit(ctx)
	return publication.result, nil
}

type pluginBundlePublicationClient interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

type pluginBundlePublication struct {
	client       pluginBundlePublicationClient
	pluginRoot   string
	backupDir    string
	targetExists bool
	closed       bool
	result       pluginBundleInstallResult
}

func publishPluginBundleArchiveTransaction(
	ctx context.Context,
	client pluginBundleWriter,
	workspaceOS string,
	targetPluginID string,
	archive pluginBundleArchive,
) (*pluginBundlePublication, error) {
	result := newPluginBundleInstallResult()
	if client == nil {
		return nil, errors.New("workspace is not reachable")
	}
	pluginRoot, err := skillset.PluginDirForID(targetPluginID)
	if err != nil {
		return nil, err
	}
	stagingRoot := path.Join(skillset.PluginDirPath, ".staging")
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	tempDir := path.Join(stagingRoot, "install-"+targetPluginID+"-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+targetPluginID+"-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() {
		cleanupCtx, cancel := pluginPublicationCleanupContext(ctx)
		defer cancel()
		_ = client.DeleteFile(cleanupCtx, tempDir, true)
	}()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return nil, fmt.Errorf("create plugin staging root: %w", err)
	}
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("create temporary plugin directory: %w", err)
	}

	executablePaths := make([]string, 0)
	for _, file := range archive.files {
		relativePath := file.entry.relativePath
		if file.entry.kind == pluginArchiveKindScripts {
			relativePath = path.Join("scripts", relativePath)
		}
		filePath := path.Clean(path.Join(tempDir, relativePath))
		if filePath == tempDir || !strings.HasPrefix(filePath, tempDir+"/") {
			return nil, fmt.Errorf("plugin bundle path escapes staging root: %s", relativePath)
		}
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return nil, fmt.Errorf("create plugin bundle directory %s: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return nil, fmt.Errorf("write plugin bundle file %s: %w", relativePath, err)
		}
		if file.entry.kind == pluginArchiveKindScripts && file.executable {
			executablePaths = append(executablePaths, filePath)
		}
		switch file.entry.kind {
		case pluginArchiveKindHooks:
			result.Hooks.FilesWritten++
		case pluginArchiveKindScripts:
			result.Scripts.FilesWritten++
		}
	}
	if err := applyPluginExecutableModes(ctx, client, workspaceOS, executablePaths); err != nil {
		return nil, err
	}

	targetExists := true
	if err := client.Rename(ctx, pluginRoot, backupDir); err != nil {
		if errors.Is(err, bridge.ErrNotFound) {
			targetExists = false
		} else {
			return nil, fmt.Errorf("prepare existing plugin bundle for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, pluginRoot); err != nil {
		if targetExists {
			rollbackCtx, cancel := pluginPublicationCleanupContext(ctx)
			defer cancel()
			if rollbackErr := client.Rename(rollbackCtx, backupDir, pluginRoot); rollbackErr != nil {
				return nil, fmt.Errorf(
					"publish plugin bundle: %w; restore previous bundle from %q: %w",
					err, backupDir, rollbackErr,
				)
			}
		}
		return nil, fmt.Errorf("publish plugin bundle: %w", err)
	}
	return &pluginBundlePublication{
		client: client, pluginRoot: pluginRoot, backupDir: backupDir, targetExists: targetExists, result: result,
	}, nil
}

func (p *pluginBundlePublication) commit(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	cleanupCtx, cancel := pluginPublicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(cleanupCtx, p.backupDir, true); err != nil {
		return err
	}
	p.closed = true
	return nil
}

func (p *pluginBundlePublication) rollback(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	rollbackCtx, cancel := pluginPublicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(rollbackCtx, p.pluginRoot, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove replacement Plugin bundle: %w", err)
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	if err := p.client.Rename(rollbackCtx, p.backupDir, p.pluginRoot); err != nil {
		return fmt.Errorf("restore previous Plugin bundle from %q: %w", p.backupDir, err)
	}
	p.closed = true
	return nil
}

func pluginPublicationCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), pluginPublicationCleanupTimeout)
}

func applyPluginExecutableModes(
	ctx context.Context,
	client pluginBundleWriter,
	workspaceOS string,
	paths []string,
) error {
	if len(paths) == 0 || strings.EqualFold(workspaceOS, "windows") || strings.EqualFold(workspaceOS, "win32") {
		return nil
	}
	for start := 0; start < len(paths); start += 100 {
		end := min(start+100, len(paths))
		quoted := make([]string, 0, end-start)
		for _, filePath := range paths[start:end] {
			quoted = append(quoted, pluginShellQuote(filePath))
		}
		result, err := client.ExecWithEnv(ctx, "chmod 755 -- "+strings.Join(quoted, " "), "/", 30, nil)
		if err != nil {
			return fmt.Errorf("preserve executable Plugin scripts: %w", err)
		}
		if result != nil && result.ExitCode != 0 {
			return fmt.Errorf("preserve executable Plugin scripts: chmod exited with code %d", result.ExitCode)
		}
	}
	return nil
}

func pluginShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
			return pluginArchiveEntry{}, false, errors.New("plugin bundle contains a path below hooks.json")
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
			return pluginArchiveEntry{}, false, errors.New("plugin bundle scripts path must name a file")
		}
		root, err := skillset.PluginScriptsDirForID(targetPluginID)
		if err != nil {
			return pluginArchiveEntry{}, false, err
		}
		return pluginArchiveEntry{kind: pluginArchiveKindScripts, root: root, relativePath: strings.Join(segments[1:], "/")}, true, nil
	}
	return pluginArchiveEntry{}, false, fmt.Errorf("plugin bundle contains unsupported file %q", rawName)
}

func isAllowedPluginBundleDirectory(name string) bool {
	return name == "scripts" || strings.HasPrefix(name, "scripts/")
}

func normalizePluginBundleArchivePath(archivePluginID, rawName string) (string, error) {
	if rawName == "" || rawName != strings.TrimSpace(rawName) || path.IsAbs(rawName) || strings.Contains(rawName, "\\") {
		return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
	}
	name := strings.TrimSuffix(rawName, "/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	pluginPrefix := archivePluginID
	if !skillset.IsValidPluginID(pluginPrefix) {
		return "", errors.New("plugin bundle archive id is invalid")
	}
	if name == pluginPrefix {
		return "", nil
	}
	if !strings.HasPrefix(name, pluginPrefix+"/") {
		return "", fmt.Errorf("plugin bundle path %q is outside the %q root", rawName, pluginPrefix)
	}
	name = strings.TrimPrefix(name, pluginPrefix+"/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("plugin bundle contains non-canonical path %q", rawName)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || segment != strings.TrimSpace(segment) ||
			strings.HasSuffix(segment, ".") || strings.ContainsAny(segment, `<>:"|?*`) || isWindowsReservedPathName(segment) {
			return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
		}
		for _, character := range segment {
			if character < 0x20 || character == 0x7f {
				return "", fmt.Errorf("plugin bundle contains unsafe path %q", rawName)
			}
		}
	}
	return name, nil
}

func isWindowsReservedPathName(value string) bool {
	base := strings.ToLower(strings.SplitN(value, ".", 2)[0])
	if base == "con" || base == "prn" || base == "aux" || base == "nul" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) &&
		base[3] >= '1' && base[3] <= '9'
}

func recordPluginBundleArchivePath(seen map[string]bool, name string, isFile bool) error {
	canonicalName := strings.ToLower(name)
	if _, exists := seen[canonicalName]; exists {
		return fmt.Errorf("plugin bundle contains duplicate path %q", name)
	}
	for parent := path.Dir(canonicalName); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if seen[parent] {
			return fmt.Errorf("plugin bundle path %q is nested below file %q", name, parent)
		}
	}
	if isFile {
		for candidate := range seen {
			if strings.HasPrefix(candidate, canonicalName+"/") {
				return fmt.Errorf("plugin bundle file %q conflicts with child path %q", name, candidate)
			}
		}
	}
	seen[canonicalName] = isFile
	return nil
}

func validatePluginBundleManifest(content []byte, targetPluginID string) ([]pluginspkg.PackageReference, error) {
	var manifest struct {
		ID       string `yaml:"id"`
		Packages []struct {
			RegistryID string `yaml:"registry_id"`
			PackageID  string `yaml:"package_id"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("plugin bundle contains an invalid plugin.yaml: %w", err)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	if manifest.ID != targetPluginID {
		return nil, fmt.Errorf("plugin bundle manifest id %q does not match %q", manifest.ID, targetPluginID)
	}
	references := make([]pluginspkg.PackageReference, 0, len(manifest.Packages))
	for _, reference := range manifest.Packages {
		references = append(references, pluginspkg.PackageReference{
			RegistryID: strings.TrimSpace(reference.RegistryID),
			PackageID:  strings.TrimSpace(reference.PackageID),
		})
	}
	if err := pluginspkg.ValidatePackageReferences(references); err != nil {
		return nil, fmt.Errorf("plugin bundle manifest contains invalid Package references: %w", err)
	}
	return references, nil
}

func samePluginPackageReferences(left, right []pluginspkg.PackageReference) bool {
	if len(left) != len(right) {
		return false
	}
	identities := make(map[string]struct{}, len(left))
	for _, reference := range left {
		identities[pluginspkg.PackageReferenceIdentity(reference)] = struct{}{}
	}
	for _, reference := range right {
		if _, ok := identities[pluginspkg.PackageReferenceIdentity(reference)]; !ok {
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
