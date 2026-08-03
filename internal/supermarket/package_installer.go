package supermarket

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/memohai/memoh/internal/apperror"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxSkillArtifactCompressedBytes   = 6 * 1024 * 1024
	maxSkillArtifactUncompressedBytes = 5 * 1024 * 1024
	maxSkillArtifactArchiveBytes      = 5 * 1024 * 1024
	maxSkillArtifactFiles             = 1_000
	maxPackageSkills                  = 128
	maxPackageArtifactsCompressed     = 128 * 1024 * 1024
	maxPackageArtifactsUncompressed   = 128 * 1024 * 1024
	maxPackageArtifactsArchive        = 128 * 1024 * 1024
	maxPackageArtifactFiles           = 10_000
)

type InstallPackageRequest struct {
	RegistryID        string
	PackageID         string
	Revision          string
	WorkspaceTargetID string
}

type InstallPackageResponse struct {
	OK                bool                   `json:"ok" validate:"required"`
	RegistryID        string                 `json:"registry_id" validate:"required"`
	PackageID         string                 `json:"package_id" validate:"required"`
	Revision          string                 `json:"revision" validate:"required"`
	WorkspaceTargetID string                 `json:"workspace_target_id" validate:"required"`
	Skills            []InstallSkillResponse `json:"skills" validate:"required"`
} // @name handlers.InstallRegistryPackageResponse

type InstallSkillResponse struct {
	OK                bool   `json:"ok" validate:"required"`
	RegistryID        string `json:"registry_id" validate:"required"`
	PackageID         string `json:"package_id" validate:"required"`
	SkillID           string `json:"skill_id" validate:"required"`
	InstallID         string `json:"install_id" validate:"required"`
	WorkspaceTargetID string `json:"workspace_target_id" validate:"required"`
	ArtifactDigest    string `json:"artifact_digest" validate:"required"`
	FilesWritten      int    `json:"files_written" validate:"required"`
} // @name handlers.InstallRegistrySkillResponse

type preparedSkill struct {
	skill   CatalogSkill
	archive skillset.Archive
}

type preparedPackage struct {
	descriptor        SkillPackageDescriptor
	skills            []preparedSkill
	expectedArtifacts map[string]string
	workspaceOS       string
}

func (i *Installer) InstallPackage(ctx context.Context, botID string, req InstallPackageRequest) (InstallPackageResponse, error) {
	registryID := strings.TrimSpace(req.RegistryID)
	if !skillset.IsValidRegistryID(registryID) {
		return InstallPackageResponse{}, &StatusError{Status: http.StatusBadRequest, Message: "registry_id is invalid"}
	}
	packageID := strings.TrimSpace(req.PackageID)
	if !skillset.IsValidRegistryComponent(packageID) {
		return InstallPackageResponse{}, &StatusError{Status: http.StatusBadRequest, Message: "package_id is invalid"}
	}
	revision := strings.TrimSpace(req.Revision)
	if !isCanonicalSHA256(revision) {
		return InstallPackageResponse{}, &StatusError{Status: http.StatusBadRequest, Message: "revision is invalid"}
	}
	if i.workspaces == nil {
		return InstallPackageResponse{}, apperror.Wrap(apperror.CodeRegistryPackageInstallFailed, errors.New("workspace manager is not configured"), nil)
	}
	targetCtx := workspace.WithWorkspaceTarget(ctx, req.WorkspaceTargetID)
	target, err := i.workspaces.ResolveWorkspaceTarget(targetCtx, botID, req.WorkspaceTargetID)
	if err != nil {
		return InstallPackageResponse{}, &WorkspaceTargetError{Err: err}
	}
	release, err := i.acquirePreparation(targetCtx)
	if err != nil {
		return InstallPackageResponse{}, err
	}
	defer release()
	pkg, err := i.fetchPackageRelease(targetCtx, registryID, packageID, revision)
	if err != nil {
		return InstallPackageResponse{}, err
	}
	prepared, err := i.preparePackage(targetCtx, target.Info.OS, pkg, registryID, packageID, revision)
	if err != nil {
		return InstallPackageResponse{}, err
	}
	if err := i.checkArtifactConflicts(targetCtx, botID, "", target.TargetID, prepared.expectedArtifacts); err != nil {
		return InstallPackageResponse{}, err
	}
	var installed []InstallSkillResponse
	err = i.withBotMutation(targetCtx, botID, func(mutationCtx context.Context) error {
		if err := i.checkArtifactConflicts(mutationCtx, botID, "", target.TargetID, prepared.expectedArtifacts); err != nil {
			return err
		}
		if err := i.checkPackageMembers(mutationCtx, botID, target.TargetID, "", registryID, packageID, prepared.expectedArtifacts); err != nil {
			return err
		}
		publication, published, err := publishPackage(mutationCtx, target.Client, prepared, target.TargetID)
		if err != nil {
			return err
		}
		installed = published
		if err := publication.Commit(mutationCtx); err != nil && i.logger != nil {
			i.logger.Warn("cleanup replaced Skill Package failed", slog.Any("error", err))
		}
		return nil
	})
	if err != nil {
		return InstallPackageResponse{}, err
	}
	return InstallPackageResponse{OK: true, RegistryID: registryID, PackageID: packageID, Revision: revision, WorkspaceTargetID: target.TargetID, Skills: installed}, nil
}

func (i *Installer) fetchPackageRelease(ctx context.Context, registryID, packageID, revision string) (SkillPackageDescriptor, error) {
	pkg, err := i.client.FetchPackageRelease(ctx, registryID, packageID, revision)
	if err == nil {
		return pkg, nil
	}
	switch ErrorKindOf(err) {
	case ErrorNotFound:
		return SkillPackageDescriptor{}, apperror.New(apperror.CodeRegistryPackageNotFound, nil)
	case ErrorUnavailable:
		return SkillPackageDescriptor{}, apperror.Wrap(apperror.CodeRegistryUnavailable, fmt.Errorf("fetch Registry Package release: %w", err), nil)
	default:
		return SkillPackageDescriptor{}, invalidPackage(fmt.Errorf("invalid Registry Package release: %w", err))
	}
}

func (i *Installer) preparePackage(ctx context.Context, workspaceOS string, pkg SkillPackageDescriptor, registryID, packageID, revision string) (preparedPackage, error) {
	if pkg.Revision != revision {
		return preparedPackage{}, invalidPackage(errors.New("registry Package revision does not match the request"))
	}
	if err := validatePackage(pkg, registryID, packageID); err != nil {
		return preparedPackage{}, invalidPackage(err)
	}
	if err := validatePackageBudget(pkg.Skills); err != nil {
		return preparedPackage{}, invalidPackage(err)
	}
	prepared := preparedPackage{descriptor: pkg, skills: make([]preparedSkill, 0, len(pkg.Skills)), expectedArtifacts: make(map[string]string, len(pkg.Skills)), workspaceOS: workspaceOS}
	for _, skill := range pkg.Skills {
		prepared.expectedArtifacts[strings.Join([]string{registryID, packageID, skill.SkillID}, "/")] = skill.Artifact.Digest
		item, err := i.prepareSkill(ctx, skill)
		if err != nil {
			return preparedPackage{}, err
		}
		prepared.skills = append(prepared.skills, item)
	}
	return prepared, nil
}

func (i *Installer) prepareSkill(ctx context.Context, skill CatalogSkill) (preparedSkill, error) {
	artifact := skill.Artifact
	content, err := i.client.DownloadArtifact(ctx, ArtifactDownloadDescriptor{Digest: artifact.Digest, Size: artifact.Size, DownloadURL: artifact.DownloadURL})
	if err != nil {
		code := apperror.CodeRegistryPackageInvalid
		if ErrorKindOf(err) == ErrorUnavailable {
			code = apperror.CodeRegistryUnavailable
		}
		return preparedSkill{}, apperror.Wrap(code, fmt.Errorf("download Registry Skill Artifact: %w", err), nil)
	}
	archive, err := skillset.ReadArchiveWithLimits(content, artifact.UncompressedSize, artifact.ArchiveSize, artifact.FileCount)
	if err != nil {
		return preparedSkill{}, invalidPackage(err)
	}
	if archive.UncompressedSize() != artifact.UncompressedSize || archive.ArchiveSize() != artifact.ArchiveSize || archive.FileCount() != artifact.FileCount {
		return preparedSkill{}, invalidPackage(errors.New("registry Skill Artifact contents do not match its descriptor"))
	}
	return preparedSkill{skill: skill, archive: archive}, nil
}

func publishPackage(ctx context.Context, client *bridge.Client, prepared preparedPackage, workspaceTargetID string) (*skillset.PackagePublication, []InstallSkillResponse, error) {
	if client == nil {
		return nil, nil, apperror.Wrap(apperror.CodeRegistryPackageInstallFailed, errors.New("workspace is not reachable"), nil)
	}
	members := make([]skillset.PackageArchive, 0, len(prepared.skills))
	for _, item := range prepared.skills {
		members = append(members, skillset.PackageArchive{SkillID: item.skill.SkillID, Archive: item.archive})
	}
	publication, err := skillset.PublishPackage(ctx, client, prepared.workspaceOS, prepared.descriptor.RegistryID, prepared.descriptor.PackageID, members)
	if err != nil {
		return nil, nil, apperror.Wrap(apperror.CodeRegistryPackageInstallFailed, fmt.Errorf("publish Registry Package: %w", err), nil)
	}
	installed := make([]InstallSkillResponse, 0, len(prepared.skills))
	for _, item := range prepared.skills {
		installed = append(installed, InstallSkillResponse{OK: true, RegistryID: item.skill.RegistryID, PackageID: item.skill.PackageID, SkillID: item.skill.SkillID, InstallID: item.skill.InstallID, WorkspaceTargetID: workspaceTargetID, ArtifactDigest: item.skill.Artifact.Digest, FilesWritten: item.archive.FileCount()})
	}
	return publication, installed, nil
}

func (i *Installer) checkArtifactConflicts(ctx context.Context, botID, targetPluginID, targetID string, expected map[string]string) error {
	if i.plugins == nil {
		return nil
	}
	if err := i.plugins.CheckSkillArtifactConflicts(ctx, botID, targetPluginID, targetID, expected); err != nil {
		return withStatus(http.StatusConflict, err)
	}
	return nil
}

func (i *Installer) checkPackageMembers(ctx context.Context, botID, targetID, targetPluginID, registryID, packageID string, expected map[string]string) error {
	if i.plugins == nil {
		return nil
	}
	installations, err := i.plugins.List(ctx, botID)
	if err != nil {
		return err
	}
	for _, installation := range installations {
		if installation.Status == pluginspkg.StatusUninstalled || (targetPluginID != "" && installation.PluginID == targetPluginID) {
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
			if _, included := expected[strings.Join([]string{registryID, packageID, resourceSkill}, "/")]; !included {
				return &StatusError{Status: http.StatusConflict, Message: "An installed Plugin still uses a Skill removed from this Package release"}
			}
		}
	}
	return nil
}

func validatePackage(pkg SkillPackageDescriptor, registryID, packageID string) error {
	if pkg.SchemaVersion != "1" || pkg.RegistryID != registryID || pkg.PackageID != packageID || !isCanonicalSHA256(pkg.Revision) || len(pkg.Skills) == 0 || len(pkg.Skills) > maxPackageSkills || pkg.SkillCount != len(pkg.Skills) {
		return errors.New("registry Package release is invalid")
	}
	seen := make(map[string]struct{}, len(pkg.Skills))
	for _, skill := range pkg.Skills {
		if _, exists := seen[skill.SkillID]; exists {
			return errors.New("registry Package contains duplicate Skills")
		}
		seen[skill.SkillID] = struct{}{}
		if err := validateSkill(skill, registryID, packageID, skill.SkillID); err != nil {
			return err
		}
	}
	return nil
}

func validateSkill(skill CatalogSkill, registryID, packageID, skillID string) error {
	artifact := skill.Artifact
	if skill.RegistryID != registryID || skill.PackageID != packageID || skill.SkillID != skillID || skill.InstallID != strings.Join([]string{registryID, packageID, skillID}, "+") || !skillset.IsValidName(skill.InstallID) {
		return errors.New("registry Skill identity is invalid")
	}
	if artifact.Format != "memoh_skill_v1" || artifact.ContentType != "application/gzip" || !isCanonicalSHA256(artifact.Digest) || artifact.Size < 1 || artifact.Size > maxSkillArtifactCompressedBytes || artifact.UncompressedSize < 1 || artifact.UncompressedSize > maxSkillArtifactUncompressedBytes || artifact.ArchiveSize < 1 || artifact.ArchiveSize > maxSkillArtifactArchiveBytes || artifact.FileCount < 1 || artifact.FileCount > maxSkillArtifactFiles || strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("registry Skill Artifact descriptor is invalid")
	}
	return nil
}

func validatePackageBudget(skills []CatalogSkill) error {
	var compressed, uncompressed, archive int64
	files := 0
	for _, skill := range skills {
		artifact := skill.Artifact
		if artifact.Size > maxPackageArtifactsCompressed-compressed || artifact.UncompressedSize > maxPackageArtifactsUncompressed-uncompressed || artifact.ArchiveSize > maxPackageArtifactsArchive-archive || artifact.FileCount > maxPackageArtifactFiles-files {
			return errors.New("registry Package exceeds the aggregate Artifact limits")
		}
		compressed += artifact.Size
		uncompressed += artifact.UncompressedSize
		archive += artifact.ArchiveSize
		files += artifact.FileCount
	}
	return nil
}

func invalidPackage(err error) error {
	return apperror.Wrap(apperror.CodeRegistryPackageInvalid, err, nil)
}
