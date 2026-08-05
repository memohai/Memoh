package backfill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FilesystemSource struct {
	name string
	root string
}

func NewFilesystemSource(name, root string) *FilesystemSource {
	return &FilesystemSource{name: name, root: filepath.Clean(root)}
}

func (s *FilesystemSource) Name() string { return s.name }

func (s *FilesystemSource) Walk(ctx context.Context, visit Visitor) error {
	if visit == nil {
		return errors.New("backfill visitor is required")
	}
	if strings.TrimSpace(s.root) == "" || s.root == "." {
		return errors.New("backfill filesystem root is required")
	}
	err := filepath.WalkDir(s.root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		return visit(Object{
			Key:       key,
			SizeBytes: info.Size(),
			Open: func(context.Context) (io.ReadCloser, error) {
				return os.Open(filePath) //nolint:gosec // path is produced by WalkDir below the configured root
			},
		})
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walk %q: %w", s.root, err)
	}
	return nil
}
