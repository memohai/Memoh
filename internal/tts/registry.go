package tts

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[TtsType]TtsAdapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[TtsType]TtsAdapter)}
}

func (r *Registry) Register(a TtsAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Type()] = a
}

func (r *Registry) Get(name TtsType) (TtsAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("tts adapter not found: %s", name)
	}
	return a, nil
}

func (r *Registry) ListMeta() []ProviderMetaResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metas := make([]ProviderMetaResponse, 0, len(r.adapters))
	for _, a := range r.adapters {
		meta := a.Meta()
		metas = append(metas, ProviderMetaResponse{
			Provider:     string(a.Type()),
			DisplayName:  meta.Provider,
			Description:  meta.Description,
			DefaultModel: a.DefaultModel(),
			Models:       a.Models(),
		})
	}
	return metas
}
