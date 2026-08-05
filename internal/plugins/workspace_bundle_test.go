package plugins

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
	"github.com/memohai/memoh/internal/workspace/bridgepb"
	"github.com/memohai/memoh/internal/workspace/bridgesvc"
)

const (
	pluginBundleTestInstallationID = "22222222-2222-4222-8222-222222222222"
	pluginBundleTestBotID          = "11111111-1111-4111-8111-111111111111"
)

func TestPluginTargetChanged(t *testing.T) {
	row := sqlc.BotPluginInstallation{Status: StatusReady, WorkspaceTargetID: "target-a"}
	if !pluginTargetChanged(row, "target-b") {
		t.Fatal("pluginTargetChanged() did not detect a workspace move")
	}
	if pluginTargetChanged(row, " target-a ") {
		t.Fatal("pluginTargetChanged() treated the same target as a move")
	}
	row.Status = StatusUninstalled
	if pluginTargetChanged(row, "target-b") {
		t.Fatal("pluginTargetChanged() tried to remove an already uninstalled bundle")
	}
}

type pluginBundleTestQueries struct {
	dbstore.Queries
	row               sqlc.BotPluginInstallation
	resources         []sqlc.BotPluginResource
	packageReferences []sqlc.ListBotPluginPackageReferencesRow
	updateErr         error
	deleted           bool
}

func (q *pluginBundleTestQueries) GetBotPluginInstallationByID(
	context.Context,
	sqlc.GetBotPluginInstallationByIDParams,
) (sqlc.BotPluginInstallation, error) {
	return q.row, nil
}

func (q *pluginBundleTestQueries) ListBotPluginResources(
	context.Context,
	pgtype.UUID,
) ([]sqlc.BotPluginResource, error) {
	return q.resources, nil
}

func (*pluginBundleTestQueries) DeleteMCPConnectionsByPlugin(
	context.Context,
	sqlc.DeleteMCPConnectionsByPluginParams,
) error {
	return nil
}

func (q *pluginBundleTestQueries) DeleteBotPluginResources(context.Context, pgtype.UUID) error {
	q.resources = nil
	return nil
}

func (q *pluginBundleTestQueries) ListBotPluginPackageReferences(
	context.Context,
	pgtype.UUID,
) ([]sqlc.ListBotPluginPackageReferencesRow, error) {
	return q.packageReferences, nil
}

func (q *pluginBundleTestQueries) DeleteBotPluginPackageReferences(context.Context, pgtype.UUID) error {
	q.packageReferences = nil
	return nil
}

func (q *pluginBundleTestQueries) CountBotSkillPackageReferences(context.Context, pgtype.UUID) (int64, error) {
	return int64(len(q.packageReferences)), nil
}

func (q *pluginBundleTestQueries) DeleteBotSkillPackageInstallationIfUnreferenced(
	context.Context,
	sqlc.DeleteBotSkillPackageInstallationIfUnreferencedParams,
) (pgtype.UUID, error) {
	if len(q.packageReferences) != 0 {
		return pgtype.UUID{}, errors.New("Package is still referenced")
	}
	return pgtype.UUID{Bytes: [16]byte{3}, Valid: true}, nil
}

func (q *pluginBundleTestQueries) UpdateBotPluginInstallationStatus(
	_ context.Context,
	arg sqlc.UpdateBotPluginInstallationStatusParams,
) (sqlc.BotPluginInstallation, error) {
	if q.updateErr != nil {
		return sqlc.BotPluginInstallation{}, q.updateErr
	}
	q.row.Status = arg.Status
	q.row.Enabled = arg.Enabled
	return q.row, nil
}

func (q *pluginBundleTestQueries) DeleteBotPluginInstallation(
	context.Context,
	sqlc.DeleteBotPluginInstallationParams,
) error {
	q.deleted = true
	return nil
}

type pluginBundleTestBridgeProvider struct {
	client  *bridge.Client
	targets []string
	err     error
}

func (p *pluginBundleTestBridgeProvider) MCPClient(ctx context.Context, _ string) (*bridge.Client, error) {
	p.targets = append(p.targets, bridge.WorkspaceTargetFromContext(ctx))
	if p.err != nil {
		return nil, p.err
	}
	return p.client, nil
}

func TestPurgeUninstalledPluginDoesNotRequireOriginalWorkspace(t *testing.T) {
	botUUID, err := db.ParseUUID(pluginBundleTestBotID)
	if err != nil {
		t.Fatalf("parse bot ID: %v", err)
	}
	installationUUID, err := db.ParseUUID(pluginBundleTestInstallationID)
	if err != nil {
		t.Fatalf("parse installation ID: %v", err)
	}
	queries := &pluginBundleTestQueries{row: sqlc.BotPluginInstallation{
		ID: installationUUID, BotID: botUUID, PluginID: "notion", PluginName: "Notion",
		Status: StatusUninstalled, Config: []byte(`{}`),
		WorkspaceTargetID: "deleted-remote", Metadata: []byte(`{}`),
		Manifest: []byte(`{"id":"notion","name":"Notion","author":{"name":"Memoh"}}`),
	}}
	provider := &pluginBundleTestBridgeProvider{err: errors.New("remote workspace is gone")}
	service := NewService(
		slog.New(slog.DiscardHandler),
		queries,
		mcp.NewConnectionService(nil, queries),
		mcp.NewOAuthService(nil, queries, ""),
		nil,
		BridgeProvider{Provider: provider},
	)

	if err := service.Purge(context.Background(), pluginBundleTestBotID, pluginBundleTestInstallationID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if !queries.deleted {
		t.Fatal("Purge did not delete the installation")
	}
	if len(provider.targets) != 0 {
		t.Fatalf("Purge resolved unavailable workspace targets: %v", provider.targets)
	}
}

func TestUninstallRemovesPluginBundleAndRestoresItOnDatabaseFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		updateErr error
		wantFile  bool
	}{
		{name: "success"},
		{name: "database failure", updateErr: errors.New("injected database failure"), wantFile: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			client := newPluginBundleTestClient(t, root)
			provider := &pluginBundleTestBridgeProvider{client: client}
			botUUID, err := db.ParseUUID(pluginBundleTestBotID)
			if err != nil {
				t.Fatalf("parse bot ID: %v", err)
			}
			installationUUID, err := db.ParseUUID(pluginBundleTestInstallationID)
			if err != nil {
				t.Fatalf("parse installation ID: %v", err)
			}
			queries := &pluginBundleTestQueries{
				row: sqlc.BotPluginInstallation{
					ID: installationUUID, BotID: botUUID, PluginID: "notion", PluginName: "Notion",
					Status: StatusReady, Enabled: true, Config: []byte(`{}`),
					WorkspaceTargetID: "native", Metadata: []byte(`{}`),
					Manifest: []byte(`{"id":"notion","name":"Notion","author":{"name":"Memoh"}}`),
				},
				updateErr: test.updateErr,
			}
			pluginRoot, err := skillset.PluginDirForID("notion")
			if err != nil {
				t.Fatalf("PluginDirForID: %v", err)
			}
			pluginFile := filepath.Join(root, filepath.FromSlash(pluginRoot[len("/data/"):]), "plugin.yaml")
			if err := os.MkdirAll(filepath.Dir(pluginFile), 0o750); err != nil {
				t.Fatalf("create Plugin directory: %v", err)
			}
			const original = "id: notion\nversion: approved\n"
			if err := os.WriteFile(pluginFile, []byte(original), 0o600); err != nil {
				t.Fatalf("seed Plugin bundle: %v", err)
			}
			skillRoot, err := skillset.SkillDirForIDs("memoh", "notion", "meeting")
			if err != nil {
				t.Fatalf("SkillDirForIDs: %v", err)
			}
			skillFile := filepath.Join(root, filepath.FromSlash(skillRoot[len("/data/"):]), "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(skillFile), 0o750); err != nil {
				t.Fatalf("create Skill directory: %v", err)
			}
			if err := os.WriteFile(skillFile, []byte("---\nname: meeting\n---\n"), 0o600); err != nil {
				t.Fatalf("seed Skill: %v", err)
			}
			queries.resources = []sqlc.BotPluginResource{{
				InstallationID: installationUUID,
				ResourceType:   "skill",
				ResourceKey:    "memoh/notion/meeting",
				ResourceID:     path.Join(skillRoot, "SKILL.md"),
				Status:         "installed",
				Metadata:       []byte(`{"workspace_target_id":"native"}`),
			}}
			queries.packageReferences = []sqlc.ListBotPluginPackageReferencesRow{{
				PluginInstallationID:  installationUUID,
				PackageInstallationID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
				BotID:                 botUUID, WorkspaceTargetID: "native",
				RegistryID: "memoh", PackageID: "notion",
				Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}}

			service := NewService(
				slog.New(slog.DiscardHandler),
				queries,
				mcp.NewConnectionService(nil, queries),
				mcp.NewOAuthService(nil, queries, ""),
				nil,
				BridgeProvider{Provider: provider},
			)
			_, uninstallErr := service.Uninstall(
				context.Background(), pluginBundleTestBotID, pluginBundleTestInstallationID,
			)
			if test.updateErr == nil && uninstallErr != nil {
				t.Fatalf("Uninstall: %v", uninstallErr)
			}
			if test.updateErr != nil && !errors.Is(uninstallErr, test.updateErr) {
				t.Fatalf("Uninstall error = %v, want %v", uninstallErr, test.updateErr)
			}
			//nolint:gosec // pluginFile is constructed below t.TempDir().
			content, readErr := os.ReadFile(pluginFile)
			if test.wantFile {
				if readErr != nil || string(content) != original {
					t.Fatalf("Plugin bundle was not restored: content=%q error=%v", content, readErr)
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("Plugin bundle still exists after uninstall: %v", readErr)
			}
			//nolint:gosec // skillFile is constructed below t.TempDir().
			if _, err := os.ReadFile(skillFile); test.wantFile && err != nil {
				t.Fatalf("Plugin uninstall did not restore its Package: %v", err)
			} else if !test.wantFile && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Plugin uninstall retained its unowned Package: %v", err)
			}
			if len(provider.targets) != 2 || provider.targets[0] != "native" || provider.targets[1] != "native" {
				t.Fatalf("workspace targets = %v, want [native native]", provider.targets)
			}
		})
	}
}

func newPluginBundleTestClient(t *testing.T, root string) *bridge.Client {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	bridgepb.RegisterContainerServiceServer(server, bridgesvc.New(bridgesvc.Options{
		DefaultWorkDir: root,
		WorkspaceRoot:  root,
		DataMount:      "/data",
	}))
	go func() { _ = server.Serve(lis) }()
	conn, err := grpc.NewClient(
		"passthrough:///plugin-bundle-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		_ = lis.Close()
		t.Fatalf("create bridge client: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	})
	return bridge.NewClientFromConn(conn)
}
