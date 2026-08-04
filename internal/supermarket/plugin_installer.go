package supermarket

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	pluginspkg "github.com/memohai/memoh/internal/plugins"
	"github.com/memohai/memoh/internal/skillpackages"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxPluginReleasePackages   = 128
	pluginScriptTimeoutSeconds = 10 * 60
	pluginScriptOutputLimit    = 64 * 1024
)

type InstallPluginRequest struct {
	PluginID                  string
	ReleaseRevision           string
	ExpectedInstalledRevision *string
	ExpectedInstallationTime  *time.Time
	Variables                 map[string]string
}

type pluginSkillsResult struct {
	OK     bool                `json:"ok"`
	Skills []pluginSkillResult `json:"skills,omitempty"`
	Error  string              `json:"error,omitempty"`
}

type pluginSkillResult struct {
	RegistryID     string `json:"registry_id"`
	PackageID      string `json:"package_id"`
	SkillID        string `json:"skill_id"`
	InstallID      string `json:"install_id,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
	FilesWritten   int    `json:"files_written,omitempty"`
}

type pluginScriptsResult struct {
	OK          bool                  `json:"ok"`
	CommandsRun int                   `json:"commands_run"`
	Results     []pluginCommandResult `json:"results,omitempty"`
	Error       string                `json:"error,omitempty"`
}

type pluginCommandResult struct {
	Command  string `json:"command"`
	ExitCode int32  `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

type pluginScriptExecutor interface {
	ExecWithEnv(context.Context, string, string, int32, []string) (*bridge.ExecResult, error)
}

func (i *Installer) InstallPlugin(ctx context.Context, botID string, req InstallPluginRequest) (pluginspkg.Installation, error) {
	if i.plugins == nil {
		return pluginspkg.Installation{}, errors.New("plugin service is not configured")
	}
	req.PluginID = strings.TrimSpace(req.PluginID)
	if !skillset.IsValidPluginID(req.PluginID) {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusBadRequest, Message: "plugin_id is invalid"}
	}
	req.ReleaseRevision = strings.TrimSpace(req.ReleaseRevision)
	if !isCanonicalSHA256(req.ReleaseRevision) {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusBadRequest, Message: "release_revision is invalid"}
	}
	if (req.ExpectedInstalledRevision == nil) != (req.ExpectedInstallationTime == nil) {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusBadRequest, Message: "expected Plugin revision and installation timestamp must both be null or both be set"}
	}
	if req.ExpectedInstalledRevision != nil {
		revision := strings.TrimSpace(*req.ExpectedInstalledRevision)
		if !isCanonicalSHA256(revision) {
			return pluginspkg.Installation{}, &StatusError{Status: http.StatusBadRequest, Message: "expected_installed_revision is invalid"}
		}
		req.ExpectedInstalledRevision = &revision
	}
	entry, err := i.client.FetchPluginEntry(ctx, req.PluginID)
	if err != nil {
		if i.logger != nil {
			i.logger.Error("supermarket Plugin fetch failed", slog.String("plugin_id", req.PluginID), slog.Any("error", err))
		}
		if ErrorKindOf(err) == ErrorNotFound {
			return pluginspkg.Installation{}, &StatusError{Status: http.StatusNotFound, Message: fmt.Sprintf("plugin %q not found in supermarket", req.PluginID)}
		}
		return pluginspkg.Installation{}, withStatus(http.StatusBadGateway, err)
	}
	if entry.Release.Revision != req.ReleaseRevision {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusConflict, Message: "Plugin release changed; refresh before installing"}
	}
	manifest := pluginspkg.NormalizeManifest(entry.Manifest)
	if err := pluginspkg.ValidatePackageReferences(manifest.Packages); err != nil {
		return pluginspkg.Installation{}, withStatus(http.StatusBadRequest, err)
	}
	if err := validatePluginEntry(entry, req.PluginID, manifest); err != nil {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusBadGateway, Message: fmt.Sprintf("invalid Plugin release from supermarket: %v", err)}
	}
	target, err := i.resolvePluginTarget(ctx, botID)
	if err != nil {
		return pluginspkg.Installation{}, err
	}
	if len(entry.Release.Packages) > 0 {
		release, err := i.acquirePreparation(ctx)
		if err != nil {
			return pluginspkg.Installation{}, err
		}
		defer release()
	}
	packageDescriptors, err := i.resolvePluginPackages(ctx, entry.Release.Packages)
	if err != nil {
		return pluginspkg.Installation{}, err
	}
	bundle, err := i.preparePluginBundle(ctx, req.PluginID, manifest.ID, entry.Release.Artifact, manifest.Packages)
	if err != nil {
		return pluginspkg.Installation{}, withStatus(http.StatusBadGateway, err)
	}
	packages, err := i.preparePluginPackages(ctx, target.Info.OS, entry.Release.Packages, packageDescriptors)
	if err != nil {
		return pluginspkg.Installation{}, err
	}
	lockKeys := make([]string, 0, len(entry.Release.Packages)+1)
	lockKeys = append(lockKeys, pluginInstallationLockKey(botID, target.TargetID, manifest.ID))
	for _, pkg := range entry.Release.Packages {
		lockKeys = append(lockKeys, packageInstallationLockKey(
			botID, target.TargetID, pkg.RegistryID, pkg.PackageID,
		))
	}
	releaseInstall, err := acquireInstallationResources(ctx, lockKeys...)
	if err != nil {
		return pluginspkg.Installation{}, err
	}
	defer releaseInstall()

	var (
		installation  pluginspkg.Installation
		bundleResult  pluginspkg.BundleInstallResult
		scriptsResult pluginScriptsResult
		skillsResult  = pluginSkillsResult{OK: true}
	)
	state, installed, err := i.plugins.InstalledPluginState(ctx, botID, manifest.ID)
	if err != nil {
		return pluginspkg.Installation{}, withStatus(http.StatusInternalServerError, err)
	}
	if !matchesExpectedInstallation(req.ExpectedInstalledRevision, req.ExpectedInstallationTime, state, installed) {
		return pluginspkg.Installation{}, &StatusError{Status: http.StatusConflict, Message: "installed Plugin changed; refresh before installing"}
	}
	installedSkills, installedPackages, publications, err := publishPluginPackages(ctx, target.Client, target.TargetID, packages, &skillsResult)
	if err != nil {
		return pluginspkg.Installation{}, err
	}
	bundlePublication, err := pluginspkg.PublishBundleArchive(ctx, target.Client, target.Info.OS, manifest.ID, bundle)
	if err != nil {
		return pluginspkg.Installation{}, rollbackPluginPublications(ctx, withStatus(http.StatusBadGateway, err), nil, publications)
	}
	bundleResult = bundlePublication.Result()
	scriptsResult, err = runPluginScripts(ctx, target.Client, botID, manifest.ID, manifest.Install)
	if err != nil {
		return pluginspkg.Installation{}, rollbackPluginPublications(ctx, withStatus(http.StatusBadGateway, err), bundlePublication, publications)
	}
	installation, err = i.plugins.Install(ctx, botID, pluginspkg.InstallRequest{
		Manifest: manifest, Variables: req.Variables, InstalledSkills: installedSkills, InstalledPackages: installedPackages, ReplacePackages: true,
		Release:           pluginspkg.ReleaseMetadata{Revision: entry.Release.Revision, ArtifactDigest: entry.Release.Artifact.Digest},
		WorkspaceTargetID: target.TargetID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, skillpackages.ErrRevisionConflict) {
			status = http.StatusConflict
		}
		return pluginspkg.Installation{}, rollbackPluginPublications(ctx, withStatus(status, err), bundlePublication, publications)
	}
	if err := bundlePublication.Commit(ctx); err != nil && i.logger != nil {
		i.logger.Warn("cleanup Plugin bundle backup failed", slog.Any("error", err))
	}
	for _, publication := range publications {
		if err := publication.Commit(ctx); err != nil && i.logger != nil {
			i.logger.Warn("cleanup Plugin Package backup failed", slog.Any("error", err))
		}
	}
	installation = withInstallMetadata(installation, "hooks_install", bundleResult.Hooks)
	installation = withInstallMetadata(installation, "scripts_install", bundleResult.Scripts)
	installation = withInstallMetadata(installation, "install_scripts", scriptsResult)
	installation = withInstallMetadata(installation, "skills_install", skillsResult)
	return installation, nil
}

func validatePluginEntry(entry PluginEntry, pluginID string, manifest pluginspkg.Manifest) error {
	if manifest.ID != pluginID || !isCanonicalSHA256(entry.Release.Revision) {
		return errors.New("plugin identity or release revision is invalid")
	}
	if _, err := time.Parse(time.RFC3339, entry.Release.PublishedAt); err != nil {
		return errors.New("plugin release publication time is invalid")
	}
	artifact := entry.Release.Artifact
	if artifact.Format != "memoh_plugin_v1" || artifact.ContentType != "application/gzip" || !isCanonicalSHA256(artifact.Digest) || artifact.Size < 1 || artifact.Size > pluginspkg.MaxBundleCompressedBytes || strings.TrimSpace(artifact.DownloadURL) == "" {
		return errors.New("plugin Artifact descriptor is invalid")
	}
	if len(entry.Release.Packages) != len(manifest.Packages) || len(entry.Release.Packages) > maxPluginReleasePackages {
		return errors.New("plugin release Package locks are invalid")
	}
	resolved := make([]pluginspkg.PackageReference, 0, len(entry.Release.Packages))
	for _, item := range entry.Release.Packages {
		if !isCanonicalSHA256(item.Revision) {
			return errors.New("plugin Package revision is invalid")
		}
		resolved = append(resolved, pluginspkg.PackageReference{RegistryID: item.RegistryID, PackageID: item.PackageID})
	}
	if !pluginspkg.SamePackageReferences(resolved, manifest.Packages) {
		return errors.New("plugin release Package locks do not match plugin.yaml")
	}
	return nil
}

func (i *Installer) resolvePluginTarget(ctx context.Context, botID string) (workspace.ResolvedWorkspaceTarget, error) {
	if i.workspaces != nil {
		target, err := i.workspaces.ResolveWorkspaceTarget(ctx, botID, "")
		if err != nil {
			return workspace.ResolvedWorkspaceTarget{}, &WorkspaceTargetError{Err: err}
		}
		return target, nil
	}
	if i.containers == nil {
		return workspace.ResolvedWorkspaceTarget{}, errors.New("workspace runtime provider is not configured")
	}
	client, err := i.containers.MCPClient(ctx, botID)
	if err != nil {
		return workspace.ResolvedWorkspaceTarget{}, fmt.Errorf("workspace is not reachable: %w", err)
	}
	return workspace.ResolvedWorkspaceTarget{Client: client}, nil
}

func (i *Installer) resolvePluginPackages(ctx context.Context, locks []PluginResolvedPackage) ([]SkillPackageDescriptor, error) {
	if len(locks) > maxPluginReleasePackages {
		return nil, errors.New("plugin release exceeds the Package limit")
	}
	descriptors := make([]SkillPackageDescriptor, 0, len(locks))
	var compressed, uncompressed, archive int64
	files := 0
	for _, lock := range locks {
		pkg, err := i.fetchPackageRelease(ctx, lock.RegistryID, lock.PackageID, lock.Revision)
		if err != nil {
			return nil, err
		}
		if err := validatePackage(pkg, lock.RegistryID, lock.PackageID); err != nil || pkg.Revision != lock.Revision {
			if err == nil {
				err = errors.New("plugin Package revision does not match its lock")
			}
			return nil, invalidPackage(err)
		}
		for _, skill := range pkg.Skills {
			a := skill.Artifact
			if a.Size > maxPackageArtifactsCompressed-compressed {
				return nil, errors.New("plugin release Skills exceed the compressed limit")
			}
			if a.UncompressedSize > maxPackageArtifactsUncompressed-uncompressed {
				return nil, errors.New("plugin release Skills exceed the uncompressed limit")
			}
			if a.ArchiveSize > maxPackageArtifactsArchive-archive {
				return nil, errors.New("plugin release Skills exceed the decompressed archive limit")
			}
			if a.FileCount > maxPackageArtifactFiles-files {
				return nil, errors.New("plugin release Skills exceed the file limit")
			}
			compressed += a.Size
			uncompressed += a.UncompressedSize
			archive += a.ArchiveSize
			files += a.FileCount
		}
		descriptors = append(descriptors, pkg)
	}
	return descriptors, nil
}

func (i *Installer) preparePluginPackages(ctx context.Context, workspaceOS string, locks []PluginResolvedPackage, descriptors []SkillPackageDescriptor) ([]preparedPackage, error) {
	if len(locks) != len(descriptors) {
		return nil, errors.New("plugin Package descriptor count does not match its locks")
	}
	packages := make([]preparedPackage, 0, len(locks))
	for index, lock := range locks {
		prepared, err := i.preparePackage(ctx, workspaceOS, descriptors[index], lock.RegistryID, lock.PackageID, lock.Revision)
		if err != nil {
			return nil, err
		}
		packages = append(packages, prepared)
	}
	return packages, nil
}

func (i *Installer) preparePluginBundle(ctx context.Context, sourceID, targetID string, artifact PluginArtifact, packages []pluginspkg.PackageReference) (pluginspkg.BundleArchive, error) {
	content, err := i.client.DownloadArtifact(ctx, ArtifactDownloadDescriptor{Digest: artifact.Digest, Size: artifact.Size, DownloadURL: artifact.DownloadURL})
	if err != nil {
		return pluginspkg.BundleArchive{}, fmt.Errorf("download Plugin Artifact: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return pluginspkg.BundleArchive{}, fmt.Errorf("invalid gzip response from supermarket: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return pluginspkg.ReadBundleArchive(sourceID, targetID, gz, packages)
}

func publishPluginPackages(ctx context.Context, client *bridge.Client, targetID string, packages []preparedPackage, result *pluginSkillsResult) ([]pluginspkg.InstalledSkill, []pluginspkg.InstalledPackage, []*skillset.PackagePublication, error) {
	installedSkills := make([]pluginspkg.InstalledSkill, 0)
	installedPackages := make([]pluginspkg.InstalledPackage, 0, len(packages))
	publications := make([]*skillset.PackagePublication, 0, len(packages))
	for _, pkg := range packages {
		publication, installed, err := publishPackage(ctx, client, pkg, targetID)
		if err != nil {
			result.OK = false
			result.Error = err.Error()
			return nil, nil, publications, errors.Join(err, rollbackPackages(ctx, publications))
		}
		publications = append(publications, publication)
		installedPackages = append(installedPackages, pluginspkg.InstalledPackage{
			RegistryID: pkg.descriptor.RegistryID, PackageID: pkg.descriptor.PackageID, Revision: pkg.descriptor.Revision,
		})
		for _, skill := range installed {
			member := pluginspkg.InstalledSkill{RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID}
			installedSkills = append(installedSkills, member)
			result.Skills = append(result.Skills, pluginSkillResult{RegistryID: skill.RegistryID, PackageID: skill.PackageID, SkillID: skill.SkillID, InstallID: skill.InstallID, ArtifactDigest: skill.ArtifactDigest, FilesWritten: skill.FilesWritten})
		}
	}
	return installedSkills, installedPackages, publications, nil
}

func rollbackPackages(ctx context.Context, publications []*skillset.PackagePublication) error {
	var errs []error
	for index := len(publications) - 1; index >= 0; index-- {
		if err := publications[index].Rollback(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func rollbackPluginPublications(ctx context.Context, cause error, bundle *pluginspkg.BundlePublication, packages []*skillset.PackagePublication) error {
	errs := []error{cause}
	if bundle != nil {
		if err := bundle.Rollback(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := rollbackPackages(ctx, packages); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func runPluginScripts(ctx context.Context, client pluginScriptExecutor, botID, pluginID string, commands pluginspkg.InstallCommands) (pluginScriptsResult, error) {
	result := pluginScriptsResult{OK: true}
	if len(commands) == 0 {
		return result, nil
	}
	if client == nil {
		return result, errors.New("workspace is not reachable")
	}
	root, err := skillset.PluginDirForID(pluginID)
	if err != nil {
		return result, err
	}
	env := []string{"MEMOH_PLUGIN_ID=" + pluginID, "MEMOH_PLUGIN_DIR=" + root, "MEMOH_BOT_ID=" + botID}
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		item := pluginCommandResult{Command: command}
		execResult, execErr := client.ExecWithEnv(ctx, command, root, pluginScriptTimeoutSeconds, env)
		result.CommandsRun++
		if execResult != nil {
			item.ExitCode = execResult.ExitCode
			item.Stdout = truncateOutput(execResult.Stdout)
			item.Stderr = truncateOutput(execResult.Stderr)
		}
		if execErr != nil {
			item.Error = execErr.Error()
		} else if execResult != nil && execResult.ExitCode != 0 {
			item.Error = fmt.Sprintf("command exited with code %d", execResult.ExitCode)
		}
		result.Results = append(result.Results, item)
		if item.Error != "" {
			result.OK = false
			result.Error = item.Error
			return result, errors.New(item.Error)
		}
	}
	return result, nil
}

func matchesExpectedInstallation(revision *string, updatedAt *time.Time, actual pluginspkg.InstalledPluginState, installed bool) bool {
	if revision == nil || updatedAt == nil {
		return !installed
	}
	return installed && actual.ReleaseRevision == *revision && actual.UpdatedAt.Equal(*updatedAt)
}

func withInstallMetadata(installation pluginspkg.Installation, key string, value any) pluginspkg.Installation {
	metadata := maps.Clone(installation.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[key] = value
	installation.Metadata = metadata
	return installation
}

func truncateOutput(value string) string {
	if len(value) <= pluginScriptOutputLimit {
		return value
	}
	return value[:pluginScriptOutputLimit]
}
