package plugins

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
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

const pluginBundleTestInstallationID = "22222222-2222-4222-8222-222222222222"

type pluginBundleTestQueries struct {
	dbstore.Queries
	row       sqlc.BotPluginInstallation
	updateErr error
	deleted   bool
}

func (q *pluginBundleTestQueries) GetBotPluginInstallationByID(
	context.Context,
	sqlc.GetBotPluginInstallationByIDParams,
) (sqlc.BotPluginInstallation, error) {
	return q.row, nil
}

func (*pluginBundleTestQueries) ListBotPluginResources(
	context.Context,
	pgtype.UUID,
) ([]sqlc.BotPluginResource, error) {
	return nil, nil
}

func (*pluginBundleTestQueries) DeleteMCPConnectionsByPlugin(
	context.Context,
	sqlc.DeleteMCPConnectionsByPluginParams,
) error {
	return nil
}

func (*pluginBundleTestQueries) DeleteBotPluginResources(context.Context, pgtype.UUID) error {
	return nil
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
	botUUID, err := db.ParseUUID(mutationTestBotID)
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
		Metadata: []byte(`{"workspace_target_id":"deleted-remote"}`),
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

	if err := service.Purge(context.Background(), mutationTestBotID, pluginBundleTestInstallationID); err != nil {
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
			botUUID, err := db.ParseUUID(mutationTestBotID)
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
					Metadata: []byte(`{"workspace_target_id":"native"}`),
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

			service := NewService(
				slog.New(slog.DiscardHandler),
				queries,
				mcp.NewConnectionService(nil, queries),
				mcp.NewOAuthService(nil, queries, ""),
				nil,
				BridgeProvider{Provider: provider},
			)
			_, uninstallErr := service.Uninstall(
				context.Background(), mutationTestBotID, pluginBundleTestInstallationID,
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
			if len(provider.targets) != 1 || provider.targets[0] != "native" {
				t.Fatalf("workspace targets = %v, want [native]", provider.targets)
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
