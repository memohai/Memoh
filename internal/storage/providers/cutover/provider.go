// Package cutover implements the temporary read compatibility layer used
// while a deployment moves its canonical media store to a new provider.
package cutover

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/memohai/memoh/internal/storage"
)

var (
	_ storage.Provider     = (*Provider)(nil)
	_ storage.PrefixLister = (*Provider)(nil)
)

// Provider sends every new write to primary. Reads consult legacy only when
// primary definitively reports ErrNotFound; outages and authorization failures
// are never hidden by legacy storage.
type Provider struct {
	primary storage.Provider
	legacy  storage.Provider
}

func New(primary, legacy storage.Provider) (*Provider, error) {
	if primary == nil {
		return nil, errors.New("cutover primary provider is required")
	}
	if legacy == nil {
		return nil, errors.New("cutover legacy provider is required")
	}
	return &Provider{primary: primary, legacy: legacy}, nil
}

func (p *Provider) Put(ctx context.Context, key string, reader io.Reader) error {
	return p.primary.Put(ctx, key, reader)
}

func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	rc, err := p.primary.Open(ctx, key)
	if err == nil {
		return rc, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("open primary: %w", err)
	}
	rc, legacyErr := p.legacy.Open(ctx, key)
	if legacyErr != nil {
		return nil, fmt.Errorf("open legacy after primary miss: %w", legacyErr)
	}
	return rc, nil
}

// Delete removes both copies so a deleted legacy object cannot reappear via
// the read fallback after its primary copy is removed.
func (p *Provider) Delete(ctx context.Context, key string) error {
	var errs []error
	if err := p.primary.Delete(ctx, key); err != nil {
		errs = append(errs, fmt.Errorf("delete primary: %w", err))
	}
	if err := p.legacy.Delete(ctx, key); err != nil {
		errs = append(errs, fmt.Errorf("delete legacy: %w", err))
	}
	return errors.Join(errs...)
}

// AccessPath deliberately exposes only the canonical provider. Returning a
// legacy filesystem path would make callers depend on the compatibility layer
// that is intended to be removed after backfill.
func (p *Provider) AccessPath(ctx context.Context, key string) string {
	return p.primary.AccessPath(ctx, key)
}

// ListPrefix merges canonical and legacy keys. A primary listing error is
// propagated because it may represent an S3 outage; it must not be mistaken
// for an empty bucket.
func (p *Provider) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	primaryKeys, err := listPrefix(ctx, p.primary, prefix)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("list primary: %w", err)
	}
	legacyKeys, legacyErr := listPrefix(ctx, p.legacy, prefix)
	if legacyErr != nil && !errors.Is(legacyErr, storage.ErrNotFound) {
		return nil, fmt.Errorf("list legacy: %w", legacyErr)
	}

	seen := make(map[string]struct{}, len(primaryKeys)+len(legacyKeys))
	keys := make([]string, 0, len(primaryKeys)+len(legacyKeys))
	for _, key := range append(primaryKeys, legacyKeys...) {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

func listPrefix(ctx context.Context, provider storage.Provider, prefix string) ([]string, error) {
	lister, ok := provider.(storage.PrefixLister)
	if !ok {
		return nil, nil
	}
	return lister.ListPrefix(ctx, prefix)
}
