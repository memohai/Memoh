package handlers

import (
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
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/apperror"
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

type pluginAssetInstallResult = pluginspkg.BundleAssetInstallResult

type pluginBundleInstallResult = pluginspkg.BundleInstallResult

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
	maxPluginBundleCompressedBytes                 = pluginspkg.MaxBundleCompressedBytes
	maxPluginMetadataBytes                         = 2 * 1024 * 1024
	maxPluginReleasePackages                       = 128
	maxPluginSkillArtifactsCompressedBytes         = 128 * 1024 * 1024
	maxPluginSkillArtifactsUncompressedBytes       = 128 * 1024 * 1024
	maxPluginSkillArtifactsArchiveBytes            = 128 * 1024 * 1024
	maxPluginSkillArtifactFiles                    = 10_000
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
		upstream:       supermarketclient.NewClient(cfg.Supermarket.GetBaseURL(), nil),
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
	if len(entry.Release.Packages) > 0 {
		releasePreparation, err := acquireRegistryPackagePreparation(ctx)
		if err != nil {
			return err
		}
		defer releasePreparation()
	}
	packageDescriptors, skillsResult, err := h.resolvePluginPackages(ctx, entry.Release.Packages)
	if err != nil {
		return err
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
		bundlePublication, installErr := pluginspkg.PublishBundleArchive(
			mutationCtx, target.Client, target.Info.OS, manifest.ID, bundleArchive,
		)
		if installErr != nil {
			return rollbackPluginWorkspace(
				mutationCtx, echo.NewHTTPError(http.StatusBadGateway, installErr.Error()), nil, packagePublications,
			)
		}
		bundleResult = bundlePublication.Result()
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
		if err := bundlePublication.Commit(mutationCtx); err != nil && h.logger != nil {
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
	result, err := h.installRegistryPackage(c.Request().Context(), botID, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// --- Supermarket upstream types (for swagger) ---

type SupermarketAuthor = supermarketclient.Author

type SupermarketPluginArtifact = supermarketclient.PluginArtifact

type SupermarketPluginResolvedPackage = supermarketclient.PluginResolvedPackage

type SupermarketPluginRelease = supermarketclient.PluginRelease

type SupermarketImmutablePluginRelease = supermarketclient.ImmutablePluginRelease

type SupermarketPluginEntry = supermarketclient.PluginEntry

type SupermarketPluginListResponse = supermarketclient.PluginListResponse

// --- Internal helpers ---

func (h *SupermarketHandler) fetchPluginEntry(c echo.Context, pluginID string) (SupermarketPluginEntry, error) {
	pluginID = strings.TrimSpace(pluginID)
	if !skillset.IsValidName(pluginID) {
		return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadRequest, "plugin id is invalid")
	}
	entry, err := h.upstream.FetchPluginEntry(c.Request().Context(), pluginID)
	if err == nil {
		return entry, nil
	}
	if h.logger != nil {
		h.logger.Error("supermarket Plugin fetch failed", slog.String("plugin_id", pluginID), slog.Any("error", err))
	}
	if supermarketclient.ErrorKindOf(err) == supermarketclient.ErrorNotFound {
		return SupermarketPluginEntry{}, echo.NewHTTPError(
			http.StatusNotFound, fmt.Sprintf("plugin %q not found in supermarket", pluginID),
		)
	}
	return SupermarketPluginEntry{}, echo.NewHTTPError(http.StatusBadGateway, err.Error())
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
			return nil, result, apperror.Wrap(apperror.CodeRegistryPackageInvalid, err, nil)
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
	bundle *pluginspkg.BundlePublication,
	packages []*skillset.PackagePublication,
) error {
	errorsToJoin := []error{cause}
	if bundle != nil {
		if err := bundle.Rollback(ctx); err != nil {
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
	publication, err := pluginspkg.PublishBundleArchive(ctx, client, workspaceOS, targetPluginID, archive)
	if err != nil {
		return pluginBundleInstallResult{}, err
	}
	_ = publication.Commit(ctx)
	return publication.Result(), nil
}

func (h *SupermarketHandler) preparePluginBundle(
	ctx context.Context,
	downloadPluginID, targetPluginID string,
	artifact SupermarketPluginArtifact,
	expectedPackages []pluginspkg.PackageReference,
) (pluginspkg.BundleArchive, error) {
	bundle, err := h.downloadSupermarketArtifact(ctx, supermarketArtifactDownloadDescriptor{
		Digest: artifact.Digest, Size: artifact.Size, DownloadURL: artifact.DownloadURL,
	})
	if err != nil {
		return pluginspkg.BundleArchive{}, fmt.Errorf("download Plugin Artifact: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return pluginspkg.BundleArchive{}, fmt.Errorf("invalid gzip response from supermarket: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return pluginspkg.ReadBundleArchive(downloadPluginID, targetPluginID, gz, expectedPackages)
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

func samePluginPackageReferences(left, right []pluginspkg.PackageReference) bool {
	return pluginspkg.SamePackageReferences(left, right)
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
