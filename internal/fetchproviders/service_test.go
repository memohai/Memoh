package fetchproviders

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
)

func TestEnsureDefaultsCreatesFetchProviders(t *testing.T) {
	ctx := context.Background()
	conn, svc := newFetchProviderTestService(t, ctx)

	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	list, err := svc.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	byProvider := map[string]GetResponse{}
	for _, item := range list {
		byProvider[item.Provider] = item
	}

	if len(byProvider) != 3 {
		t.Fatalf("providers len = %d, want 3", len(byProvider))
	}
	if got := byProvider[string(ProviderNative)]; got.Name != "Native" || !got.Enable {
		t.Fatalf("native = %+v, want enabled Native", got)
	}
	if got := byProvider[string(ProviderJina)]; got.Name != "Jina Reader" || got.Enable {
		t.Fatalf("jina = %+v, want disabled Jina Reader", got)
	}
	if got := byProvider[string(ProviderCloudflareMarkdown)]; got.Name != "Cloudflare Markdown" || got.Enable {
		t.Fatalf("cloudflare = %+v, want disabled Cloudflare Markdown", got)
	}

	if _, err := conn.ExecContext(ctx, `UPDATE fetch_providers SET enable = 0 WHERE provider = ?`, string(ProviderNative)); err != nil {
		t.Fatalf("disable native directly: %v", err)
	}
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatalf("EnsureDefaults() re-enable error = %v", err)
	}
	nativeRows, err := svc.List(ctx, string(ProviderNative))
	if err != nil {
		t.Fatalf("List(native) error = %v", err)
	}
	if len(nativeRows) != 1 || !nativeRows[0].Enable {
		t.Fatalf("native rows = %+v, want one enabled native", nativeRows)
	}
}

func TestNativeFetchProviderIsManaged(t *testing.T) {
	ctx := context.Background()
	_, svc := newFetchProviderTestService(t, ctx)

	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}
	nativeRows, err := svc.List(ctx, string(ProviderNative))
	if err != nil {
		t.Fatalf("List(native) error = %v", err)
	}
	nativeID := nativeRows[0].ID

	if _, err := svc.Create(ctx, CreateRequest{Name: "Another Native", Provider: ProviderNative}); !errors.Is(err, ErrManagedNativeProvider) {
		t.Fatalf("Create(native) error = %v, want ErrManagedNativeProvider", err)
	}
	disabled := false
	if _, err := svc.Update(ctx, nativeID, UpdateRequest{Enable: &disabled}); !errors.Is(err, ErrManagedNativeProvider) {
		t.Fatalf("Update(native disable) error = %v, want ErrManagedNativeProvider", err)
	}
	if err := svc.Delete(ctx, nativeID); !errors.Is(err, ErrManagedNativeProvider) {
		t.Fatalf("Delete(native) error = %v, want ErrManagedNativeProvider", err)
	}
}

func newFetchProviderTestService(t *testing.T, ctx context.Context) (*sql.DB, *Service) {
	t.Helper()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE fetch_providers (
  id TEXT PRIMARY KEY DEFAULT (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  ),
  name TEXT NOT NULL UNIQUE,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  enable INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, NewService(slog.New(slog.DiscardHandler), sqlitestore.NewQueries(store))
}
