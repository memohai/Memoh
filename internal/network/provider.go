package network

import "context"

// Provider is the read-only descriptor entry registered for a network provider.
type Provider interface {
	Kind() string
	Descriptor() ProviderDescriptor
}

// OverlayDriver is the behavioral surface for future SD-WAN/overlay providers.
type OverlayDriver interface {
	Provider
	EnsureAttached(ctx context.Context, req AttachmentRequest) (OverlayStatus, error)
	Detach(ctx context.Context, req AttachmentRequest) error
	Status(ctx context.Context, req AttachmentRequest) (OverlayStatus, error)
}

// ProviderDescriptor is the read-only metadata exported by a provider.
type ProviderDescriptor struct {
	Kind         string               `json:"kind"`
	DisplayName  string               `json:"display_name"`
	Capabilities ProviderCapabilities `json:"capabilities"`
}

// ProviderCapabilities describes the feature matrix of an overlay provider.
type ProviderCapabilities struct {
	Mesh         bool `json:"mesh"`
	PrivateDNS   bool `json:"private_dns"`
	ServiceProxy bool `json:"service_proxy"`
	EgressProxy  bool `json:"egress_proxy"`
	ExitNode     bool `json:"exit_node"`
	Userspace    bool `json:"userspace"`
	KernelTUN    bool `json:"kernel_tun"`
}
