package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/skillpackages"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type packageRemovals struct {
	items []*skillset.PackageRemoval
}

func (s *Service) prepareObsoletePackageRemovals(ctx context.Context, botID string, req InstallRequest) (*packageRemovals, error) {
	if !req.ReplacePackages {
		return nil, nil
	}
	row, found, err := s.pluginRowByID(ctx, botID, req.Manifest.ID)
	if err != nil || !found {
		return nil, err
	}
	keep := make(map[string]struct{}, len(req.InstalledPackages))
	for _, pkg := range req.InstalledPackages {
		keep[packageTargetIdentity(skillpackages.NormalizeWorkspaceTargetID(req.WorkspaceTargetID), pkg.RegistryID, pkg.PackageID)] = struct{}{}
	}
	return s.preparePackageRemovals(ctx, botID, row, func(item skillpackages.Installation) bool {
		_, retained := keep[packageTargetIdentity(item.WorkspaceTargetID, item.RegistryID, item.PackageID)]
		return !retained
	})
}

func packageTargetIdentity(targetID, registryID, packageID string) string {
	return strings.TrimSpace(targetID) + "/" + PackageReferenceIdentity(PackageReference{RegistryID: registryID, PackageID: packageID})
}

func (s *Service) prepareUnownedPackageRemovals(ctx context.Context, botID string, row sqlc.BotPluginInstallation) (*packageRemovals, error) {
	return s.preparePackageRemovals(ctx, botID, row, func(skillpackages.Installation) bool { return true })
}

func (s *Service) preparePackageRemovals(
	ctx context.Context,
	botID string,
	row sqlc.BotPluginInstallation,
	include func(skillpackages.Installation) bool,
) (*packageRemovals, error) {
	if s.bridges == nil {
		return nil, nil
	}
	references, err := skillpackages.ListPluginReferences(ctx, s.queries, row.ID)
	if err != nil {
		return nil, err
	}
	result := &packageRemovals{}
	for _, item := range references {
		if item.DirectlyInstalled || item.PluginReferenceCount != 1 || !include(item) {
			continue
		}
		targetCtx := ctx
		if targetID := strings.TrimSpace(item.WorkspaceTargetID); targetID != "" {
			targetCtx = bridge.WithWorkspaceTarget(ctx, targetID)
		}
		client, err := s.bridges.MCPClient(targetCtx, botID)
		if err != nil {
			return nil, errors.Join(err, result.rollback(ctx))
		}
		removal, err := skillset.PreparePackageRemoval(targetCtx, client, item.RegistryID, item.PackageID)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("stage Plugin Package removal: %w", err), result.rollback(ctx))
		}
		if removal != nil {
			result.items = append(result.items, removal)
		}
	}
	return result, nil
}

func (s *Service) pluginRowByID(ctx context.Context, botID, pluginID string) (sqlc.BotPluginInstallation, bool, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, false, err
	}
	rows, err := s.queries.ListBotPluginInstallations(ctx, botUUID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, false, err
	}
	for _, row := range rows {
		if row.PluginID == pluginID && row.Status != StatusUninstalled {
			return row, true, nil
		}
	}
	return sqlc.BotPluginInstallation{}, false, nil
}

func (r *packageRemovals) commit(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	for _, item := range r.items {
		if err := item.Commit(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *packageRemovals) rollback(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	for index := len(r.items) - 1; index >= 0; index-- {
		if err := r.items[index].Rollback(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
