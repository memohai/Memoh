package provider

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Factory creates a Provider from a provider type string and JSON config.
// The registry uses factories to lazily instantiate providers from DB rows.
type Factory func(id string, config map[string]any) (Provider, error)

// Registry manages provider instances keyed by their DB id.
// It caches instantiated providers and uses registered factories to create
// them on demand from stored configuration.
type Registry struct {
	mu        sync.RWMutex
	instances map[string]Provider
	factories map[string]Factory
	logger    *slog.Logger
}

func NewRegistry(log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	return &Registry{
		instances: map[string]Provider{},
		factories: map[string]Factory{},
		logger:    log.With(slog.String("component", "memory_provider_registry")),
	}
}

// RegisterFactory registers a factory for a given provider type (e.g. "builtin").
func (r *Registry) RegisterFactory(providerType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[strings.TrimSpace(providerType)] = factory
}

// Register adds a pre-built provider instance by ID.
func (r *Registry) Register(id string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[strings.TrimSpace(id)] = provider
}

// Get returns the provider for the given DB record ID.
func (r *Registry) Get(id string) (Provider, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	r.mu.RLock()
	p, ok := r.instances[id]
	r.mu.RUnlock()
	if ok {
		return p, nil
	}
	return nil, fmt.Errorf("memory provider not found: %s", id)
}

// Instantiate creates a provider from a DB row and caches it.
// If the instance already exists, it is returned directly.
func (r *Registry) Instantiate(id, providerType string, config map[string]any) (Provider, error) {
	id = strings.TrimSpace(id)
	providerType = strings.TrimSpace(providerType)

	r.mu.RLock()
	if p, ok := r.instances[id]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock.
	if p, ok := r.instances[id]; ok {
		return p, nil
	}

	factory, ok := r.factories[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown memory provider type: %s", providerType)
	}
	p, err := factory(id, config)
	if err != nil {
		return nil, fmt.Errorf("instantiate memory provider %s (%s): %w", id, providerType, err)
	}
	r.instances[id] = p
	return p, nil
}

// Remove evicts a cached provider instance (e.g. after config update or delete).
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, strings.TrimSpace(id))
}
