package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"

	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxRegistrySkillArtifactCompressedBytes   = 25 * 1024 * 1024
	maxRegistrySkillArtifactUncompressedBytes = 100 * 1024 * 1024
	maxRegistrySkillArtifactFiles             = 10_000
	maxRegistrySkillMetadataBytes             = 2 * 1024 * 1024
)

var registryComponentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type SupermarketRegistryListResponse struct {
	Data []SupermarketRegistry `json:"data"`
}

type SupermarketRegistry struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	Priority               int    `json:"priority"`
	Revision               string `json:"revision,omitempty"`
	SyncedAt               string `json:"synced_at,omitempty"`
	RefreshIntervalSeconds int    `json:"refresh_interval_seconds"`
	NextRefreshAt          string `json:"next_refresh_at,omitempty"`
	Status                 string `json:"status"`
	LastError              string `json:"last_error,omitempty"`
}

type SupermarketSkillCategoryListResponse struct {
	Data []SupermarketSkillCategory `json:"data"`
}

type SupermarketSkillCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SupermarketSkillRuntimeRequirements struct {
	OS []string `json:"os"`
}

type SupermarketSkillSource struct {
	Type       string `json:"type"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	Repository string `json:"repository,omitempty"`
}

type SupermarketSkillArtifact struct {
	RegistryID     string `json:"registry_id"`
	PackageID      string `json:"package_id"`
	SkillID        string `json:"skill_id"`
	SourceRevision string `json:"source_revision"`
	Format         string `json:"format"`
	Digest         string `json:"digest"`
	Size           int64  `json:"size"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type"`
	CreatedAt      string `json:"created_at"`
	DownloadURL    string `json:"download_url"`
}

type SupermarketSkillImage struct {
	Digest      string `json:"digest"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

type SupermarketSkillIcon struct {
	Card       *SupermarketSkillImage `json:"card,omitempty"`
	Detail     *SupermarketSkillImage `json:"detail,omitempty"`
	Dark       *SupermarketSkillImage `json:"dark,omitempty"`
	BrandColor string                 `json:"brand_color,omitempty"`
}

type SupermarketCatalogSkill struct {
	SchemaVersion       string                              `json:"schema_version"`
	RegistryID          string                              `json:"registry_id"`
	PackageID           string                              `json:"package_id"`
	SkillID             string                              `json:"skill_id"`
	InstallID           string                              `json:"install_id"`
	Name                string                              `json:"name"`
	Description         string                              `json:"description"`
	Author              SupermarketAuthor                   `json:"author"`
	Homepage            string                              `json:"homepage,omitempty"`
	Tags                []string                            `json:"tags"`
	Category            string                              `json:"category"`
	CategoryName        string                              `json:"category_name"`
	SourceCategory      string                              `json:"source_category,omitempty"`
	RuntimeRequirements SupermarketSkillRuntimeRequirements `json:"runtime_requirements"`
	Source              SupermarketSkillSource              `json:"source"`
	Files               []string                            `json:"files"`
	Icon                *SupermarketSkillIcon               `json:"icon,omitempty"`
	Artifact            SupermarketSkillArtifact            `json:"artifact"`
}

type SupermarketCatalogSkillListResponse struct {
	Total int                       `json:"total"`
	Page  int                       `json:"page"`
	Limit int                       `json:"limit"`
	Data  []SupermarketCatalogSkill `json:"data"`
}

type InstallRegistrySkillResponse struct {
	OK                bool   `json:"ok"`
	RegistryID        string `json:"registry_id"`
	PackageID         string `json:"package_id"`
	SkillID           string `json:"skill_id"`
	InstallID         string `json:"install_id"`
	WorkspaceTargetID string `json:"workspace_target_id"`
	ArtifactDigest    string `json:"artifact_digest"`
	FilesWritten      int    `json:"files_written"`
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

// SearchRegistrySkills godoc
// @Summary Search Skills across supermarket Registries
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
// @Router /supermarket/catalog/skills [get].
func (h *SupermarketHandler) SearchRegistrySkills(c echo.Context) error {
	return h.proxy(c, "/api/catalog/skills")
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

// GetRegistrySkillImage proxies an immutable Skill image from Supermarket.
// @Summary Get a mirrored Skill image
// @Tags supermarket
// @Param digest path string true "SHA-256 digest"
// @Success 200 {file} binary
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /supermarket/skill-images/{digest} [get].
func (h *SupermarketHandler) GetRegistrySkillImage(c echo.Context) error {
	digest := strings.TrimSpace(c.Param("digest"))
	if len(digest) != sha256.Size*2 {
		return echo.NewHTTPError(http.StatusBadRequest, "digest is invalid")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "digest is invalid")
	}
	return h.proxySkillImage(c, digest)
}

func (h *SupermarketHandler) proxySkillImage(c echo.Context, digest string) error {
	req, err := http.NewRequestWithContext(
		c.Request().Context(), http.MethodGet, h.baseURL+"/api/skill-images/"+digest, nil,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "image/svg+xml,image/png,image/jpeg,image/webp")
	if value := c.Request().Header.Get("If-None-Match"); value != "" {
		req.Header.Set("If-None-Match", value)
	}
	resp, err := h.httpClient.Do(req) //nolint:gosec // URL is based on trusted Supermarket configuration.
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		copySkillImageHeaders(c.Response().Header(), resp.Header)
		return c.NoContent(http.StatusNotModified)
	}
	if resp.StatusCode != http.StatusOK {
		return echo.NewHTTPError(resp.StatusCode, "Skill image unavailable")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType != "image/svg+xml" && contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return echo.NewHTTPError(http.StatusBadGateway, "Supermarket returned an unsupported Skill image")
	}
	const maxSkillImageBytes = 512 * 1024
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSkillImageBytes+1))
	if err != nil || len(content) == 0 || len(content) > maxSkillImageBytes {
		return echo.NewHTTPError(http.StatusBadGateway, "Supermarket returned an invalid Skill image")
	}
	actualDigest := sha256.Sum256(content)
	if hex.EncodeToString(actualDigest[:]) != digest {
		return echo.NewHTTPError(http.StatusBadGateway, "Skill image digest mismatch")
	}
	copySkillImageHeaders(c.Response().Header(), resp.Header)
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	return c.Blob(http.StatusOK, contentType, content)
}

func copySkillImageHeaders(target, source http.Header) {
	for _, name := range []string{"Cache-Control", "Content-Security-Policy", "ETag", "X-Content-SHA256", "X-Content-Type-Options"} {
		if value := source.Get(name); value != "" {
			target.Set(name, value)
		}
	}
}

func requireRegistryComponent(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if !registryComponentPattern.MatchString(value) {
		return "", echo.NewHTTPError(http.StatusBadRequest, field+" is invalid")
	}
	return value, nil
}

func registrySkillIdentity(registryValue, packageValue, skillValue string) (string, string, string, error) {
	registryID, err := requireRegistryComponent(registryValue, "registry_id")
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
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusInternalServerError, "workspace manager is not configured")
	}

	targetContext := workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	target, err := h.workspaces.ResolveWorkspaceTarget(targetContext, botID, req.WorkspaceTargetID)
	if err != nil {
		return InstallRegistrySkillResponse{}, workspaceTargetHTTPError(h.logger, err)
	}

	skill, err := h.fetchRegistrySkill(ctx, registryID, packageID, skillID)
	if err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	if err := validateRegistrySkill(skill, registryID, packageID, skillID); err != nil {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if !supportsWorkspaceOS(skill.RuntimeRequirements.OS, target.Info.OS) {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(
			http.StatusConflict,
			fmt.Sprintf("skill %q does not support workspace OS %q", skill.InstallID, target.Info.OS),
		)
	}

	artifactBytes, err := h.downloadRegistrySkillArtifact(ctx, skill.Artifact)
	if err != nil {
		return InstallRegistrySkillResponse{}, err
	}
	archive, err := readRegistrySkillArchive(artifactBytes, skill.InstallID)
	if err != nil {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if err := installRegistrySkillArchive(targetContext, target.Client, target.Info.OS, archive); err != nil {
		return InstallRegistrySkillResponse{}, echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("install skill: %v", err))
	}

	return InstallRegistrySkillResponse{
		OK:                true,
		RegistryID:        registryID,
		PackageID:         packageID,
		SkillID:           skillID,
		InstallID:         skill.InstallID,
		WorkspaceTargetID: target.TargetID,
		ArtifactDigest:    skill.Artifact.Digest,
		FilesWritten:      len(archive.files),
	}, nil
}

func (h *SupermarketHandler) fetchRegistrySkill(
	ctx context.Context,
	registryID, packageID, skillID string,
) (SupermarketCatalogSkill, error) {
	endpoint := strings.TrimRight(h.baseURL, "/") + registrySkillUpstreamPath(registryID, packageID, skillID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpClient.Do(req) //nolint:gosec // Endpoint is built from configured Supermarket URL.
	if err != nil {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusBadGateway, "supermarket unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusNotFound, "registry skill not found")
	}
	if resp.StatusCode != http.StatusOK {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned status %d", resp.StatusCode))
	}

	var skill SupermarketCatalogSkill
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxRegistrySkillMetadataBytes+1))
	if err := decoder.Decode(&skill); err != nil {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusBadGateway, "invalid Registry Skill response")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return SupermarketCatalogSkill{}, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill response is too large or malformed")
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
	artifact := skill.Artifact
	if artifact.RegistryID != registryID || artifact.PackageID != packageID || artifact.SkillID != skillID {
		return errors.New("registry Skill Artifact identity does not match the request")
	}
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
	if strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("registry Skill Artifact download URL is missing")
	}
	return nil
}

func supportsWorkspaceOS(supported []string, workspaceOS string) bool {
	normalized := strings.ToLower(strings.TrimSpace(workspaceOS))
	if normalized == "windows" {
		normalized = "win32"
	}
	for _, candidate := range supported {
		if strings.EqualFold(strings.TrimSpace(candidate), normalized) {
			return true
		}
	}
	return false
}

func (h *SupermarketHandler) downloadRegistrySkillArtifact(
	ctx context.Context,
	artifact SupermarketSkillArtifact,
) ([]byte, error) {
	base, err := url.Parse(h.baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Supermarket URL is invalid")
	}
	download, err := base.Parse(artifact.DownloadURL)
	if err != nil || download.Scheme != base.Scheme || !strings.EqualFold(download.Host, base.Host) || download.User != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact URL is not on the configured Supermarket origin")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.String(), nil)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	req.Header.Set("Accept", "application/gzip")
	client := *h.httpClient
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		if next.URL.Scheme != base.Scheme || !strings.EqualFold(next.URL.Host, base.Host) || next.URL.User != nil {
			return errors.New("artifact redirect left the configured Supermarket origin")
		}
		return nil
	}
	resp, err := client.Do(req) //nolint:gosec // URL and every redirect are restricted to the configured origin.
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact download failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Registry Skill Artifact not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("supermarket returned Artifact status %d", resp.StatusCode))
	}
	if resp.Request.URL.Scheme != base.Scheme || !strings.EqualFold(resp.Request.URL.Host, base.Host) {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact response left the configured Supermarket origin")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != artifact.Size {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact content length does not match its descriptor")
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, artifact.Size+1))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact download failed")
	}
	if int64(len(content)) != artifact.Size {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact size does not match its descriptor")
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != artifact.Digest {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "Registry Skill Artifact SHA-256 verification failed")
	}
	return content, nil
}

type registrySkillArchiveFile struct {
	path       string
	content    []byte
	executable bool
}

type registrySkillArchive struct {
	installID string
	files     []registrySkillArchiveFile
}

func readRegistrySkillArchive(content []byte, installID string) (registrySkillArchive, error) {
	gz, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return registrySkillArchive{}, errors.New("registry Skill Artifact is not valid gzip")
	}
	defer func() { _ = gz.Close() }()

	archive := registrySkillArchive{installID: installID}
	seen := make(map[string]bool)
	var totalSize int64
	hasManifest := false
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains invalid tar data: %w", err)
		}
		if header.Name == "" || path.IsAbs(header.Name) || strings.Contains(header.Name, `\`) {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains unsafe path %q", header.Name)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.Clean(name) != name {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains non-canonical path %q", header.Name)
		}
		parts := strings.Split(name, "/")
		if parts[0] != installID || containsUnsafePathPart(parts) {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact path %q is outside install_id", header.Name)
		}
		if _, exists := seen[name]; exists {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains duplicate path %q", header.Name)
		}
		seen[name] = header.Typeflag == tar.TypeReg
		if err := rejectArchivePathConflict(seen, name); err != nil {
			return registrySkillArchive{}, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		case tar.TypeSymlink, tar.TypeLink:
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains a link at %q", header.Name)
		default:
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains unsupported entry %q", header.Name)
		}
		if len(parts) == 1 {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact root %q must be a directory", header.Name)
		}
		if len(archive.files) >= maxRegistrySkillArtifactFiles {
			return registrySkillArchive{}, errors.New("registry Skill Artifact exceeds the file limit")
		}
		if header.Size < 0 || header.Size > maxRegistrySkillArtifactUncompressedBytes-totalSize {
			return registrySkillArchive{}, errors.New("registry Skill Artifact exceeds the uncompressed size limit")
		}
		fileContent, err := io.ReadAll(io.LimitReader(tr, header.Size+1))
		if err != nil || int64(len(fileContent)) != header.Size {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact file %q is truncated", header.Name)
		}
		totalSize += int64(len(fileContent))
		relativePath := strings.Join(parts[1:], "/")
		if relativePath == "SKILL.md" {
			fileContent, err = rewriteRegistrySkillManifestName(fileContent, installID)
			if err != nil {
				return registrySkillArchive{}, err
			}
			hasManifest = true
		}
		archive.files = append(archive.files, registrySkillArchiveFile{
			path: relativePath, content: fileContent, executable: header.FileInfo().Mode().Perm()&0o111 != 0,
		})
	}
	if len(archive.files) == 0 {
		return registrySkillArchive{}, errors.New("registry Skill Artifact is empty")
	}
	if !hasManifest {
		return registrySkillArchive{}, errors.New("registry Skill Artifact does not contain a root SKILL.md")
	}
	sort.Slice(archive.files, func(i, j int) bool { return archive.files[i].path < archive.files[j].path })
	return archive, nil
}

func containsUnsafePathPart(parts []string) bool {
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

func rejectArchivePathConflict(seen map[string]bool, name string) error {
	for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if seen[parent] {
			return fmt.Errorf("registry Skill Artifact path %q is nested below file %q", name, parent)
		}
	}
	if !seen[name] {
		return nil
	}
	prefix := name + "/"
	for candidate := range seen {
		if strings.HasPrefix(candidate, prefix) {
			return fmt.Errorf("registry Skill Artifact file %q conflicts with child path %q", name, candidate)
		}
	}
	return nil
}

func rewriteRegistrySkillManifestName(content []byte, installID string) ([]byte, error) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, errors.New("registry Skill SKILL.md is missing YAML frontmatter")
	}
	rest := normalized[4:]
	closing := strings.Index(rest, "\n---\n")
	closingDelimiterLength := len("\n---\n")
	if closing < 0 && strings.HasSuffix(rest, "\n---") {
		closing = len(rest) - len("\n---")
		closingDelimiterLength = len("\n---")
	}
	if closing < 0 {
		return nil, errors.New("registry Skill SKILL.md has malformed YAML frontmatter")
	}
	frontmatter := rest[:closing]
	bodyStart := 4 + closing + closingDelimiterLength

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("registry Skill SKILL.md has invalid YAML frontmatter")
	}
	mapping := document.Content[0]
	nameUpdated := false
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == "name" {
			mapping.Content[index+1].Kind = yaml.ScalarNode
			mapping.Content[index+1].Tag = "!!str"
			mapping.Content[index+1].Value = installID
			nameUpdated = true
			break
		}
	}
	if !nameUpdated {
		mapping.Content = append([]*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: installID},
		}, mapping.Content...)
	}
	encoded, err := yaml.Marshal(mapping)
	if err != nil {
		return nil, errors.New("registry Skill SKILL.md frontmatter could not be encoded")
	}
	return []byte("---\n" + strings.TrimSuffix(string(encoded), "\n") + "\n---\n" + normalized[bodyStart:]), nil
}

func installRegistrySkillArchive(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS string,
	archive registrySkillArchive,
) error {
	if client == nil {
		return errors.New("workspace is not reachable")
	}
	targetDir, err := skillset.ManagedSkillDirForName(archive.installID)
	if err != nil {
		return errors.New("registry Skill install_id is invalid")
	}
	suffix, err := randomRegistryInstallSuffix()
	if err != nil {
		return err
	}
	tempDir := path.Join(skillset.ManagedDir(), "registry-install-"+suffix)
	backupDir := path.Join(skillset.ManagedDir(), "registry-backup-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() {
		_ = client.DeleteFile(context.WithoutCancel(ctx), tempDir, true)
	}()
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return fmt.Errorf("create temporary Skill directory: %w", err)
	}

	executablePaths := make([]string, 0)
	for _, file := range archive.files {
		filePath := path.Join(tempDir, file.path)
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return fmt.Errorf("create Skill directory %q: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return fmt.Errorf("write Skill file %q: %w", file.path, err)
		}
		if file.executable {
			executablePaths = append(executablePaths, filePath)
		}
	}
	if err := applyRegistrySkillExecutableModes(ctx, client, workspaceOS, executablePaths); err != nil {
		return err
	}

	targetExists := false
	if _, err := client.Stat(ctx, targetDir); err == nil {
		targetExists = true
	} else if !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if targetExists {
		if err := client.Rename(ctx, targetDir, backupDir); err != nil {
			return fmt.Errorf("prepare existing Skill for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, targetDir); err != nil {
		if targetExists {
			if rollbackErr := client.Rename(context.WithoutCancel(ctx), backupDir, targetDir); rollbackErr != nil {
				return fmt.Errorf("publish installed Skill: %w; restore previous Skill from %q: %w", err, backupDir, rollbackErr)
			}
		}
		return fmt.Errorf("publish installed Skill: %w", err)
	}
	if targetExists {
		_ = client.DeleteFile(ctx, backupDir, true)
	}
	return nil
}

func randomRegistryInstallSuffix() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create installation ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func applyRegistrySkillExecutableModes(ctx context.Context, client *bridge.Client, workspaceOS string, paths []string) error {
	if len(paths) == 0 || strings.EqualFold(workspaceOS, "windows") || strings.EqualFold(workspaceOS, "win32") {
		return nil
	}
	for start := 0; start < len(paths); start += 100 {
		end := min(start+100, len(paths))
		quoted := make([]string, 0, end-start)
		for _, filePath := range paths[start:end] {
			quoted = append(quoted, shellQuoteRegistryPath(filePath))
		}
		result, err := client.ExecWithEnv(ctx, "chmod 755 -- "+strings.Join(quoted, " "), "/", 30, nil)
		if err != nil {
			return fmt.Errorf("preserve executable Skill files: %w", err)
		}
		if result != nil && result.ExitCode != 0 {
			return fmt.Errorf("preserve executable Skill files: chmod exited with code %d", result.ExitCode)
		}
	}
	return nil
}

func shellQuoteRegistryPath(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
