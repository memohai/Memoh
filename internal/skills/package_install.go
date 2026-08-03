package skills

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

// PackageArchive is one validated Skill member prepared for Package publication.
type PackageArchive struct {
	SkillID string
	Archive Archive
}

type packagePublicationClient interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

// PackagePublication retains the previous Package directory until the caller
// commits, so a larger Plugin installation can roll back all Package changes.
type PackagePublication struct {
	client       packagePublicationClient
	targetDir    string
	backupDir    string
	targetExists bool
	closed       bool
}

// PublishPackage stages every member before replacing the Package root.
func PublishPackage(
	ctx context.Context,
	client *bridge.Client,
	workspaceOS, registryID, packageID string,
	members []PackageArchive,
) (*PackagePublication, error) {
	if client == nil {
		return nil, errors.New("workspace is not reachable")
	}
	targetDir, err := SkillPackageDirForIDs(registryID, packageID)
	if err != nil || registryID == UserSkillNamespace || len(members) == 0 {
		return nil, errors.New("registry Package identity is invalid")
	}
	registryDir, err := skillNamespaceDirForID(registryID)
	if err != nil {
		return nil, errors.New("registry Package identity is invalid")
	}
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if _, err := SkillDirForIDs(registryID, packageID, member.SkillID); err != nil {
			return nil, errors.New("registry Package Skill identity is invalid")
		}
		if _, exists := seen[member.SkillID]; exists {
			return nil, errors.New("registry Package contains duplicate Skills")
		}
		seen[member.SkillID] = struct{}{}
	}
	suffix, err := randomInstallSuffix()
	if err != nil {
		return nil, err
	}
	stagingRoot := path.Join(ManagedDir(), ".staging")
	tempDir := path.Join(stagingRoot, "install-package-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-package-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	published := false
	defer func() {
		if published {
			return
		}
		cleanupCtx, cancel := publicationCleanupContext(ctx)
		defer cancel()
		_ = client.DeleteFile(cleanupCtx, tempDir, true)
	}()
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("create temporary Package directory: %w", err)
	}

	for _, member := range members {
		skillDir := path.Join(tempDir, member.SkillID)
		if err := client.Mkdir(ctx, skillDir); err != nil {
			return nil, fmt.Errorf("create temporary Package Skill directory: %w", err)
		}
		if err := writeArchiveFiles(ctx, client, workspaceOS, skillDir, member.Archive); err != nil {
			return nil, fmt.Errorf("stage Package Skill %q: %w", member.SkillID, err)
		}
	}

	if err := client.Mkdir(ctx, registryDir); err != nil {
		return nil, fmt.Errorf("create Registry Skill directory: %w", err)
	}
	targetExists := false
	if _, err := client.Stat(ctx, targetDir); err == nil {
		targetExists = true
	} else if !errors.Is(err, bridge.ErrNotFound) {
		return nil, fmt.Errorf("inspect existing Package: %w", err)
	}
	if targetExists {
		if err := client.Rename(ctx, targetDir, backupDir); err != nil {
			return nil, fmt.Errorf("prepare existing Package for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, targetDir); err != nil {
		if targetExists {
			rollbackCtx, cancel := publicationCleanupContext(ctx)
			defer cancel()
			if rollbackErr := client.Rename(rollbackCtx, backupDir, targetDir); rollbackErr != nil {
				return nil, fmt.Errorf(
					"publish Package: %w; restore previous Package from %q: %w",
					err, backupDir, rollbackErr,
				)
			}
		}
		return nil, fmt.Errorf("publish Package: %w", err)
	}
	published = true
	return &PackagePublication{
		client: client, targetDir: targetDir, backupDir: backupDir, targetExists: targetExists,
	}, nil
}

func (p *PackagePublication) Commit(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	cleanupCtx, cancel := publicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(cleanupCtx, p.backupDir, true); err != nil {
		return err
	}
	p.closed = true
	return nil
}

func (p *PackagePublication) Rollback(ctx context.Context) error {
	if p == nil || p.closed {
		return nil
	}
	rollbackCtx, cancel := publicationCleanupContext(ctx)
	defer cancel()
	if err := p.client.DeleteFile(rollbackCtx, p.targetDir, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove replacement Package: %w", err)
	}
	if !p.targetExists {
		p.closed = true
		return nil
	}
	if err := p.client.Rename(rollbackCtx, p.backupDir, p.targetDir); err != nil {
		return fmt.Errorf("restore previous Package from %q: %w", p.backupDir, err)
	}
	p.closed = true
	return nil
}
