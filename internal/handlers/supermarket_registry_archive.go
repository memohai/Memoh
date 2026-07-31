package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	maxRegistrySkillArtifactUncompressedBytes = 100 * 1024 * 1024
	maxRegistrySkillArtifactFiles             = 10_000
	maxRegistrySkillArtifactEntries           = 20_000
)

type registrySkillArchiveFile struct {
	path       string
	content    []byte
	executable bool
}

type registrySkillArchive struct {
	files []registrySkillArchiveFile
}

func readRegistrySkillArchive(content []byte) (registrySkillArchive, error) {
	gz, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return registrySkillArchive{}, errors.New("registry Skill Artifact is not valid gzip")
	}
	defer func() { _ = gz.Close() }()

	archive := registrySkillArchive{}
	seen := make(map[string]bool)
	var totalSize int64
	totalEntries := 0
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
		totalEntries++
		if totalEntries > maxRegistrySkillArtifactEntries {
			return registrySkillArchive{}, errors.New("registry Skill Artifact contains too many entries")
		}
		if header.Name == "" || path.IsAbs(header.Name) || strings.Contains(header.Name, `\`) {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains unsafe path %q", header.Name)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.Clean(name) != name {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains non-canonical path %q", header.Name)
		}
		parts := strings.Split(name, "/")
		if containsUnsafePathPart(parts) {
			return registrySkillArchive{}, fmt.Errorf("registry Skill Artifact contains unsafe path %q", header.Name)
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
		if name == "SKILL.md" {
			if err := validateRegistrySkillManifest(fileContent); err != nil {
				return registrySkillArchive{}, err
			}
			hasManifest = true
		}
		archive.files = append(archive.files, registrySkillArchiveFile{
			path: name, content: fileContent, executable: header.FileInfo().Mode().Perm()&0o111 != 0,
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

func validateRegistrySkillManifest(content []byte) error {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return errors.New("registry Skill SKILL.md is missing YAML frontmatter")
	}
	rest := normalized[4:]
	closing := strings.Index(rest, "\n---\n")
	if closing < 0 && strings.HasSuffix(rest, "\n---") {
		closing = len(rest) - len("\n---")
	}
	if closing < 0 {
		return errors.New("registry Skill SKILL.md has malformed YAML frontmatter")
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:closing]), &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("registry Skill SKILL.md has invalid YAML frontmatter")
	}
	return nil
}

func installRegistrySkillArchive(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS string,
	registryID, packageID, skillID string,
	archive registrySkillArchive,
) error {
	if client == nil {
		return errors.New("workspace is not reachable")
	}
	targetDir, err := skillset.RegistrySkillDirForIDs(registryID, packageID, skillID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	packageDir, err := skillset.RegistryPackageSkillsDirForIDs(registryID, packageID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	suffix, err := randomRegistryInstallSuffix()
	if err != nil {
		return err
	}
	stagingRoot := path.Join(skillset.ManagedDir(), ".staging")
	tempDir := path.Join(stagingRoot, "install-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() {
		_ = client.DeleteFile(context.WithoutCancel(ctx), tempDir, true)
	}()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return fmt.Errorf("create registry Skill staging directory: %w", err)
	}
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

	registryDir, err := skillset.RegistryDirForID(registryID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	if err := skillset.GuardRegistryInstall(ctx, client, registryID); err != nil {
		return err
	}
	if err := client.Mkdir(ctx, registryDir); err != nil {
		return fmt.Errorf("create registry Skill registry directory: %w", err)
	}
	if err := client.Mkdir(ctx, packageDir); err != nil {
		return fmt.Errorf("create registry Skill package directory: %w", err)
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
