package skills

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const archivePublicationCleanupTimeout = 30 * time.Second

// InstallArchive atomically replaces a registry Skill with a validated archive.
func InstallArchive(ctx context.Context, client *bridge.Client, workspaceOS, registryID, packageID, skillID string, archive Archive) error {
	publication, err := PublishArchive(ctx, client, workspaceOS, registryID, packageID, skillID, archive)
	if err != nil {
		return err
	}
	_ = publication.Commit(ctx)
	return nil
}

// ArchivePublication retains the previous Skill directory until the caller
// commits, allowing a larger Plugin installation to roll back atomically.
type archivePublicationClient interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

type ArchivePublication struct {
	client       archivePublicationClient
	targetDir    string
	backupDir    string
	targetExists bool
	closed       bool
}

func PublishArchive(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS, registryID, packageID, skillID string,
	archive Archive,
) (*ArchivePublication, error) {
	if client == nil {
		return nil, errors.New("workspace is not reachable")
	}
	if strings.EqualFold(strings.TrimSpace(registryID), UserSkillNamespace) {
		return nil, errors.New("registry Skill identity is invalid")
	}
	targetDir, err := SkillDirForIDs(registryID, packageID, skillID)
	if err != nil {
		return nil, errors.New("registry Skill identity is invalid")
	}
	packageDir, err := SkillPackageDirForIDs(registryID, packageID)
	if err != nil {
		return nil, errors.New("registry Skill identity is invalid")
	}
	suffix, err := randomInstallSuffix()
	if err != nil {
		return nil, err
	}
	stagingRoot := path.Join(ManagedDir(), ".staging")
	tempDir := path.Join(stagingRoot, "install-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() {
		cleanupCtx, cancel := archivePublicationCleanupContext(ctx)
		defer cancel()
		_ = client.DeleteFile(cleanupCtx, tempDir, true)
	}()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return nil, fmt.Errorf("create Skill staging directory: %w", err)
	}
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("create temporary Skill directory: %w", err)
	}

	executablePaths := make([]string, 0)
	for _, file := range archive.files {
		filePath := path.Join(tempDir, file.path)
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return nil, fmt.Errorf("create Skill directory %q: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return nil, fmt.Errorf("write Skill file %q: %w", file.path, err)
		}
		if file.executable {
			executablePaths = append(executablePaths, filePath)
		}
	}
	if err := applyExecutableModes(ctx, client, workspaceOS, executablePaths); err != nil {
		return nil, err
	}
	registryDir, err := skillNamespaceDirForID(registryID)
	if err != nil {
		return nil, errors.New("registry Skill identity is invalid")
	}
	if err := client.Mkdir(ctx, registryDir); err != nil {
		return nil, fmt.Errorf("create registry Skill directory: %w", err)
	}
	if err := client.Mkdir(ctx, packageDir); err != nil {
		return nil, fmt.Errorf("create registry package directory: %w", err)
	}

	targetExists := false
	if _, err := client.Stat(ctx, targetDir); err == nil {
		targetExists = true
	} else if !errors.Is(err, bridge.ErrNotFound) {
		return nil, fmt.Errorf("inspect existing Skill: %w", err)
	}
	if targetExists {
		if err := client.Rename(ctx, targetDir, backupDir); err != nil {
			return nil, fmt.Errorf("prepare existing Skill for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, targetDir); err != nil {
		if targetExists {
			rollbackCtx, cancel := archivePublicationCleanupContext(ctx)
			defer cancel()
			if rollbackErr := client.Rename(rollbackCtx, backupDir, targetDir); rollbackErr != nil {
				return nil, fmt.Errorf("publish installed Skill: %w; restore previous Skill from %q: %w", err, backupDir, rollbackErr)
			}
		}
		return nil, fmt.Errorf("publish installed Skill: %w", err)
	}
	return &ArchivePublication{
		client: client, targetDir: targetDir, backupDir: backupDir, targetExists: targetExists,
	}, nil
}

func (p *ArchivePublication) Commit(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	cleanupCtx, cancel := archivePublicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(cleanupCtx, p.backupDir, true); err != nil {
		return err
	}
	p.closed = true
	return nil
}

func (p *ArchivePublication) Rollback(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	rollbackCtx, cancel := archivePublicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(rollbackCtx, p.targetDir, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove replacement Skill: %w", err)
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	if err := p.client.Rename(rollbackCtx, p.backupDir, p.targetDir); err != nil {
		return fmt.Errorf("restore previous Skill from %q: %w", p.backupDir, err)
	}
	p.closed = true
	return nil
}

func archivePublicationCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), archivePublicationCleanupTimeout)
}

func randomInstallSuffix() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create installation ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func applyExecutableModes(ctx context.Context, client *bridge.Client, workspaceOS string, paths []string) error {
	if len(paths) == 0 || strings.EqualFold(workspaceOS, "windows") || strings.EqualFold(workspaceOS, "win32") {
		return nil
	}
	for start := 0; start < len(paths); start += 100 {
		end := min(start+100, len(paths))
		quoted := make([]string, 0, end-start)
		for _, filePath := range paths[start:end] {
			quoted = append(quoted, shellQuote(filePath))
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
