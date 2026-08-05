package skillpackages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/skills"
)

var (
	ErrNotInstalled     = errors.New("skill Package is not installed")
	ErrNotDirect        = errors.New("skill Package is not directly installed")
	ErrRevisionConflict = errors.New("skill Package revision conflicts with an existing owner")
)

type Installation struct {
	ID                   string    `json:"id" validate:"required"`
	BotID                string    `json:"bot_id" validate:"required"`
	WorkspaceTargetID    string    `json:"workspace_target_id" validate:"required"`
	RegistryID           string    `json:"registry_id" validate:"required"`
	PackageID            string    `json:"package_id" validate:"required"`
	Revision             string    `json:"revision" validate:"required"`
	DirectlyInstalled    bool      `json:"directly_installed" validate:"required"`
	PluginReferenceCount int64     `json:"plugin_reference_count" validate:"required"`
	InstalledAt          time.Time `json:"installed_at" validate:"required"`
	UpdatedAt            time.Time `json:"updated_at" validate:"required"`
}

type Requirement struct {
	RegistryID string
	PackageID  string
	Revision   string
}

func NormalizeWorkspaceTargetID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "native"
	}
	return value
}

type Store interface {
	CountBotSkillPackageReferences(context.Context, pgtype.UUID) (int64, error)
	DeleteBotPluginPackageReferences(context.Context, pgtype.UUID) error
	DeleteBotSkillPackageInstallationIfUnreferenced(context.Context, dbsqlc.DeleteBotSkillPackageInstallationIfUnreferencedParams) (pgtype.UUID, error)
	GetBotSkillPackageInstallation(context.Context, dbsqlc.GetBotSkillPackageInstallationParams) (dbsqlc.BotSkillPackageInstallation, error)
	GetBotSkillPackageInstallationByID(context.Context, dbsqlc.GetBotSkillPackageInstallationByIDParams) (dbsqlc.BotSkillPackageInstallation, error)
	ListBotPluginPackageReferences(context.Context, pgtype.UUID) ([]dbsqlc.ListBotPluginPackageReferencesRow, error)
	ListBotSkillPackageInstallations(context.Context, pgtype.UUID) ([]dbsqlc.ListBotSkillPackageInstallationsRow, error)
	SetBotSkillPackageDirectlyInstalled(context.Context, dbsqlc.SetBotSkillPackageDirectlyInstalledParams) (dbsqlc.BotSkillPackageInstallation, error)
	UpsertBotPluginPackageReference(context.Context, dbsqlc.UpsertBotPluginPackageReferenceParams) (dbsqlc.BotPluginPackageReference, error)
	UpsertDirectBotSkillPackageInstallation(context.Context, dbsqlc.UpsertDirectBotSkillPackageInstallationParams) (dbsqlc.BotSkillPackageInstallation, error)
	UpsertPluginBotSkillPackageInstallation(context.Context, dbsqlc.UpsertPluginBotSkillPackageInstallationParams) (dbsqlc.BotSkillPackageInstallation, error)
}

type transactionalQueries interface {
	InTx(context.Context, func(dbstore.Queries) error) error
}

type Service struct {
	queries Store
}

func NewService(queries Store) *Service {
	return &Service{queries: queries}
}

func (s *Service) List(ctx context.Context, botID string) ([]Installation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotSkillPackageInstallations(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	result := make([]Installation, 0, len(rows))
	for _, row := range rows {
		result = append(result, Installation{
			ID: row.ID.String(), BotID: row.BotID.String(), WorkspaceTargetID: row.WorkspaceTargetID,
			RegistryID: row.RegistryID, PackageID: row.PackageID, Revision: row.Revision,
			DirectlyInstalled: row.DirectlyInstalled, PluginReferenceCount: row.PluginReferenceCount,
			InstalledAt: row.InstalledAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return result, nil
}

func (s *Service) ListForTarget(ctx context.Context, botID, workspaceTargetID string) ([]Installation, error) {
	items, err := s.List(ctx, botID)
	if err != nil {
		return nil, err
	}
	targetID := strings.TrimSpace(workspaceTargetID)
	result := make([]Installation, 0, len(items))
	for _, item := range items {
		if item.WorkspaceTargetID == targetID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, botID, workspaceTargetID, registryID, packageID string) (Installation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Installation{}, err
	}
	row, err := s.queries.GetBotSkillPackageInstallation(ctx, dbsqlc.GetBotSkillPackageInstallationParams{
		BotID: botUUID, WorkspaceTargetID: strings.TrimSpace(workspaceTargetID),
		RegistryID: strings.TrimSpace(registryID), PackageID: strings.TrimSpace(packageID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Installation{}, ErrNotInstalled
	}
	if err != nil {
		return Installation{}, err
	}
	return installationWithReferences(ctx, s.queries, row)
}

func (s *Service) GetByID(ctx context.Context, botID, installationID string) (Installation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Installation{}, err
	}
	id, err := db.ParseUUID(installationID)
	if err != nil {
		return Installation{}, err
	}
	row, err := s.queries.GetBotSkillPackageInstallationByID(ctx, dbsqlc.GetBotSkillPackageInstallationByIDParams{
		BotID: botUUID,
		ID:    id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Installation{}, ErrNotInstalled
	}
	if err != nil {
		return Installation{}, err
	}
	return installationWithReferences(ctx, s.queries, row)
}

func (s *Service) RecordDirect(ctx context.Context, botID, workspaceTargetID string, requirement Requirement) (Installation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Installation{}, err
	}
	if err := validateRequirement(requirement); err != nil {
		return Installation{}, err
	}
	row, err := s.queries.UpsertDirectBotSkillPackageInstallation(ctx, dbsqlc.UpsertDirectBotSkillPackageInstallationParams{
		BotID: botUUID, WorkspaceTargetID: strings.TrimSpace(workspaceTargetID),
		RegistryID: requirement.RegistryID, PackageID: requirement.PackageID, Revision: requirement.Revision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Installation{}, ErrRevisionConflict
	}
	if err != nil {
		return Installation{}, err
	}
	return installationWithReferences(ctx, s.queries, row)
}

// ReleaseDirect removes the direct owner. The Package row is deleted only when
// no Plugin still references it. The bool reports whether files may be removed.
func (s *Service) ReleaseDirect(ctx context.Context, botID, installationID string) (Installation, bool, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Installation{}, false, err
	}
	id, err := db.ParseUUID(installationID)
	if err != nil {
		return Installation{}, false, err
	}
	var (
		result  Installation
		removed bool
	)
	err = s.inTx(ctx, func(q Store) error {
		row, getErr := q.GetBotSkillPackageInstallationByID(ctx, dbsqlc.GetBotSkillPackageInstallationByIDParams{
			BotID: botUUID,
			ID:    id,
		})
		if errors.Is(getErr, pgx.ErrNoRows) {
			return ErrNotInstalled
		}
		if getErr != nil {
			return getErr
		}
		if !row.DirectlyInstalled {
			return ErrNotDirect
		}
		row, updateErr := q.SetBotSkillPackageDirectlyInstalled(ctx, dbsqlc.SetBotSkillPackageDirectlyInstalledParams{
			BotID: botUUID, ID: id, DirectlyInstalled: false,
		})
		if updateErr != nil {
			return updateErr
		}
		result, updateErr = installationWithReferences(ctx, q, row)
		if updateErr != nil {
			return updateErr
		}
		if _, deleteErr := q.DeleteBotSkillPackageInstallationIfUnreferenced(ctx, dbsqlc.DeleteBotSkillPackageInstallationIfUnreferencedParams{BotID: botUUID, ID: id}); deleteErr == nil {
			removed = true
		} else if !errors.Is(deleteErr, pgx.ErrNoRows) {
			return deleteErr
		}
		return nil
	})
	return result, removed, err
}

func (s *Service) inTx(ctx context.Context, fn func(Store) error) error {
	if tx, ok := s.queries.(transactionalQueries); ok {
		return tx.InTx(ctx, func(q dbstore.Queries) error { return fn(q) })
	}
	return fn(s.queries)
}

// ReplacePluginReferences runs inside the Plugin installation transaction.
// It makes Package revision ownership authoritative and returns Package rows
// that became unowned after replacing the Plugin's previous references.
func ReplacePluginReferences(ctx context.Context, q Store, botID, pluginInstallationID pgtype.UUID, workspaceTargetID string, requirements []Requirement) ([]Installation, error) {
	seen := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		if err := validateRequirement(requirement); err != nil {
			return nil, err
		}
		key := requirement.RegistryID + "/" + requirement.PackageID
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate Skill Package requirement %q", key)
		}
		seen[key] = struct{}{}
	}
	previous, err := q.ListBotPluginPackageReferences(ctx, pluginInstallationID)
	if err != nil {
		return nil, err
	}
	if err := q.DeleteBotPluginPackageReferences(ctx, pluginInstallationID); err != nil {
		return nil, err
	}
	for _, requirement := range requirements {
		row, err := q.UpsertPluginBotSkillPackageInstallation(ctx, dbsqlc.UpsertPluginBotSkillPackageInstallationParams{
			BotID: botID, WorkspaceTargetID: strings.TrimSpace(workspaceTargetID),
			RegistryID: requirement.RegistryID, PackageID: requirement.PackageID, Revision: requirement.Revision,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s/%s", ErrRevisionConflict, requirement.RegistryID, requirement.PackageID)
		}
		if err != nil {
			return nil, err
		}
		if _, err := q.UpsertBotPluginPackageReference(ctx, dbsqlc.UpsertBotPluginPackageReferenceParams{
			BotID: botID, WorkspaceTargetID: NormalizeWorkspaceTargetID(workspaceTargetID),
			PluginInstallationID: pluginInstallationID, PackageInstallationID: row.ID,
			RequiredRevision: requirement.Revision,
		}); err != nil {
			return nil, err
		}
	}

	orphaned := make([]Installation, 0)
	for _, old := range previous {
		if _, err := q.DeleteBotSkillPackageInstallationIfUnreferenced(ctx, dbsqlc.DeleteBotSkillPackageInstallationIfUnreferencedParams{
			BotID: botID, ID: old.PackageInstallationID,
		}); err == nil {
			orphaned = append(orphaned, installationFromReference(old))
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return orphaned, nil
}

func ListPluginReferences(ctx context.Context, q Store, pluginInstallationID pgtype.UUID) ([]Installation, error) {
	rows, err := q.ListBotPluginPackageReferences(ctx, pluginInstallationID)
	if err != nil {
		return nil, err
	}
	items := make([]Installation, 0, len(rows))
	for _, row := range rows {
		item := installationFromReference(row)
		count, err := q.CountBotSkillPackageReferences(ctx, row.PackageInstallationID)
		if err != nil {
			return nil, err
		}
		item.PluginReferenceCount = count
		items = append(items, item)
	}
	return items, nil
}

func validateRequirement(requirement Requirement) error {
	if !skills.IsValidRegistryID(requirement.RegistryID) || !skills.IsValidRegistryComponent(requirement.PackageID) || !isDigest(requirement.Revision) {
		return errors.New("skill Package requirement is invalid")
	}
	return nil
}

func isDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func installationWithReferences(ctx context.Context, q Store, row dbsqlc.BotSkillPackageInstallation) (Installation, error) {
	result := installationFromRow(row)
	count, err := q.CountBotSkillPackageReferences(ctx, row.ID)
	if err != nil {
		return Installation{}, err
	}
	result.PluginReferenceCount = count
	return result, nil
}

func installationFromRow(row dbsqlc.BotSkillPackageInstallation) Installation {
	return Installation{
		ID: row.ID.String(), BotID: row.BotID.String(), WorkspaceTargetID: row.WorkspaceTargetID,
		RegistryID: row.RegistryID, PackageID: row.PackageID, Revision: row.Revision,
		DirectlyInstalled: row.DirectlyInstalled, InstalledAt: row.InstalledAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func installationFromReference(row dbsqlc.ListBotPluginPackageReferencesRow) Installation {
	return Installation{
		ID: row.PackageInstallationID.String(), BotID: row.BotID.String(), WorkspaceTargetID: row.WorkspaceTargetID,
		RegistryID: row.RegistryID, PackageID: row.PackageID, Revision: row.Revision,
		DirectlyInstalled: row.DirectlyInstalled,
	}
}
