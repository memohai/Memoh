package handlers

import (
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
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxRegistrySkillArtifactCompressedBytes   = 25 * 1024 * 1024
	maxRegistrySkillArtifactUncompressedBytes = 5 * 1024 * 1024
	maxRegistrySkillMetadataBytes             = 2 * 1024 * 1024
)

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

type SupermarketSkillRuntimeRequirements struct {
	OS []string `json:"os" validate:"required"`
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
	SchemaVersion       string                              `json:"schema_version" validate:"required"`
	RegistryID          string                              `json:"registry_id" validate:"required"`
	PackageID           string                              `json:"package_id" validate:"required"`
	SkillID             string                              `json:"skill_id" validate:"required"`
	InstallID           string                              `json:"install_id" validate:"required"`
	Name                string                              `json:"name" validate:"required"`
	Description         string                              `json:"description" validate:"required"`
	Author              SupermarketAuthor                   `json:"author" validate:"required"`
	Homepage            string                              `json:"homepage,omitempty"`
	Tags                []string                            `json:"tags" validate:"required"`
	Category            string                              `json:"category" validate:"required"`
	CategoryName        string                              `json:"category_name" validate:"required"`
	SourceCategory      string                              `json:"source_category,omitempty"`
	RuntimeRequirements SupermarketSkillRuntimeRequirements `json:"runtime_requirements"`
	Source              SupermarketSkillSource              `json:"source" validate:"required"`
	Files               []string                            `json:"files" validate:"required"`
	Icon                *SupermarketSkillIcon               `json:"icon,omitempty"`
	Artifact            SupermarketSkillArtifact            `json:"artifact" validate:"required"`
}

type SupermarketCatalogSkillListResponse struct {
	Total int                       `json:"total" validate:"required"`
	Page  int                       `json:"page" validate:"required"`
	Limit int                       `json:"limit" validate:"required"`
	Data  []SupermarketCatalogSkill `json:"data" validate:"required"`
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
// @Param os query string false "Target OS"
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

func (h *SupermarketHandler) installRegistrySkill(
	ctx context.Context,
	botID string,
	req InstallSkillRequest,
) (InstallRegistrySkillResponse, error) {
	registryID, packageID, skillID, err := registrySkillIdentity(req.RegistryID, req.PackageID, req.SkillID)
	if err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	if h.workspaces == nil {
		return InstallRegistrySkillResponse{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			errors.New("workspace manager is not configured"),
			nil,
		)
	}
	expectedArtifactDigest := strings.TrimSpace(req.ArtifactDigest)
	if !isCanonicalSHA256(expectedArtifactDigest) {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusBadRequest, "artifact_digest is invalid")
	}

	targetContext := workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	target, err := h.workspaces.ResolveWorkspaceTarget(targetContext, botID, req.WorkspaceTargetID)
	if err != nil {
		return InstallRegistrySkillResponse{}, workspaceTargetHTTPError(h.logger, err)
	}
	skill, err := h.fetchRegistrySkill(targetContext, registryID, packageID, skillID)
	if err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	if err := validateRegistrySkill(skill, registryID, packageID, skillID); err != nil {
		return InstallRegistrySkillResponse{}, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
	}
	if skill.Artifact.Digest != expectedArtifactDigest {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(
			http.StatusConflict, "Skill Artifact changed; refresh before installing",
		)
	}
	identity := strings.Join([]string{registryID, packageID, skillID}, "/")
	if h.pluginService != nil {
		if err := h.pluginService.CheckSkillArtifactConflicts(
			targetContext, botID, "", target.TargetID, map[string]string{identity: skill.Artifact.Digest},
		); err != nil {
			return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusConflict, err.Error())
		}
	}
	prepared, err := h.prepareResolvedRegistrySkillArtifact(targetContext, target.Info.OS, skill)
	if err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	var installed registrySkillArtifactInstallResult
	if err := withBotMutation(targetContext, botID, h.pluginService, func(mutationCtx context.Context) error {
		if h.pluginService != nil {
			if conflictErr := h.pluginService.CheckSkillArtifactConflicts(
				mutationCtx, botID, "", target.TargetID, map[string]string{identity: skill.Artifact.Digest},
			); conflictErr != nil {
				return echo.NewHTTPError(http.StatusConflict, conflictErr.Error())
			}
		}
		publication, published, publishErr := publishPreparedRegistrySkillArtifact(
			mutationCtx, target.Client, true, prepared,
		)
		if publishErr != nil {
			return publishErr
		}
		installed = published
		if commitErr := publication.Commit(mutationCtx); commitErr != nil && h.logger != nil {
			h.logger.Warn("cleanup Registry Skill backup failed", slog.Any("error", commitErr))
		}
		return nil
	}); err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	return InstallRegistrySkillResponse{
		OK:                true,
		RegistryID:        registryID,
		PackageID:         packageID,
		SkillID:           skillID,
		InstallID:         installed.Skill.InstallID,
		WorkspaceTargetID: target.TargetID,
		ArtifactDigest:    installed.Skill.Artifact.Digest,
		FilesWritten:      installed.FilesWritten,
	}, nil
}

func (h *SupermarketHandler) installRegistrySkillArtifact(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS string,
	directOwner bool,
	registryID, packageID, skillID string,
) (registrySkillArtifactInstallResult, error) {
	if client == nil {
		return registrySkillArtifactInstallResult{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			errors.New("workspace is not reachable"),
			nil,
		)
	}
	skill, err := h.fetchRegistrySkill(ctx, registryID, packageID, skillID)
	if err != nil {
		return registrySkillArtifactInstallResult{}, err
	}
	if err := validateRegistrySkill(skill, registryID, packageID, skillID); err != nil {
		return registrySkillArtifactInstallResult{}, apperror.Wrap(apperror.CodeRegistrySkillInvalid, err, nil)
	}
	return h.installResolvedRegistrySkillArtifact(ctx, client, workspaceOS, directOwner, skill)
}

func (h *SupermarketHandler) installResolvedRegistrySkillArtifact(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS string,
	directOwner bool,
	skill SupermarketCatalogSkill,
) (registrySkillArtifactInstallResult, error) {
	if client == nil {
		return registrySkillArtifactInstallResult{}, apperror.Wrap(
			apperror.CodeRegistrySkillInstallFailed,
			errors.New("workspace is not reachable"),
			nil,
		)
	}
	prepared, err := h.prepareResolvedRegistrySkillArtifact(ctx, workspaceOS, skill)
	if err != nil {
		return registrySkillArtifactInstallResult{}, err
	}
	publication, installed, err := publishPreparedRegistrySkillArtifact(ctx, client, directOwner, prepared)
	if err != nil {
		return registrySkillArtifactInstallResult{}, err
	}
	if err := publication.Commit(ctx); err != nil && h.logger != nil {
		h.logger.Warn("cleanup Registry Skill backup failed", slog.Any("error", err))
	}
	return installed, nil
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
	if !registrySkillSupportsOS(skill.RuntimeRequirements, workspaceOS) {
		return preparedRegistrySkillArtifact{}, apperror.New(
			apperror.CodeRegistrySkillIncompatible,
			map[string]string{
				"os":           normalizeRegistrySkillOSLabel(workspaceOS),
				"supported_os": strings.Join(normalizedRegistrySkillOSList(skill.RuntimeRequirements.OS), ","),
			},
		)
	}
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
	artifactBytes, err := h.downloadRegistrySkillArtifact(ctx, skill.Artifact)
	if err != nil {
		return preparedRegistrySkillArtifact{}, err
	}
	archive, err := skillset.ReadArchiveWithUncompressedLimit(
		artifactBytes,
		min(maxUncompressedBytes, skill.Artifact.UncompressedSize),
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
	return preparedRegistrySkillArtifact{Skill: skill, Archive: archive, WorkspaceOS: workspaceOS}, nil
}

func publishPreparedRegistrySkillArtifact(
	ctx context.Context,
	client *bridge.Client,
	directOwner bool,
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
	var (
		publication *skillset.ArchivePublication
		installErr  error
	)
	if directOwner {
		publication, installErr = skillset.PublishArchiveWithDirectOwner(
			ctx, client, prepared.WorkspaceOS, skill.RegistryID, skill.PackageID, skill.SkillID,
			prepared.Archive, skill.Artifact.Digest,
		)
	} else {
		publication, installErr = skillset.PublishArchive(
			ctx, client, prepared.WorkspaceOS, skill.RegistryID, skill.PackageID, skill.SkillID, prepared.Archive,
		)
	}
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

func validateRegistrySkill(skill SupermarketCatalogSkill, registryID, packageID, skillID string) error {
	expectedInstallID := strings.Join([]string{registryID, packageID, skillID}, "+")
	if skill.RegistryID != registryID || skill.PackageID != packageID || skill.SkillID != skillID {
		return errors.New("registry Skill identity does not match the request")
	}
	if skill.InstallID != expectedInstallID || !skillset.IsValidName(skill.InstallID) {
		return errors.New("registry Skill install_id is invalid")
	}
	for _, runtimeOS := range skill.RuntimeRequirements.OS {
		if _, ok := normalizeRegistrySkillOS(runtimeOS); !ok {
			return fmt.Errorf("registry Skill runtime requirement OS %q is unsupported", runtimeOS)
		}
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
	if strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("registry Skill Artifact download URL is missing")
	}
	return nil
}

func normalizeRegistrySkillOS(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "darwin":
		return "darwin", true
	case "linux":
		return "linux", true
	case "windows", "win32":
		return "win32", true
	default:
		return "", false
	}
}

func normalizeRegistrySkillOSLabel(value string) string {
	if normalized, ok := normalizeRegistrySkillOS(value); ok {
		return normalized
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedRegistrySkillOSList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized, ok := normalizeRegistrySkillOS(value)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func registrySkillSupportsOS(requirements SupermarketSkillRuntimeRequirements, workspaceOS string) bool {
	if len(requirements.OS) == 0 {
		return true
	}
	targetOS, ok := normalizeRegistrySkillOS(workspaceOS)
	if !ok {
		return false
	}
	for _, supportedOS := range normalizedRegistrySkillOSList(requirements.OS) {
		if supportedOS == targetOS {
			return true
		}
	}
	return false
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
