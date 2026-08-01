package plugins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const pluginBundleCleanupTimeout = 30 * time.Second

type pluginBundleRemovalClient interface {
	DeleteFile(ctx context.Context, path string, recursive bool) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

type pluginBundleRemoval struct {
	client     pluginBundleRemovalClient
	pluginRoot string
	backupDir  string
	closed     bool
}

func (s *Service) preparePluginBundleRemoval(
	ctx context.Context,
	botID string,
	row sqlc.BotPluginInstallation,
) (*pluginBundleRemoval, error) {
	if s.bridges == nil {
		return nil, nil
	}
	pluginRoot, err := skillset.PluginDirForID(row.PluginID)
	if err != nil {
		return nil, fmt.Errorf("resolve Plugin bundle path: %w", err)
	}
	metadata, err := decodeJSONMap(row.Metadata)
	if err != nil {
		return nil, fmt.Errorf("decode Plugin workspace metadata: %w", err)
	}
	workspaceTargetID, _ := metadata["workspace_target_id"].(string)
	targetContext := ctx
	if workspaceTargetID = strings.TrimSpace(workspaceTargetID); workspaceTargetID != "" {
		targetContext = bridge.WithWorkspaceTarget(ctx, workspaceTargetID)
	}
	client, err := s.bridges.MCPClient(targetContext, botID)
	if err != nil {
		return nil, fmt.Errorf("resolve Plugin workspace: %w", err)
	}
	suffixBytes := make([]byte, 12)
	if _, err := rand.Read(suffixBytes); err != nil {
		return nil, fmt.Errorf("create Plugin removal ID: %w", err)
	}
	stagingRoot := path.Join(skillset.PluginDirPath, ".staging")
	backupDir := path.Join(stagingRoot, "remove-"+row.PluginID+"-"+hex.EncodeToString(suffixBytes))
	_ = client.DeleteFile(targetContext, backupDir, true)
	if err := client.Mkdir(targetContext, stagingRoot); err != nil {
		return nil, fmt.Errorf("create Plugin removal staging root: %w", err)
	}
	if err := client.Rename(targetContext, pluginRoot, backupDir); err != nil {
		if errors.Is(err, bridge.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("stage Plugin bundle removal: %w", err)
	}
	return &pluginBundleRemoval{client: client, pluginRoot: pluginRoot, backupDir: backupDir}, nil
}

func (r *pluginBundleRemoval) commit(ctx context.Context) error {
	if r == nil || r.closed {
		return nil
	}
	cleanupCtx, cancel := pluginBundleCleanupContext(ctx)
	defer cancel()
	if err := r.client.DeleteFile(cleanupCtx, r.backupDir, true); err != nil {
		return err
	}
	r.closed = true
	return nil
}

func (r *pluginBundleRemoval) rollback(ctx context.Context) error {
	if r == nil || r.closed {
		return nil
	}
	rollbackContext, cancel := pluginBundleCleanupContext(ctx)
	defer cancel()
	if err := r.client.DeleteFile(rollbackContext, r.pluginRoot, true); err != nil && !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("remove conflicting Plugin bundle: %w", err)
	}
	if err := r.client.Rename(rollbackContext, r.backupDir, r.pluginRoot); err != nil {
		return fmt.Errorf("restore Plugin bundle: %w", err)
	}
	r.closed = true
	return nil
}

func pluginBundleCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), pluginBundleCleanupTimeout)
}
