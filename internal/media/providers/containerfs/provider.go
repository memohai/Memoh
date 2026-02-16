// Package containerfs implements media.StorageProvider for bot containers
// backed by host-side bind mounts. Writing to <dataRoot>/bots/<bot_id>/media/<subpath>
// on the host makes the file available at /data/media/<subpath> inside the container.
package containerfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const containerMediaRoot = "/data/media"

// Provider stores media assets via the host-side bind mount path
// that maps to /data inside bot containers.
type Provider struct {
	dataRoot string
}

// New creates a container-based storage provider.
// dataRoot is the host directory that contains per-bot data (e.g. "data").
func New(dataRoot string) (*Provider, error) {
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	return &Provider{dataRoot: abs}, nil
}

// Put writes data to the host bind mount path for the bot container.
func (p *Provider) Put(_ context.Context, key string, reader io.Reader) error {
	dest, err := p.hostPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// Open reads a file from the host bind mount path.
func (p *Provider) Open(_ context.Context, key string) (io.ReadCloser, error) {
	dest, err := p.hostPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(dest)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

// Delete removes a file from the host bind mount path.
func (p *Provider) Delete(_ context.Context, key string) error {
	dest, err := p.hostPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// AccessPath returns the container-internal path for a storage key.
// Key format: "<bot_id>/<subpath>" → "/data/media/<subpath>".
func (p *Provider) AccessPath(key string) string {
	sub := key
	if idx := strings.IndexByte(sub, '/'); idx >= 0 {
		sub = sub[idx+1:]
	}
	return containerMediaRoot + "/" + sub
}

// hostPath converts a storage key into the host-side file path.
// Key format: "<bot_id>/<subpath>" → "<dataRoot>/bots/<bot_id>/media/<subpath>".
func (p *Provider) hostPath(key string) (string, error) {
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute key is forbidden: %s", key)
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("path traversal is forbidden: %s", key)
	}
	idx := strings.IndexByte(clean, filepath.Separator)
	if idx <= 0 {
		return "", fmt.Errorf("storage key must contain bot_id prefix: %s", key)
	}
	botID := clean[:idx]
	subPath := clean[idx+1:]
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(subPath) == "" {
		return "", fmt.Errorf("invalid storage key: %s", key)
	}
	joined := filepath.Join(p.dataRoot, "bots", botID, "media", subPath)
	if !strings.HasPrefix(joined, p.dataRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes data root: %s", key)
	}
	return joined, nil
}
