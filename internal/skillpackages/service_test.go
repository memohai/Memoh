package skillpackages

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type packageStoreStub struct {
	Store
	row                dbsqlc.BotSkillPackageInstallation
	referenceCount     int64
	directUpsertErr    error
	pluginUpsertErr    error
	previousReferences []dbsqlc.ListBotPluginPackageReferencesRow
	deletedReferences  bool
	deleteUnreferenced bool
}

func (s *packageStoreStub) CountBotSkillPackageReferences(context.Context, pgtype.UUID) (int64, error) {
	return s.referenceCount, nil
}

func (s *packageStoreStub) GetBotSkillPackageInstallationByID(context.Context, dbsqlc.GetBotSkillPackageInstallationByIDParams) (dbsqlc.BotSkillPackageInstallation, error) {
	if !s.row.ID.Valid {
		return dbsqlc.BotSkillPackageInstallation{}, pgx.ErrNoRows
	}
	return s.row, nil
}

func (s *packageStoreStub) UpsertDirectBotSkillPackageInstallation(context.Context, dbsqlc.UpsertDirectBotSkillPackageInstallationParams) (dbsqlc.BotSkillPackageInstallation, error) {
	return s.row, s.directUpsertErr
}

func (s *packageStoreStub) SetBotSkillPackageDirectlyInstalled(_ context.Context, arg dbsqlc.SetBotSkillPackageDirectlyInstalledParams) (dbsqlc.BotSkillPackageInstallation, error) {
	s.row.DirectlyInstalled = arg.DirectlyInstalled
	return s.row, nil
}

func (s *packageStoreStub) DeleteBotSkillPackageInstallationIfUnreferenced(context.Context, dbsqlc.DeleteBotSkillPackageInstallationIfUnreferencedParams) (pgtype.UUID, error) {
	if !s.deleteUnreferenced {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return s.row.ID, nil
}

func (s *packageStoreStub) ListBotPluginPackageReferences(context.Context, pgtype.UUID) ([]dbsqlc.ListBotPluginPackageReferencesRow, error) {
	return s.previousReferences, nil
}

func (s *packageStoreStub) DeleteBotPluginPackageReferences(context.Context, pgtype.UUID) error {
	s.deletedReferences = true
	return nil
}

func (s *packageStoreStub) UpsertPluginBotSkillPackageInstallation(context.Context, dbsqlc.UpsertPluginBotSkillPackageInstallationParams) (dbsqlc.BotSkillPackageInstallation, error) {
	return s.row, s.pluginUpsertErr
}

func (*packageStoreStub) UpsertBotPluginPackageReference(context.Context, dbsqlc.UpsertBotPluginPackageReferenceParams) (dbsqlc.BotPluginPackageReference, error) {
	return dbsqlc.BotPluginPackageReference{}, nil
}

func TestRecordDirectMapsRevisionConflict(t *testing.T) {
	store := &packageStoreStub{directUpsertErr: pgx.ErrNoRows}
	service := NewService(store)

	_, err := service.RecordDirect(context.Background(), packageUUID(1).String(), "", Requirement{
		RegistryID: "openai",
		PackageID:  "documents",
		Revision:   strings.Repeat("a", 64),
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("RecordDirect() error = %v, want ErrRevisionConflict", err)
	}
}

func TestReleaseDirectKeepsPackageReferencedByPlugin(t *testing.T) {
	row := packageRow(true)
	store := &packageStoreStub{row: row, referenceCount: 1}
	service := NewService(store)

	installation, removed, err := service.ReleaseDirect(context.Background(), row.BotID.String(), row.ID.String())
	if err != nil {
		t.Fatalf("ReleaseDirect() error = %v", err)
	}
	if removed || installation.DirectlyInstalled {
		t.Fatalf("ReleaseDirect() = removed %v, installation %+v", removed, installation)
	}
}

func TestReleaseDirectDeletesPackageWithoutPluginReference(t *testing.T) {
	row := packageRow(true)
	store := &packageStoreStub{row: row, deleteUnreferenced: true}
	service := NewService(store)

	_, removed, err := service.ReleaseDirect(context.Background(), row.BotID.String(), row.ID.String())
	if err != nil {
		t.Fatalf("ReleaseDirect() error = %v", err)
	}
	if !removed {
		t.Fatal("ReleaseDirect() retained a Package with no owner")
	}
}

func TestReplacePluginReferencesMapsRevisionConflict(t *testing.T) {
	row := packageRow(false)
	store := &packageStoreStub{row: row, pluginUpsertErr: pgx.ErrNoRows}
	_, err := ReplacePluginReferences(
		context.Background(),
		store,
		row.BotID,
		packageUUID(3),
		"",
		[]Requirement{{RegistryID: "openai", PackageID: "documents", Revision: strings.Repeat("b", 64)}},
	)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("ReplacePluginReferences() error = %v, want ErrRevisionConflict", err)
	}
}

func TestReplacePluginReferencesDeletesOrphanedPreviousPackage(t *testing.T) {
	row := packageRow(false)
	store := &packageStoreStub{
		row:                row,
		deleteUnreferenced: true,
		previousReferences: []dbsqlc.ListBotPluginPackageReferencesRow{{
			PackageInstallationID: row.ID,
			BotID:                 row.BotID, RegistryID: row.RegistryID, PackageID: row.PackageID,
			Revision: row.Revision, DirectlyInstalled: false,
		}},
	}
	orphaned, err := ReplacePluginReferences(context.Background(), store, row.BotID, packageUUID(3), "", nil)
	if err != nil {
		t.Fatalf("ReplacePluginReferences() error = %v", err)
	}
	if !store.deletedReferences || len(orphaned) != 1 || orphaned[0].PackageID != row.PackageID {
		t.Fatalf("ReplacePluginReferences() orphaned = %+v, deleted references = %v", orphaned, store.deletedReferences)
	}
}

func packageRow(direct bool) dbsqlc.BotSkillPackageInstallation {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return dbsqlc.BotSkillPackageInstallation{
		ID: packageUUID(2), BotID: packageUUID(1),
		RegistryID: "openai", PackageID: "documents", Revision: strings.Repeat("a", 64),
		DirectlyInstalled: direct, InstalledAt: now, UpdatedAt: now,
	}
}

func packageUUID(last byte) pgtype.UUID {
	var value [16]byte
	value[15] = last
	return pgtype.UUID{Bytes: value, Valid: true}
}
