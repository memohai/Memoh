package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Registry holds all registered channel adapters and provides dispatch methods
// for configuration normalization, target resolution, and binding operations.
// It replaces the former global registry, and must be created via NewRegistry
// and passed explicitly to components that need it.
type Registry struct {
	mu       sync.RWMutex
	adapters map[Type]Adapter
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: map[Type]Adapter{},
	}
}

// Register adds an adapter to the registry.
func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return errors.New("adapter is nil")
	}
	ct := normalizeType(adapter.Type().String())
	if ct == "" {
		return errors.New("channel type is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[ct]; exists {
		return fmt.Errorf("channel type already registered: %s", ct)
	}
	r.adapters[ct] = adapter
	return nil
}

// MustRegister calls Register and panics on error.
func (r *Registry) MustRegister(adapter Adapter) {
	if err := r.Register(adapter); err != nil {
		panic(err)
	}
}

// Unregister removes a channel type from the registry.
func (r *Registry) Unregister(channelType Type) bool {
	ct := normalizeType(channelType.String())
	if ct == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[ct]; !exists {
		return false
	}
	delete(r.adapters, ct)
	return true
}

// Get returns the adapter for the given channel type.
func (r *Registry) Get(channelType Type) (Adapter, bool) {
	ct := normalizeType(channelType.String())
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[ct]
	return adapter, ok
}

// DirectoryAdapter returns the directory adapter for the given channel type if it implements DirectoryAdapter.
func (r *Registry) DirectoryAdapter(channelType Type) (DirectoryAdapter, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	dir, ok := adapter.(DirectoryAdapter)
	return dir, ok
}

// List returns all registered adapters.
func (r *Registry) List() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		items = append(items, a)
	}
	return items
}

// Types returns all registered channel types.
func (r *Registry) Types() []Type {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Type, 0, len(r.adapters))
	for ct := range r.adapters {
		items = append(items, ct)
	}
	return items
}

// --- Descriptor accessors ---

// GetDescriptor returns the descriptor for the given channel type.
func (r *Registry) GetDescriptor(channelType Type) (Descriptor, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return Descriptor{}, false
	}
	return adapter.Descriptor(), true
}

// ListDescriptors returns descriptors for all registered channel types.
func (r *Registry) ListDescriptors() []Descriptor {
	adapters := r.List()
	items := make([]Descriptor, 0, len(adapters))
	for _, a := range adapters {
		items = append(items, a.Descriptor())
	}
	return items
}

// ParseChannelType validates and normalizes a raw string into a registered Type.
func (r *Registry) ParseChannelType(raw string) (Type, error) {
	ct := normalizeType(raw)
	if ct == "" {
		return "", fmt.Errorf("unsupported channel type: %s", raw)
	}
	if _, ok := r.Get(ct); !ok {
		return "", fmt.Errorf("unsupported channel type: %s", raw)
	}
	return ct, nil
}

// --- Capability accessors ---

// GetCapabilities returns the capability matrix for the given channel type.
func (r *Registry) GetCapabilities(channelType Type) (Capabilities, bool) {
	desc, ok := r.GetDescriptor(channelType)
	if !ok {
		return Capabilities{}, false
	}
	return desc.Capabilities, true
}

// GetOutboundPolicy returns the outbound policy for the given channel type.
func (r *Registry) GetOutboundPolicy(channelType Type) (OutboundPolicy, bool) {
	desc, ok := r.GetDescriptor(channelType)
	if !ok {
		return OutboundPolicy{}, false
	}
	return desc.OutboundPolicy, true
}

// GetConfigSchema returns the configuration schema for the given channel type.
func (r *Registry) GetConfigSchema(channelType Type) (ConfigSchema, bool) {
	desc, ok := r.GetDescriptor(channelType)
	if !ok {
		return ConfigSchema{}, false
	}
	return desc.ConfigSchema, true
}

// GetUserConfigSchema returns the user-binding configuration schema.
func (r *Registry) GetUserConfigSchema(channelType Type) (ConfigSchema, bool) {
	desc, ok := r.GetDescriptor(channelType)
	if !ok {
		return ConfigSchema{}, false
	}
	return desc.UserConfigSchema, true
}

// IsConfigless reports whether the channel type operates without per-bot configuration.
func (r *Registry) IsConfigless(channelType Type) bool {
	desc, ok := r.GetDescriptor(channelType)
	if !ok {
		return false
	}
	return desc.Configless
}

// --- Sender / Receiver accessors ---

// GetSender returns the Sender for the given channel type, or nil if unsupported.
func (r *Registry) GetSender(channelType Type) (Sender, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	sender, ok := adapter.(Sender)
	return sender, ok
}

// GetStreamSender returns the StreamSender for the given channel type, or nil if unsupported.
func (r *Registry) GetStreamSender(channelType Type) (StreamSender, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	streamSender, ok := adapter.(StreamSender)
	return streamSender, ok
}

// GetMessageEditor returns the MessageEditor for the given channel type, or nil if unsupported.
func (r *Registry) GetMessageEditor(channelType Type) (MessageEditor, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	editor, ok := adapter.(MessageEditor)
	return editor, ok
}

// GetReactor returns the Reactor for the given channel type, or nil if unsupported.
func (r *Registry) GetReactor(channelType Type) (Reactor, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	reactor, ok := adapter.(Reactor)
	return reactor, ok
}

// GetReceiver returns the Receiver for the given channel type, or nil if unsupported.
func (r *Registry) GetReceiver(channelType Type) (Receiver, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	receiver, ok := adapter.(Receiver)
	return receiver, ok
}

// GetProcessingStatusNotifier returns the ProcessingStatusNotifier for the given channel type, or nil if unsupported.
func (r *Registry) GetProcessingStatusNotifier(channelType Type) (ProcessingStatusNotifier, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, false
	}
	notifier, ok := adapter.(ProcessingStatusNotifier)
	return notifier, ok
}

// DiscoverSelf calls the SelfDiscoverer for the given channel type if supported.
func (r *Registry) DiscoverSelf(ctx context.Context, channelType Type, credentials map[string]any) (map[string]any, string, error) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, "", fmt.Errorf("unsupported channel type: %s", channelType)
	}
	discoverer, ok := adapter.(SelfDiscoverer)
	if !ok {
		return nil, "", nil
	}
	return discoverer.DiscoverSelf(ctx, credentials)
}

// --- Dispatch methods (replace former global functions in config.go / target.go) ---

// NormalizeConfig validates and normalizes a channel configuration map.
func (r *Registry) NormalizeConfig(channelType Type, raw map[string]any) (map[string]any, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, fmt.Errorf("unsupported channel type: %s", channelType)
	}
	if normalizer, ok := adapter.(ConfigNormalizer); ok {
		return normalizer.NormalizeConfig(raw)
	}
	return raw, nil
}

// NormalizeUserConfig validates and normalizes a user-channel binding configuration.
func (r *Registry) NormalizeUserConfig(channelType Type, raw map[string]any) (map[string]any, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	adapter, ok := r.Get(channelType)
	if !ok {
		return nil, fmt.Errorf("unsupported channel type: %s", channelType)
	}
	if normalizer, ok := adapter.(ConfigNormalizer); ok {
		return normalizer.NormalizeUserConfig(raw)
	}
	return raw, nil
}

// ResolveTargetFromUserConfig derives a delivery target from a user-channel binding.
func (r *Registry) ResolveTargetFromUserConfig(channelType Type, config map[string]any) (string, error) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return "", fmt.Errorf("unsupported channel type: %s", channelType)
	}
	if resolver, ok := adapter.(TargetResolver); ok {
		return resolver.ResolveTarget(config)
	}
	return "", fmt.Errorf("channel type %s does not support target resolution", channelType)
}

// NormalizeTarget applies the channel-specific target normalization.
func (r *Registry) NormalizeTarget(channelType Type, raw string) (string, bool) {
	adapter, ok := r.Get(channelType)
	if !ok {
		return strings.TrimSpace(raw), false
	}
	if resolver, ok := adapter.(TargetResolver); ok {
		normalized := strings.TrimSpace(resolver.NormalizeTarget(raw))
		if normalized == "" {
			return "", false
		}
		return normalized, true
	}
	return strings.TrimSpace(raw), false
}

// MatchUserBinding reports whether the given binding config matches the criteria.
func (r *Registry) MatchUserBinding(channelType Type, config map[string]any, criteria BindingCriteria) bool {
	adapter, ok := r.Get(channelType)
	if !ok {
		return false
	}
	if matcher, ok := adapter.(BindingMatcher); ok {
		return matcher.MatchBinding(config, criteria)
	}
	return false
}

// BuildUserBindingConfig constructs a user-channel binding config from an Identity.
func (r *Registry) BuildUserBindingConfig(channelType Type, identity Identity) map[string]any {
	adapter, ok := r.Get(channelType)
	if !ok {
		return map[string]any{}
	}
	if matcher, ok := adapter.(BindingMatcher); ok {
		return matcher.BuildUserConfig(identity)
	}
	return map[string]any{}
}

func normalizeType(raw string) Type {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return ""
	}
	return Type(normalized)
}
