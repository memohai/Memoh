package capabilities

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/models"
)

//go:embed litellm_snapshot.json.gz
var bundledRegistryFS embed.FS

// DefaultRegistryURL is the LiteLLM static model registry (the same file shipped
// with the litellm package). It is a large JSON object keyed by model name.
const DefaultRegistryURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

const (
	defaultTTL          = 6 * time.Hour
	defaultFetchTimeout = 30 * time.Second
)

// Registry resolves model capabilities from the LiteLLM registry. It keeps an
// in-memory snapshot with a TTL and refreshes lazily in the background. It is
// fail-open: if a refresh fails, the previous snapshot keeps serving. By
// default that previous snapshot is the bundled LiteLLM registry, so
// offline/blocked GitHub access still provides baseline capability discovery.
type Registry struct {
	url     string
	ttl     time.Duration
	client  *http.Client
	logger  *slog.Logger
	fetchFn func(context.Context) (map[string]litellmEntry, error)
	bundled []byte

	mu        sync.RWMutex
	entries   map[string]litellmEntry
	idx       *index
	fetchedAt time.Time

	refreshMu  sync.Mutex
	refreshing bool
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

// withoutBundledSnapshot disables the embedded LiteLLM baseline. Intended for
// tests that need an exactly controlled registry.
func withoutBundledSnapshot() Option { return func(r *Registry) { r.bundled = nil } }

// withFetchFn injects a fetch function (tests).
func withFetchFn(fn func(context.Context) (map[string]litellmEntry, error)) Option {
	return func(r *Registry) { r.fetchFn = fn }
}

// NewRegistry builds a Registry from the bundled LiteLLM snapshot. Remote
// refreshes happen later through Warm/Lookup and never during construction, so
// server startup does not depend on raw GitHub access. The bundled snapshot
// keeps Desktop/local imports useful when raw GitHub is unavailable.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		url:     DefaultRegistryURL,
		ttl:     defaultTTL,
		client:  models.NewProviderHTTPClient(defaultFetchTimeout),
		logger:  slog.Default(),
		bundled: bundledRegistryJSON(),
	}
	for _, o := range opts {
		o(r)
	}
	if r.fetchFn == nil {
		r.fetchFn = r.fetchHTTP
	}
	r.loadBundledSnapshot()
	return r
}

// Lookup resolves capabilities for an upstream model identifier. The bool is
// false when the model could not be matched (or the registry is unavailable and
// nothing is cached). Lookup never waits for network I/O: stale snapshots keep
// serving while a background refresh is attempted.
func (r *Registry) Lookup(ctx context.Context, modelID string) (Capabilities, bool) {
	idx, entries, stale := r.currentSnapshot()
	if stale {
		r.Warm(ctx)
	}
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

// Warm triggers a background snapshot refresh if stale (best-effort, fail-open)
// without returning data. Useful for overlapping the registry fetch with other
// I/O. Only one refresh runs at a time.
func (r *Registry) Warm(ctx context.Context) {
	if !r.needsRefresh() {
		return
	}

	r.refreshMu.Lock()
	if r.refreshing {
		r.refreshMu.Unlock()
		return
	}
	r.refreshing = true
	r.refreshMu.Unlock()

	go func() {
		defer func() {
			r.refreshMu.Lock()
			r.refreshing = false
			r.refreshMu.Unlock()
		}()

		refreshCtx, cancel := refreshContext(ctx)
		defer cancel()
		if err := r.refresh(refreshCtx); err != nil {
			r.logger.Warn("capabilities: registry refresh failed, using cached snapshot",
				slog.String("url", r.url), slog.Any("error", err))
		}
	}()
}

func (r *Registry) currentSnapshot() (*index, map[string]litellmEntry, bool) {
	r.mu.RLock()
	idx, entries := r.idx, r.entries
	stale := idx == nil || time.Since(r.fetchedAt) >= r.ttl
	r.mu.RUnlock()
	return idx, entries, stale
}

func (r *Registry) needsRefresh() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.idx == nil || time.Since(r.fetchedAt) >= r.ttl
}

func (r *Registry) refresh(ctx context.Context) error {
	if !r.needsRefresh() {
		return nil
	}

	fetched, err := r.fetchFn(ctx)
	if err != nil {
		return err
	}

	newIdx := buildIndex(keysOf(fetched))
	r.mu.Lock()
	r.entries = fetched
	r.idx = newIdx
	r.fetchedAt = time.Now()
	r.mu.Unlock()
	r.logger.Info("capabilities: registry refreshed",
		slog.Int("models", len(fetched)))
	return nil
}

func refreshContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), defaultFetchTimeout)
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

func (r *Registry) loadBundledSnapshot() {
	if len(r.bundled) == 0 {
		return
	}
	entries, err := parseRegistry(r.bundled)
	if err != nil {
		r.logger.Warn("capabilities: bundled registry snapshot ignored",
			slog.Any("error", err))
		return
	}
	r.entries = entries
	r.idx = buildIndex(keysOf(entries))
	r.fetchedAt = time.Now()
}

func bundledRegistryJSON() []byte {
	body, err := bundledRegistryFS.ReadFile("litellm_snapshot.json.gz")
	if err != nil {
		return nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	defer func() { _ = reader.Close() }()
	out, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	return out
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
