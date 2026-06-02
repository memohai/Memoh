package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/memohai/memoh/internal/models"
)

// DefaultRegistryURL is the LiteLLM static model registry (the same file shipped
// with the litellm package). It is a large JSON object keyed by model name.
const DefaultRegistryURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

const (
	defaultTTL          = 6 * time.Hour
	defaultFetchTimeout = 30 * time.Second
)

// Registry resolves model capabilities from the LiteLLM registry. It keeps an
// in-memory snapshot with a TTL and refreshes lazily. It is fail-open: if a
// refresh fails, the previous snapshot (if any) keeps serving; if nothing has
// ever loaded, Lookup simply reports "unknown" and callers fall back to their
// own defaults. Nothing here ever blocks model import.
type Registry struct {
	url     string
	ttl     time.Duration
	client  *http.Client
	logger  *slog.Logger
	fetchFn func(context.Context) (map[string]litellmEntry, error)
	group   singleflight.Group

	mu        sync.RWMutex
	entries   map[string]litellmEntry
	idx       *index
	fetchedAt time.Time
}

// Option configures a Registry.
type Option func(*Registry)

// WithURL overrides the registry URL.
func WithURL(url string) Option { return func(r *Registry) { r.url = url } }

// WithTTL overrides the snapshot TTL.
func WithTTL(ttl time.Duration) Option { return func(r *Registry) { r.ttl = ttl } }

// WithHTTPClient overrides the HTTP client used for fetching.
func WithHTTPClient(c *http.Client) Option { return func(r *Registry) { r.client = c } }

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option { return func(r *Registry) { r.logger = l } }

// withFetchFn injects a fetch function (tests).
func withFetchFn(fn func(context.Context) (map[string]litellmEntry, error)) Option {
	return func(r *Registry) { r.fetchFn = fn }
}

// NewRegistry builds a Registry. By default it fetches DefaultRegistryURL over
// HTTP with a 30s timeout and caches for 6h.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		url:    DefaultRegistryURL,
		ttl:    defaultTTL,
		client: models.NewProviderHTTPClient(defaultFetchTimeout),
		logger: slog.Default(),
	}
	for _, o := range opts {
		o(r)
	}
	if r.fetchFn == nil {
		r.fetchFn = r.fetchHTTP
	}
	return r
}

// Lookup resolves capabilities for an upstream model identifier. The bool is
// false when the model could not be matched (or the registry is unavailable and
// nothing is cached). Lookup is non-blocking beyond a single concurrent refresh.
func (r *Registry) Lookup(ctx context.Context, modelID string) (Capabilities, bool) {
	idx, entries := r.snapshot(ctx)
	if idx == nil {
		return Capabilities{}, false
	}
	if key, ok := idx.match(modelID); ok {
		if entry, ok := entries[key]; ok {
			return derive(entry), true
		}
	}
	// Fallback for low-latency variants (e.g. "...-fast") that the registry has
	// not catalogued yet: borrow the base model's reasoning SHAPE only. The
	// context window is dropped because latency variants frequently differ from
	// the base (e.g. claude-opus-4.6-fast is 128k vs the base's 1M), so it must
	// come from the upstream provider or the local default, never the base.
	if key, ok := idx.matchLatencyBase(modelID); ok {
		if entry, ok := entries[key]; ok {
			caps := derive(entry)
			caps.ContextWindow = nil
			return caps, true
		}
	}
	return Capabilities{}, false
}

// Warm triggers a snapshot refresh if stale (best-effort, fail-open) without
// returning data. Useful for overlapping the registry fetch with other I/O.
func (r *Registry) Warm(ctx context.Context) {
	r.snapshot(ctx)
}

// snapshot returns the current index/entries, refreshing if stale. On refresh
// failure it returns whatever is cached (possibly nil).
func (r *Registry) snapshot(ctx context.Context) (*index, map[string]litellmEntry) {
	r.mu.RLock()
	fresh := r.idx != nil && time.Since(r.fetchedAt) < r.ttl
	idx, entries := r.idx, r.entries
	r.mu.RUnlock()
	if fresh {
		return idx, entries
	}

	// Coalesce concurrent refreshes; the result is shared.
	_, _, _ = r.group.Do("refresh", func() (any, error) {
		// Re-check freshness inside the flight (another goroutine may have just
		// refreshed).
		r.mu.RLock()
		stillStale := r.idx == nil || time.Since(r.fetchedAt) >= r.ttl
		r.mu.RUnlock()
		if !stillStale {
			return nil, nil
		}

		fetched, err := r.fetchFn(ctx)
		if err != nil {
			r.logger.Warn("capabilities: registry refresh failed, using cached snapshot",
				slog.String("url", r.url), slog.Any("error", err))
			return nil, nil
		}

		newIdx := buildIndex(keysOf(fetched))
		r.mu.Lock()
		r.entries = fetched
		r.idx = newIdx
		r.fetchedAt = time.Now()
		r.mu.Unlock()
		r.logger.Info("capabilities: registry refreshed",
			slog.Int("models", len(fetched)))
		return nil, nil
	})

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.idx, r.entries
}

func (r *Registry) fetchHTTP(ctx context.Context) (map[string]litellmEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, err
	}
	// The registry URL is operator-configured (defaults to the LiteLLM raw JSON),
	// not attacker-controlled, so this fetch is intentional.
	resp, err := r.client.Do(req) //nolint:gosec // configured registry URL
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseRegistry(body)
}

// parseRegistry decodes the LiteLLM registry JSON. It skips the non-model
// "sample_spec" sentinel key and tolerates per-entry decode noise.
func parseRegistry(body []byte) (map[string]litellmEntry, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]litellmEntry, len(raw))
	for key, rawEntry := range raw {
		if key == "sample_spec" {
			continue
		}
		var entry litellmEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			// Some records are not objects (or have unexpected shapes); skip them
			// rather than failing the whole registry.
			continue
		}
		out[key] = entry
	}
	if len(out) == 0 {
		return nil, errors.New("registry contained no usable entries")
	}
	return out, nil
}

func keysOf(m map[string]litellmEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
