// Package fallback implements storage.Provider that tries a primary provider
// first (e.g. containerfs) and falls back to a secondary (e.g. localfs) on
// failure. Reads check both providers so assets stored by either are reachable.
package fallback

import (
	"context"
	"io"

	"github.com/memohai/memoh/internal/storage"
)

var _ storage.ContainerFileOpener = (*Provider)(nil)

// Provider delegates to primary and falls back to secondary on write errors.
type Provider struct {
	primary   storage.Provider
	secondary storage.Provider
}

// New creates a fallback provider.
func New(primary, secondary storage.Provider) *Provider {
	return &Provider{primary: primary, secondary: secondary}
}

func (p *Provider) Put(ctx context.Context, key string, reader io.Reader) error {
	err := p.primary.Put(ctx, key, reader)
	if err == nil {
		return nil
	}
	if seeker, ok := reader.(io.Seeker); ok {
		if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
			return err
		}
	}
	return p.secondary.Put(ctx, key, reader)
}

func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	rc, err := p.primary.Open(ctx, key)
	if err == nil {
		return rc, nil
	}
	return p.secondary.Open(ctx, key)
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	err := p.primary.Delete(ctx, key)
	if err == nil {
		return nil
	}
	return p.secondary.Delete(ctx, key)
}

func (p *Provider) AccessPath(key string) string {
	return p.primary.AccessPath(key)
}

// ListPrefix delegates to both providers and deduplicates.
func (p *Provider) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	keys, _ := tryListPrefix(ctx, p.primary, prefix)
	secondaryKeys, _ := tryListPrefix(ctx, p.secondary, prefix)
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		seen[k] = struct{}{}
	}
	for _, k := range secondaryKeys {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	return keys, nil
}

func tryListPrefix(ctx context.Context, p storage.Provider, prefix string) ([]string, error) {
	if lister, ok := p.(storage.PrefixLister); ok {
		return lister.ListPrefix(ctx, prefix)
	}
	return nil, nil
}

// OpenContainerFile delegates to whichever inner provider implements
// storage.ContainerFileOpener, trying the primary first.
func (p *Provider) OpenContainerFile(ctx context.Context, botID, containerPath string) (io.ReadCloser, error) {
	if opener, ok := p.primary.(storage.ContainerFileOpener); ok {
		rc, err := opener.OpenContainerFile(ctx, botID, containerPath)
		if err == nil {
			return rc, nil
		}
	}
	if opener, ok := p.secondary.(storage.ContainerFileOpener); ok {
		return opener.OpenContainerFile(ctx, botID, containerPath)
	}
	return nil, storage.ErrContainerFileNotSupported
}
