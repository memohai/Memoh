package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxRegistrySkillArtifactCompressedBytes   = 6 * 1024 * 1024
	maxRegistrySkillArtifactUncompressedBytes = 5 * 1024 * 1024
	maxRegistrySkillArtifactArchiveBytes      = 5 * 1024 * 1024
	maxRegistrySkillArtifactFiles             = 1_000
	maxRegistrySkillMetadataBytes             = 2 * 1024 * 1024
	maxRegistryPackageMetadataBytes           = 8 * 1024 * 1024
	maxRegistryPackageSkills                  = 128
	maxConcurrentSkillArtifactPreparations    = 2
)

var skillArtifactPreparationTokens = make(chan struct{}, maxConcurrentSkillArtifactPreparations)

type SupermarketRegistryListResponse struct {
	Data []SupermarketRegistry `json:"data" validate:"required"`
}

type SupermarketRegistry struct {
	ID                  string `json:"id" validate:"required"`
	Name                string `json:"name" validate:"required"`
	Enabled             bool   `json:"enabled" validate:"required"`
	Priority            int    `json:"priority" validate:"required"`
	Adapter             string `json:"adapter" validate:"required"`
	Revision            string `json:"revision,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	SkillCount          int    `json:"skill_count" validate:"required"`
	PackageCount        int    `json:"package_count" validate:"required"`
	CategoryCount       int    `json:"category_count" validate:"required"`
	SkippedPackageCount int    `json:"skipped_package_count" validate:"required"`
}

type SupermarketSkillCategoryListResponse struct {
	Data []SupermarketSkillCategory `json:"data" validate:"required"`
}

type SupermarketSkillCategoryRegistry struct {
	ID    string `json:"id" validate:"required"`
	Count int    `json:"count" validate:"required"`
}

type SupermarketSkillCategory struct {
	ID         string                             `json:"id" validate:"required"`
	Name       string                             `json:"name" validate:"required"`
	Count      int                                `json:"count" validate:"required"`
	Registries []SupermarketSkillCategoryRegistry `json:"registries" validate:"required"`
}

type SupermarketSkillSource struct {
	Type       string `json:"type" validate:"required"`
	Revision   string `json:"revision" validate:"required"`
	Path       string `json:"path" validate:"required"`
	Repository string `json:"repository,omitempty"`
}

type SupermarketSkillArtifact struct {
	Format           string `json:"format" validate:"required"`
	Digest           string `json:"digest" validate:"required"`
	Size             int64  `json:"size" validate:"required"`
	UncompressedSize int64  `json:"uncompressed_size" validate:"required"`
	ArchiveSize      int64  `json:"archive_size" validate:"required"`
	FileCount        int    `json:"file_count" validate:"required"`
	ContentType      string `json:"content_type" validate:"required"`
	DownloadURL      string `json:"download_url" validate:"required"`
}

type supermarketArtifactDownloadDescriptor struct {
	Digest      string
	Size        int64
	DownloadURL string
}

type SupermarketSkillIconAsset struct {
	Digest      string `json:"digest" validate:"required"`
	Size        int64  `json:"size" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
}

type SupermarketSkillIcon struct {
	Card       *SupermarketSkillIconAsset `json:"card,omitempty"`
	Detail     *SupermarketSkillIconAsset `json:"detail,omitempty"`
	Dark       *SupermarketSkillIconAsset `json:"dark,omitempty"`
	BrandColor string                     `json:"brand_color,omitempty"`
}

type SupermarketCatalogSkill struct {
	SchemaVersion  string                   `json:"schema_version" validate:"required"`
	RegistryID     string                   `json:"registry_id" validate:"required"`
	PackageID      string                   `json:"package_id" validate:"required"`
	SkillID        string                   `json:"skill_id" validate:"required"`
	InstallID      string                   `json:"install_id" validate:"required"`
	Name           string                   `json:"name" validate:"required"`
	Description    string                   `json:"description" validate:"required"`
	Author         SupermarketAuthor        `json:"author" validate:"required"`
	Homepage       string                   `json:"homepage,omitempty"`
	Tags           []string                 `json:"tags" validate:"required"`
	Category       string                   `json:"category" validate:"required"`
	CategoryName   string                   `json:"category_name" validate:"required"`
	SourceCategory string                   `json:"source_category,omitempty"`
	Source         SupermarketSkillSource   `json:"source" validate:"required"`
	Files          []string                 `json:"files" validate:"required"`
	Icon           *SupermarketSkillIcon    `json:"icon,omitempty"`
	Artifact       SupermarketSkillArtifact `json:"artifact" validate:"required"`
}

type SupermarketCatalogSkillListResponse struct {
	Total int                       `json:"total" validate:"required"`
	Page  int                       `json:"page" validate:"required"`
	Limit int                       `json:"limit" validate:"required"`
	Data  []SupermarketCatalogSkill `json:"data" validate:"required"`
}

type SupermarketSkillPackageCategory struct {
	ID         string `json:"id" validate:"required"`
	Name       string `json:"name" validate:"required"`
	SkillCount int    `json:"skill_count" validate:"required"`
}

type SupermarketSkillPackageSummary struct {
	SchemaVersion string                            `json:"schema_version" validate:"required"`
	RegistryID    string                            `json:"registry_id" validate:"required"`
	PackageID     string                            `json:"package_id" validate:"required"`
	Name          string                            `json:"name" validate:"required"`
	Description   string                            `json:"description" validate:"required"`
	Tags          []string                          `json:"tags" validate:"required"`
	Categories    []SupermarketSkillPackageCategory `json:"categories" validate:"required"`
	SkillCount    int                               `json:"skill_count" validate:"required"`
	Icon          *SupermarketSkillIcon             `json:"icon,omitempty"`
}

type SupermarketSkillPackageDescriptor struct {
	SupermarketSkillPackageSummary
	Revision   string                    `json:"revision" validate:"required"`
	Skills     []SupermarketCatalogSkill `json:"skills" validate:"required"`
	ReleaseURL string                    `json:"release_url,omitempty"`
}

type supermarketSkillPackageReleaseSkill struct {
	SchemaVersion  string                   `json:"schema_version"`
	RegistryID     string                   `json:"registry_id"`
	PackageID      string                   `json:"package_id"`
	SkillID        string                   `json:"skill_id"`
	InstallID      string                   `json:"install_id"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Author         SupermarketAuthor        `json:"author"`
	Homepage       string                   `json:"homepage,omitempty"`
	Tags           []string                 `json:"tags"`
	Category       string                   `json:"category"`
	CategoryName   string                   `json:"category_name"`
	SourceCategory string                   `json:"source_category,omitempty"`
	Files          []string                 `json:"files"`
	Icon           *SupermarketSkillIcon    `json:"icon,omitempty"`
	Artifact       SupermarketSkillArtifact `json:"artifact"`
}

type SupermarketSkillPackageRelease struct {
	SchemaVersion string                                `json:"schema_version"`
	RegistryID    string                                `json:"registry_id"`
	PackageID     string                                `json:"package_id"`
	Name          string                                `json:"name"`
	Description   string                                `json:"description"`
	Tags          []string                              `json:"tags"`
	Icon          *SupermarketSkillIcon                 `json:"icon,omitempty"`
	Skills        []supermarketSkillPackageReleaseSkill `json:"skills"`
}

type SupermarketSkillPackageListResponse struct {
	Total int                              `json:"total" validate:"required"`
	Page  int                              `json:"page" validate:"required"`
	Limit int                              `json:"limit" validate:"required"`
	Data  []SupermarketSkillPackageSummary `json:"data" validate:"required"`
}

type InstallRegistryPackageResponse struct {
	OK                bool                           `json:"ok" validate:"required"`
	RegistryID        string                         `json:"registry_id" validate:"required"`
	PackageID         string                         `json:"package_id" validate:"required"`
	Revision          string                         `json:"revision" validate:"required"`
	WorkspaceTargetID string                         `json:"workspace_target_id" validate:"required"`
	Skills            []InstallRegistrySkillResponse `json:"skills" validate:"required"`
}

type InstallRegistrySkillResponse struct {
	OK                bool   `json:"ok" validate:"required"`
	RegistryID        string `json:"registry_id" validate:"required"`
	PackageID         string `json:"package_id" validate:"required"`
	SkillID           string `json:"skill_id" validate:"required"`
	InstallID         string `json:"install_id" validate:"required"`
	WorkspaceTargetID string `json:"workspace_target_id" validate:"required"`
	ArtifactDigest    string `json:"artifact_digest" validate:"required"`
	FilesWritten      int    `json:"files_written" validate:"required"`
}

type registrySkillArtifactInstallResult struct {
	Skill        SupermarketCatalogSkill
	FilesWritten int
}

type preparedRegistrySkillArtifact struct {
	Skill       SupermarketCatalogSkill
	Archive     skillset.Archive
	WorkspaceOS string
}

type preparedRegistryPackage struct {
	Descriptor        SupermarketSkillPackageDescriptor
	Skills            []preparedRegistrySkillArtifact
	ExpectedArtifacts map[string]string
}

type registryPackagePublication struct {
	client       *bridge.Client
	packageDir   string
	backupDir    string
	rootMoved    bool
	publications []*skillset.ArchivePublication
}

// ListRegistries godoc
// @Summary List Skill Registries from supermarket
// @Tags supermarket
// @Success 200 {object} SupermarketRegistryListResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/registries [get].
func (h *SupermarketHandler) ListRegistries(c echo.Context) error {
	return h.proxy(c, "/api/registries")
}

// ListRegistryCategories godoc
// @Summary List categories in a Skill Registry
// @Tags supermarket
// @Param registry_id path string true "Registry ID"
// @Success 200 {object} SupermarketSkillCategoryListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/registries/{registry_id}/categories [get].
func (h *SupermarketHandler) ListRegistryCategories(c echo.Context) error {
	registryID, err := requireRegistryComponent(c.Param("registry_id"), "registry_id")
	if err != nil {
		return err
	}
	return h.proxy(c, "/api/registries/"+url.PathEscape(registryID)+"/categories")
}

// ListSkills godoc
// @Summary List Skills across supermarket Registries
// @Tags supermarket
// @Param q query string false "Search query"
// @Param registry query string false "Registry ID"
// @Param package query string false "Package ID"
// @Param category query string false "Category ID"
// @Param tag query string false "Exact tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Param sort query string false "Sort order"
// @Success 200 {object} SupermarketCatalogSkillListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/skills [get].
func (h *SupermarketHandler) ListSkills(c echo.Context) error {
	return h.proxy(c, "/api/skills")
}

// ListPackages godoc
// @Summary List Skill Packages across supermarket Registries
// @Tags supermarket
// @Param q query string false "Search query"
// @Param registry query string false "Registry ID"
// @Param category query string false "Category ID"
// @Param tag query string false "Exact tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Param sort query string false "Sort order"
// @Success 200 {object} SupermarketSkillPackageListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/packages [get].
func (h *SupermarketHandler) ListPackages(c echo.Context) error {
	return h.proxy(c, "/api/packages")
}

// ListRegistryPackages godoc
// @Summary List Skill Packages in one Registry
// @Tags supermarket
// @Param registry_id path string true "Registry ID"
// @Param q query string false "Search query"
// @Param category query string false "Category ID"
// @Param tag query string false "Exact tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Param sort query string false "Sort order"
// @Success 200 {object} SupermarketSkillPackageListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/registries/{registry_id}/packages [get].
func (h *SupermarketHandler) ListRegistryPackages(c echo.Context) error {
	registryID, err := requireRegistryID(c.Param("registry_id"), "registry_id")
	if err != nil {
		return err
	}
	return h.proxy(c, "/api/registries/"+url.PathEscape(registryID)+"/packages")
}

// GetRegistryPackage godoc
// @Summary Get a namespaced Skill Package
// @Tags supermarket
// @Param registry_id path string true "Registry ID"
// @Param package_id path string true "Package ID"
// @Success 200 {object} SupermarketSkillPackageDescriptor
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/registries/{registry_id}/packages/{package_id} [get].
func (h *SupermarketHandler) GetRegistryPackage(c echo.Context) error {
	registryID, err := requireRegistryID(c.Param("registry_id"), "registry_id")
	if err != nil {
		return err
	}
	packageID, err := requireRegistryComponent(c.Param("package_id"), "package_id")
	if err != nil {
		return err
	}
	return h.proxy(c, registryPackageUpstreamPath(registryID, packageID))
}

// GetRegistrySkill godoc
// @Summary Get a namespaced Registry Skill
// @Tags supermarket
// @Param registry_id path string true "Registry ID"
// @Param package_id path string true "Package ID"
// @Param skill_id path string true "Skill ID"
// @Success 200 {object} SupermarketCatalogSkill
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/registries/{registry_id}/packages/{package_id}/skills/{skill_id} [get].
func (h *SupermarketHandler) GetRegistrySkill(c echo.Context) error {
	registryID, packageID, skillID, err := registrySkillIdentity(
		c.Param("registry_id"), c.Param("package_id"), c.Param("skill_id"),
	)
	if err != nil {
		return err
	}
	return h.proxy(c, registrySkillUpstreamPath(registryID, packageID, skillID))
}

// GetRegistrySkillIcon proxies an immutable Skill icon from Supermarket.
// @Summary Get a mirrored Skill icon
// @Tags supermarket
// @Param digest path string true "SHA-256 digest"
// @Success 200 {file} binary
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/artifacts/icon/{digest} [get].
func (h *SupermarketHandler) GetRegistrySkillIcon(c echo.Context) error {
	digest := strings.TrimSpace(c.Param("digest"))
	if len(digest) != sha256.Size*2 {
		return echo.NewHTTPError(http.StatusBadRequest, "digest is invalid")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "digest is invalid")
	}
	return h.proxySkillIcon(c, digest)
}

func (h *SupermarketHandler) proxySkillIcon(c echo.Context, digest string) error {
	req, err := http.NewRequestWithContext(
		c.Request().Context(), http.MethodGet, h.baseURL+"/api/artifacts/icon/"+digest, nil,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "image/svg+xml,image/png,image/jpeg,image/webp")
	if value := c.Request().Header.Get("If-None-Match"); value != "" {
		req.Header.Set("If-None-Match", value)
	}
	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		copySkillIconHeaders(c.Response().Header(), resp.Header)
		return c.NoContent(http.StatusNotModified)
	}
	if resp.StatusCode != http.StatusOK {
		return echo.NewHTTPError(resp.StatusCode, "Skill icon unavailable")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType != "image/svg+xml" && contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return echo.NewHTTPError(http.StatusBadGateway, "Supermarket returned an unsupported Skill icon")
	}
	const maxSkillImageBytes = 512 * 1024
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSkillImageBytes+1))
	if err != nil || len(content) == 0 || len(content) > maxSkillImageBytes {
		return echo.NewHTTPError(http.StatusBadGateway, "Supermarket returned an invalid Skill icon")
	}
	actualDigest := sha256.Sum256(content)
	if hex.EncodeToString(actualDigest[:]) != digest {
		return echo.NewHTTPError(http.StatusBadGateway, "Skill icon digest mismatch")
	}
	copySkillIconHeaders(c.Response().Header(), resp.Header)
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	return c.Blob(http.StatusOK, contentType, content)
}

func copySkillIconHeaders(target, source http.Header) {
	for _, name := range []string{"Cache-Control", "ETag", "X-Content-SHA256"} {
		if value := source.Get(name); value != "" {
			target.Set(name, value)
		}
	}
	// These images are served unauthenticated on our own origin and may be SVG,
	// which can carry script. Enforce a strict policy regardless of what the
	// upstream sent: sandbox blocks script execution on direct navigation and
	// nosniff prevents MIME confusion. Do not forward the upstream CSP — a laxer
	// upstream value must never be able to weaken this.
	target.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	target.Set("X-Content-Type-Options", "nosniff")
}

func requireRegistryComponent(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if !skillset.IsValidRegistryComponent(value) {
		return "", echo.NewHTTPError(http.StatusBadRequest, field+" is invalid")
	}
	return value, nil
}

func requireRegistryID(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if !skillset.IsValidRegistryID(value) {
		return "", echo.NewHTTPError(http.StatusBadRequest, field+" is invalid")
	}
	return value, nil
}

func registrySkillIdentity(registryValue, packageValue, skillValue string) (string, string, string, error) {
	registryID, err := requireRegistryID(registryValue, "registry_id")
	if err != nil {
		return "", "", "", err
	}
	packageID, err := requireRegistryComponent(packageValue, "package_id")
	if err != nil {
		return "", "", "", err
	}
	skillID, err := requireRegistryComponent(skillValue, "skill_id")
	if err != nil {
		return "", "", "", err
	}
	return registryID, packageID, skillID, nil
}

func registrySkillUpstreamPath(registryID, packageID, skillID string) string {
	return "/api/registries/" + url.PathEscape(registryID) + "/packages/" + url.PathEscape(packageID) + "/skills/" + url.PathEscape(skillID)
}

func registryPackageUpstreamPath(registryID, packageID string) string {
	return "/api/registries/" + url.PathEscape(registryID) + "/packages/" + url.PathEscape(packageID)
}

func registryPackageReleaseUpstreamPath(registryID, packageID, revision string) string {
	return registryPackageUpstreamPath(registryID, packageID) + "/releases/" + url.PathEscape(revision)
}

func (h *SupermarketHandler) installRegistryPackage(
	ctx context.Context,
	botID string,
	req InstallPackageRequest,
) (InstallRegistryPackageResponse, error) {
	registryID, err := requireRegistryID(req.RegistryID, "registry_id")
	if err != nil {
		return InstallRegistryPackageResponse{}, err
	}
	packageID, err := requireRegistryComponent(req.PackageID, "package_id")
	if err != nil {
		return InstallRegistryPackageResponse{}, err
	}
	expectedRevision := strings.TrimSpace(req.Revision)
	if !isCanonicalSHA256(expectedRevision) {
		return InstallRegistryPackageResponse{}, echo.NewHTTPError(http.StatusBadRequest, "revision is invalid")
	}
	if h.workspaces == nil {
		return InstallRegistryPackageResponse{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			errors.New("workspace manager is not configured"),
			nil,
		)
	}
	targetContext := workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	target, err := h.workspaces.ResolveWorkspaceTarget(targetContext, botID, req.WorkspaceTargetID)
	if err != nil {
		return InstallRegistryPackageResponse{}, workspaceTargetHTTPError(h.logger, err)
	}
	pkg, err := h.fetchRegistryPackageRelease(targetContext, registryID, packageID, expectedRevision)
	if err != nil {
		return InstallRegistryPackageResponse{}, err
	}
	prepared, err := h.prepareRegistryPackage(targetContext, target.Info.OS, pkg, registryID, packageID, expectedRevision)
	if err != nil {
		return InstallRegistryPackageResponse{}, err
	}
	if h.pluginService != nil {
		if err := h.pluginService.CheckSkillArtifactConflicts(
			targetContext, botID, "", target.TargetID, prepared.ExpectedArtifacts,
		); err != nil {
			return InstallRegistryPackageResponse{}, echo.NewHTTPError(http.StatusConflict, err.Error())
		}
	}

	installed := make([]InstallRegistrySkillResponse, 0, len(prepared.Skills))
	if err := withBotMutation(targetContext, botID, h.pluginService, func(mutationCtx context.Context) error {
		if h.pluginService != nil {
			if conflictErr := h.pluginService.CheckSkillArtifactConflicts(
				mutationCtx, botID, "", target.TargetID, prepared.ExpectedArtifacts,
			); conflictErr != nil {
				return echo.NewHTTPError(http.StatusConflict, conflictErr.Error())
			}
		}
		if err := h.checkPluginPackageMembers(mutationCtx, botID, target.TargetID, "", registryID, packageID, prepared.ExpectedArtifacts); err != nil {
			return err
		}
		publication, published, publishErr := publishPreparedRegistryPackage(mutationCtx, target.Client, prepared, target.TargetID)
		if publishErr != nil {
			return publishErr
		}
		installed = published
		if commitErr := publication.Commit(mutationCtx); commitErr != nil && h.logger != nil {
			h.logger.Warn("cleanup replaced Skill Package failed", slog.Any("error", commitErr))
		}
		return nil
	}); err != nil {
		return InstallRegistryPackageResponse{}, err
	}
	return InstallRegistryPackageResponse{
		OK: true, RegistryID: registryID, PackageID: packageID, Revision: prepared.Descriptor.Revision,
		WorkspaceTargetID: target.TargetID, Skills: installed,
	}, nil
}

func (h *SupermarketHandler) checkPluginPackageMembers(
	ctx context.Context,
	botID, targetID, targetPluginID, registryID, packageID string,
	expected map[string]string,
) error {
	if h.pluginService == nil {
		return nil
	}
	installations, err := h.pluginService.List(ctx, botID)
	if err != nil {
		return err
	}
	for _, installation := range installations {
		if installation.Status == pluginspkg.StatusUninstalled {
			continue
		}
		if targetPluginID != "" && installation.PluginID == targetPluginID {
			continue
		}
		installedTarget, _ := installation.Metadata["workspace_target_id"].(string)
		if strings.TrimSpace(installedTarget) != targetID {
			continue
		}
		for _, resource := range installation.Resources {
			resourceRegistry, resourcePackage, resourceSkill, belongs := skillset.RegistrySkillIDs(resource.ResourceID)
			if resource.Type != "skill" || !belongs || resourceRegistry != registryID || resourcePackage != packageID {
				continue
			}
			identity := strings.Join([]string{registryID, packageID, resourceSkill}, "/")
			if _, included := expected[identity]; !included {
				return echo.NewHTTPError(http.StatusConflict, "An installed Plugin still uses a Skill removed from this Package release")
			}
		}
	}
	return nil
}

func (h *SupermarketHandler) prepareRegistryPackage(
	ctx context.Context,
	workspaceOS string,
	pkg SupermarketSkillPackageDescriptor,
	registryID, packageID, revision string,
) (preparedRegistryPackage, error) {
	if pkg.Revision != revision {
		return preparedRegistryPackage{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid, errors.New("registry Package revision does not match the request"), nil,
		)
	}
	if err := validateRegistryPackage(pkg, registryID, packageID); err != nil {
		return preparedRegistryPackage{}, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
	}
	if err := validateRegistryPackageArtifactBudget(pkg.Skills); err != nil {
		return preparedRegistryPackage{}, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
	}
	prepared := preparedRegistryPackage{
		Descriptor:        pkg,
		Skills:            make([]preparedRegistrySkillArtifact, 0, len(pkg.Skills)),
		ExpectedArtifacts: make(map[string]string, len(pkg.Skills)),
	}
	for _, skill := range pkg.Skills {
		identity := strings.Join([]string{registryID, packageID, skill.SkillID}, "/")
		prepared.ExpectedArtifacts[identity] = skill.Artifact.Digest
		item, err := h.prepareResolvedRegistrySkillArtifact(ctx, workspaceOS, skill)
		if err != nil {
			return preparedRegistryPackage{}, err
		}
		prepared.Skills = append(prepared.Skills, item)
	}
	return prepared, nil
}

func publishPreparedRegistryPackage(
	ctx context.Context,
	client *bridge.Client,
	prepared preparedRegistryPackage,
	workspaceTargetID string,
) (*registryPackagePublication, []InstallRegistrySkillResponse, error) {
	if client == nil {
		return nil, nil, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed, errors.New("workspace is not reachable"), nil,
		)
	}
	registryID := prepared.Descriptor.RegistryID
	packageID := prepared.Descriptor.PackageID
	packageDir, err := skillset.SkillPackageDirForIDs(registryID, packageID)
	if err != nil {
		return nil, nil, err
	}
	publication := &registryPackagePublication{
		client:       client,
		packageDir:   packageDir,
		backupDir:    path.Join(skillset.ManagedDirPath, ".staging", "replace-package-"+uuid.NewString()),
		publications: make([]*skillset.ArchivePublication, 0, len(prepared.Skills)),
	}
	if _, err := client.Stat(ctx, packageDir); err == nil {
		if err := client.Mkdir(ctx, path.Dir(publication.backupDir)); err != nil {
			return nil, nil, err
		}
		if err := client.Rename(ctx, packageDir, publication.backupDir); err != nil {
			return nil, nil, err
		}
		publication.rootMoved = true
	} else if !errors.Is(err, bridge.ErrNotFound) {
		return nil, nil, err
	}

	installed := make([]InstallRegistrySkillResponse, 0, len(prepared.Skills))
	for _, item := range prepared.Skills {
		skillPublication, result, err := publishPreparedRegistrySkillArtifact(ctx, client, item)
		if err != nil {
			if rollbackErr := publication.Rollback(ctx); rollbackErr != nil {
				return nil, nil, errors.Join(err, rollbackErr)
			}
			return nil, nil, err
		}
		publication.publications = append(publication.publications, skillPublication)
		installed = append(installed, InstallRegistrySkillResponse{
			OK: true, RegistryID: registryID, PackageID: packageID, SkillID: result.Skill.SkillID,
			InstallID: result.Skill.InstallID, WorkspaceTargetID: workspaceTargetID,
			ArtifactDigest: result.Skill.Artifact.Digest, FilesWritten: result.FilesWritten,
		})
	}
	return publication, installed, nil
}

func (p *registryPackagePublication) Rollback(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pluginPublicationCleanupTimeout)
	defer cancel()
	var errs []error
	for index := len(p.publications) - 1; index >= 0; index-- {
		if err := p.publications[index].Rollback(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("roll back Package Skill: %w", err))
		}
	}
	if err := p.client.DeleteFile(cleanupCtx, p.packageDir, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		errs = append(errs, fmt.Errorf("remove replacement Package: %w", err))
	}
	if p.rootMoved {
		if err := p.client.Rename(cleanupCtx, p.backupDir, p.packageDir); err != nil {
			errs = append(errs, fmt.Errorf("restore previous Package: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (p *registryPackagePublication) Commit(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pluginPublicationCleanupTimeout)
	defer cancel()
	var errs []error
	if p.rootMoved {
		if err := p.client.DeleteFile(cleanupCtx, p.backupDir, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	for _, publication := range p.publications {
		if err := publication.Commit(cleanupCtx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *SupermarketHandler) prepareResolvedRegistrySkillArtifact(
	ctx context.Context,
	workspaceOS string,
	skill SupermarketCatalogSkill,
) (preparedRegistrySkillArtifact, error) {
	return h.prepareResolvedRegistrySkillArtifactWithLimit(ctx, workspaceOS, skill, int64(^uint64(0)>>1))
}

func (h *SupermarketHandler) prepareResolvedRegistrySkillArtifactWithLimit(
	ctx context.Context,
	workspaceOS string,
	skill SupermarketCatalogSkill,
	maxUncompressedBytes int64,
) (preparedRegistrySkillArtifact, error) {
	return h.prepareResolvedRegistrySkillArtifactWithLimits(
		ctx,
		workspaceOS,
		skill,
		maxUncompressedBytes,
		skill.Artifact.ArchiveSize,
		skill.Artifact.FileCount,
	)
}

func (h *SupermarketHandler) prepareResolvedRegistrySkillArtifactWithLimits(
	ctx context.Context,
	workspaceOS string,
	skill SupermarketCatalogSkill,
	maxUncompressedBytes, maxArchiveBytes int64,
	maxFiles int,
) (preparedRegistrySkillArtifact, error) {
	if skill.Artifact.UncompressedSize < 1 ||
		skill.Artifact.UncompressedSize > maxRegistrySkillArtifactUncompressedBytes {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact uncompressed size is invalid"),
			nil,
		)
	}
	if skill.Artifact.UncompressedSize > maxUncompressedBytes {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact exceeds the remaining uncompressed size limit"),
			nil,
		)
	}
	if skill.Artifact.ArchiveSize < 1 ||
		skill.Artifact.ArchiveSize > maxRegistrySkillArtifactArchiveBytes ||
		skill.Artifact.ArchiveSize > maxArchiveBytes {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact decompressed archive size is invalid"),
			nil,
		)
	}
	if skill.Artifact.FileCount < 1 ||
		skill.Artifact.FileCount > maxRegistrySkillArtifactFiles ||
		skill.Artifact.FileCount > maxFiles {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact file count is invalid"),
			nil,
		)
	}
	select {
	case skillArtifactPreparationTokens <- struct{}{}:
		defer func() { <-skillArtifactPreparationTokens }()
	case <-ctx.Done():
		return preparedRegistrySkillArtifact{}, ctx.Err()
	}
	artifactBytes, err := h.downloadRegistrySkillArtifact(ctx, skill.Artifact)
	if err != nil {
		return preparedRegistrySkillArtifact{}, err
	}
	archive, err := skillset.ReadArchiveWithLimits(
		artifactBytes,
		min(maxUncompressedBytes, skill.Artifact.UncompressedSize),
		min(maxArchiveBytes, skill.Artifact.ArchiveSize),
		min(maxFiles, skill.Artifact.FileCount),
	)
	if err != nil {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
	}
	if archive.UncompressedSize() != skill.Artifact.UncompressedSize {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf(
				"registry skill artifact uncompressed size does not match its descriptor: expected %d, got %d",
				skill.Artifact.UncompressedSize,
				archive.UncompressedSize(),
			),
			nil,
		)
	}
	if archive.ArchiveSize() != skill.Artifact.ArchiveSize {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf(
				"registry skill artifact archive size does not match its descriptor: expected %d, got %d",
				skill.Artifact.ArchiveSize,
				archive.ArchiveSize(),
			),
			nil,
		)
	}
	if archive.FileCount() != skill.Artifact.FileCount {
		return preparedRegistrySkillArtifact{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf(
				"registry skill artifact file count does not match its descriptor: expected %d, got %d",
				skill.Artifact.FileCount,
				archive.FileCount(),
			),
			nil,
		)
	}
	return preparedRegistrySkillArtifact{Skill: skill, Archive: archive, WorkspaceOS: workspaceOS}, nil
}

func publishPreparedRegistrySkillArtifact(
	ctx context.Context,
	client *bridge.Client,
	prepared preparedRegistrySkillArtifact,
) (*skillset.ArchivePublication, registrySkillArtifactInstallResult, error) {
	if client == nil {
		return nil, registrySkillArtifactInstallResult{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			errors.New("workspace is not reachable"),
			nil,
		)
	}
	skill := prepared.Skill
	publication, installErr := skillset.PublishArchive(
		ctx, client, prepared.WorkspaceOS, skill.RegistryID, skill.PackageID, skill.SkillID, prepared.Archive,
	)
	if installErr != nil {
		return nil, registrySkillArtifactInstallResult{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			fmt.Errorf("install registry Skill: %w", installErr),
			nil,
		)
	}
	return publication, registrySkillArtifactInstallResult{
		Skill: skill, FilesWritten: prepared.Archive.FileCount(),
	}, nil
}

func (h *SupermarketHandler) fetchRegistrySkill(
	ctx context.Context,
	registryID, packageID, skillID string,
) (SupermarketCatalogSkill, error) {
	endpoint := strings.TrimRight(h.baseURL, "/") + registrySkillUpstreamPath(registryID, packageID, skillID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SupermarketCatalogSkill{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("create Registry Skill request: %w", err),
			nil,
		)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		return SupermarketCatalogSkill{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("fetch Registry Skill: %w", err),
			nil,
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return SupermarketCatalogSkill{}, apperror.New(apperror.CodeRegistrySkillNotFound, nil)
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketCatalogSkill{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("fetch Registry Skill: Supermarket returned status %d", resp.StatusCode),
			nil,
		)
	}

	var skill SupermarketCatalogSkill
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxRegistrySkillMetadataBytes+1))
	if err := decoder.Decode(&skill); err != nil {
		return SupermarketCatalogSkill{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf("decode Registry Skill response: %w", err),
			nil,
		)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return SupermarketCatalogSkill{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill response is too large or malformed"),
			nil,
		)
	}
	return skill, nil
}

func (h *SupermarketHandler) fetchRegistryPackageRelease(
	ctx context.Context,
	registryID, packageID, revision string,
) (SupermarketSkillPackageDescriptor, error) {
	endpoint := strings.TrimRight(h.baseURL, "/") + registryPackageReleaseUpstreamPath(registryID, packageID, revision)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable, fmt.Errorf("create Registry Package release request: %w", err), nil,
		)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable, fmt.Errorf("fetch Registry Package release: %w", err), nil,
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return SupermarketSkillPackageDescriptor{}, apperror.New(apperror.CodeRegistrySkillNotFound, nil)
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("fetch Registry Package release: Supermarket returned status %d", resp.StatusCode), nil,
		)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryPackageMetadataBytes+1))
	if err != nil {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistryUnavailable, fmt.Errorf("read Registry Package release: %w", err), nil,
		)
	}
	if len(payload) > maxRegistryPackageMetadataBytes {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid, errors.New("registry Package release is too large"), nil,
		)
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != revision {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid, errors.New("registry Package release SHA-256 verification failed"), nil,
		)
	}
	var release SupermarketSkillPackageRelease
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&release); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return SupermarketSkillPackageDescriptor{}, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid, errors.New("registry Package release is malformed"), nil,
		)
	}
	skills := make([]SupermarketCatalogSkill, 0, len(release.Skills))
	for _, member := range release.Skills {
		member.Artifact.DownloadURL = "/api/artifacts/skill/" + member.Artifact.Digest
		skills = append(skills, SupermarketCatalogSkill{
			SchemaVersion: member.SchemaVersion, RegistryID: member.RegistryID, PackageID: member.PackageID,
			SkillID: member.SkillID, InstallID: member.InstallID, Name: member.Name,
			Description: member.Description, Author: member.Author, Homepage: member.Homepage,
			Tags: member.Tags, Category: member.Category, CategoryName: member.CategoryName,
			SourceCategory: member.SourceCategory, Files: member.Files, Icon: member.Icon, Artifact: member.Artifact,
		})
	}
	return SupermarketSkillPackageDescriptor{
		SupermarketSkillPackageSummary: SupermarketSkillPackageSummary{
			SchemaVersion: release.SchemaVersion,
			RegistryID:    release.RegistryID,
			PackageID:     release.PackageID,
			Name:          release.Name,
			Description:   release.Description,
			Tags:          release.Tags,
			SkillCount:    len(release.Skills),
			Icon:          release.Icon,
		},
		Revision: revision,
		Skills:   skills,
	}, nil
}

func validateRegistryPackage(pkg SupermarketSkillPackageDescriptor, registryID, packageID string) error {
	if pkg.SchemaVersion != "1" {
		return errors.New("registry Package schema is unsupported")
	}
	if pkg.RegistryID != registryID || pkg.PackageID != packageID {
		return errors.New("registry Package identity does not match the request")
	}
	if !isCanonicalSHA256(pkg.Revision) {
		return errors.New("registry Package revision is invalid")
	}
	if len(pkg.Skills) == 0 || len(pkg.Skills) > maxRegistryPackageSkills || pkg.SkillCount != len(pkg.Skills) {
		return errors.New("registry Package Skill list is invalid")
	}
	seen := make(map[string]struct{}, len(pkg.Skills))
	for _, skill := range pkg.Skills {
		if _, exists := seen[skill.SkillID]; exists {
			return errors.New("registry Package contains duplicate Skills")
		}
		seen[skill.SkillID] = struct{}{}
		if err := validateRegistrySkill(skill, registryID, packageID, skill.SkillID); err != nil {
			return err
		}
	}
	return nil
}

func validateRegistryPackageArtifactBudget(skills []SupermarketCatalogSkill) error {
	var compressedBytes, uncompressedBytes, archiveBytes int64
	files := 0
	for _, skill := range skills {
		artifact := skill.Artifact
		if artifact.Size > int64(maxPluginSkillArtifactsCompressedBytes)-compressedBytes ||
			artifact.UncompressedSize > int64(maxPluginSkillArtifactsUncompressedBytes)-uncompressedBytes ||
			artifact.ArchiveSize > int64(maxPluginSkillArtifactsArchiveBytes)-archiveBytes ||
			artifact.FileCount > maxPluginSkillArtifactFiles-files {
			return errors.New("registry Package exceeds the aggregate Artifact limits")
		}
		compressedBytes += artifact.Size
		uncompressedBytes += artifact.UncompressedSize
		archiveBytes += artifact.ArchiveSize
		files += artifact.FileCount
	}
	return nil
}

func validateRegistrySkill(skill SupermarketCatalogSkill, registryID, packageID, skillID string) error {
	expectedInstallID := strings.Join([]string{registryID, packageID, skillID}, "+")
	if skill.RegistryID != registryID || skill.PackageID != packageID || skill.SkillID != skillID {
		return errors.New("registry Skill identity does not match the request")
	}
	if skill.InstallID != expectedInstallID || !skillset.IsValidName(skill.InstallID) {
		return errors.New("registry Skill install_id is invalid")
	}
	artifact := skill.Artifact
	if artifact.Format != "memoh_skill_v1" || artifact.ContentType != "application/gzip" {
		return errors.New("registry Skill Artifact format is unsupported")
	}
	if len(artifact.Digest) != sha256.Size*2 {
		return errors.New("registry Skill Artifact digest is invalid")
	}
	if _, err := hex.DecodeString(artifact.Digest); err != nil {
		return errors.New("registry Skill Artifact digest is invalid")
	}
	if artifact.Size < 1 || artifact.Size > maxRegistrySkillArtifactCompressedBytes {
		return errors.New("registry Skill Artifact size is invalid")
	}
	if artifact.UncompressedSize < 1 || artifact.UncompressedSize > maxRegistrySkillArtifactUncompressedBytes {
		return errors.New("registry Skill Artifact uncompressed size is invalid")
	}
	if artifact.ArchiveSize < 1 || artifact.ArchiveSize > maxRegistrySkillArtifactArchiveBytes {
		return errors.New("registry Skill Artifact archive size is invalid")
	}
	if artifact.FileCount < 1 || artifact.FileCount > maxRegistrySkillArtifactFiles {
		return errors.New("registry Skill Artifact file count is invalid")
	}
	if strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("registry Skill Artifact download URL is missing")
	}
	return nil
}

func (h *SupermarketHandler) downloadRegistrySkillArtifact(
	ctx context.Context,
	artifact SupermarketSkillArtifact,
) ([]byte, error) {
	return h.downloadSupermarketArtifact(ctx, supermarketArtifactDownloadDescriptor{
		Digest: artifact.Digest, Size: artifact.Size, DownloadURL: artifact.DownloadURL,
	})
}

func (h *SupermarketHandler) downloadSupermarketArtifact(
	ctx context.Context,
	artifact supermarketArtifactDownloadDescriptor,
) ([]byte, error) {
	base, err := url.Parse(h.baseURL)
	if err != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("parse configured Supermarket URL: %w", err),
			nil,
		)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			errors.New("configured Supermarket URL is invalid"),
			nil,
		)
	}
	download, err := base.Parse(artifact.DownloadURL)
	if err != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf("parse Registry Skill Artifact URL: %w", err),
			nil,
		)
	}
	if download.Scheme != base.Scheme || !strings.EqualFold(download.Host, base.Host) || download.User != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact URL is not on the configured Supermarket origin"),
			nil,
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.String(), nil)
	if err != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			fmt.Errorf("create Registry Skill Artifact request: %w", err),
			nil,
		)
	}
	req.Header.Set("Accept", "application/gzip")
	resp, err := h.doSupermarketRequest(req)
	if err != nil {
		code := apperror.CodeRegistryUnavailable
		if errors.Is(err, errSupermarketRedirectOrigin) || errors.Is(err, errSupermarketRedirectLimit) {
			code = apperror.CodeRegistrySkillInvalid
		}
		return nil, apperror.Wrap(code, fmt.Errorf("download Registry Skill Artifact: %w", err), nil)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact was not found"),
			nil,
		)
	}
	if resp.StatusCode != http.StatusOK {
		code := apperror.CodeRegistrySkillInvalid
		if resp.StatusCode >= http.StatusInternalServerError {
			code = apperror.CodeRegistryUnavailable
		}
		return nil, apperror.Wrap(
			code,
			fmt.Errorf("download Registry Skill Artifact: Supermarket returned status %d", resp.StatusCode),
			nil,
		)
	}
	if resp.Request == nil || resp.Request.URL.Scheme != base.Scheme || !strings.EqualFold(resp.Request.URL.Host, base.Host) {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact response left the configured Supermarket origin"),
			nil,
		)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != artifact.Size {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact content length does not match its descriptor"),
			nil,
		)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, artifact.Size+1))
	if err != nil {
		return nil, apperror.Wrap(
			apperror.CodeRegistryUnavailable,
			fmt.Errorf("read Registry Skill Artifact: %w", err),
			nil,
		)
	}
	if int64(len(content)) != artifact.Size {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact size does not match its descriptor"),
			nil,
		)
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != artifact.Digest {
		return nil, apperror.Wrap(
			apperror.CodeRegistrySkillInvalid,
			errors.New("registry skill artifact SHA-256 verification failed"),
			nil,
		)
	}
	return content, nil
}
