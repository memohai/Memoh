package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const sqliteDialect = "sqlite"

type storePaths struct {
	Dir string
	DB  string
}

func channelStoreRoot(dataRoot string) string {
	root := strings.TrimSpace(dataRoot)
	if root == "" {
		root = "data"
	}
	return filepath.Join(root, "channels", "whatsapp")
}

func finalStorePaths(dataRoot, storeID string) storePaths {
	dir := filepath.Join(channelStoreRoot(dataRoot), strings.TrimSpace(storeID))
	return storePaths{Dir: dir, DB: filepath.Join(dir, "store.db")}
}

func pendingStorePaths(dataRoot, loginID string) storePaths {
	dir := filepath.Join(channelStoreRoot(dataRoot), "_pending", strings.TrimSpace(loginID))
	return storePaths{Dir: dir, DB: filepath.Join(dir, "store.db")}
}

func ensureStoreDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("store dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700) // #nosec G302 -- store directories need the owner execute bit.
}

func openClientStore(ctx context.Context, paths storePaths, log waLog.Logger) (*sqlstore.Container, *whatsmeow.Client, error) {
	if err := ensureStoreDir(paths.Dir); err != nil {
		return nil, nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", filepath.ToSlash(paths.DB))
	container, err := sqlstore.New(ctx, sqliteDialect, dsn, log)
	if err != nil {
		return nil, nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, nil, err
	}
	client := whatsmeow.NewClient(device, log)
	return container, client, nil
}

func secureStoreFiles(paths storePaths) {
	_ = os.Chmod(paths.DB, 0o600)
	_ = os.Chmod(paths.DB+"-wal", 0o600)
	_ = os.Chmod(paths.DB+"-shm", 0o600)
}

func moveStore(src, dst storePaths) error {
	if strings.TrimSpace(src.Dir) == "" || strings.TrimSpace(dst.Dir) == "" {
		return errors.New("store paths are required")
	}
	if err := ensureStoreDir(filepath.Dir(dst.Dir)); err != nil {
		return err
	}
	_ = os.RemoveAll(dst.Dir)
	if err := os.Rename(src.Dir, dst.Dir); err != nil {
		return err
	}
	if err := os.Chmod(dst.Dir, 0o700); err != nil { // #nosec G302 -- store directories need the owner execute bit.
		return err
	}
	secureStoreFiles(dst)
	return nil
}

type storeReplacement struct {
	dst       storePaths
	backupDir string
	hasBackup bool
	committed bool
}

func replaceStore(src, dst storePaths, backupID string) (*storeReplacement, error) {
	if strings.TrimSpace(src.Dir) == "" || strings.TrimSpace(dst.Dir) == "" {
		return nil, errors.New("store paths are required")
	}
	if err := ensureStoreDir(filepath.Dir(dst.Dir)); err != nil {
		return nil, err
	}
	replacement := &storeReplacement{
		dst:       dst,
		backupDir: dst.Dir + ".bak-" + strings.TrimSpace(backupID),
	}
	if replacement.backupDir == dst.Dir+".bak-" {
		replacement.backupDir = dst.Dir + ".bak"
	}
	_ = os.RemoveAll(replacement.backupDir)
	if _, err := os.Stat(dst.Dir); err == nil {
		if err := os.Rename(dst.Dir, replacement.backupDir); err != nil {
			return nil, err
		}
		replacement.hasBackup = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(src.Dir, dst.Dir); err != nil {
		if replacement.hasBackup {
			_ = os.Rename(replacement.backupDir, dst.Dir)
		}
		return nil, err
	}
	if err := os.Chmod(dst.Dir, 0o700); err != nil { // #nosec G302 -- store directories need the owner execute bit.
		replacement.Rollback()
		return nil, err
	}
	secureStoreFiles(dst)
	return replacement, nil
}

func (r *storeReplacement) Commit() {
	if r == nil {
		return
	}
	r.committed = true
	if r.hasBackup {
		_ = os.RemoveAll(r.backupDir)
	}
}

func (r *storeReplacement) Rollback() {
	if r == nil || r.committed {
		return
	}
	_ = os.RemoveAll(r.dst.Dir)
	if r.hasBackup {
		_ = os.Rename(r.backupDir, r.dst.Dir)
	}
}

func removeStore(paths storePaths) error {
	if strings.TrimSpace(paths.Dir) == "" {
		return nil
	}
	return os.RemoveAll(paths.Dir)
}

func validateStoreID(storeID string) error {
	id := strings.TrimSpace(storeID)
	if id == "" {
		return errors.New("whatsapp storeId is required")
	}
	if id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("invalid whatsapp storeId: %q", storeID)
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid whatsapp storeId: %q", storeID)
	}
	return nil
}
