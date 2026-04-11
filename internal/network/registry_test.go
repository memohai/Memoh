package network

import "testing"

type testProvider struct {
	kind string
	desc ProviderDescriptor
}

func (p testProvider) Kind() string { return p.kind }

func (p testProvider) Descriptor() ProviderDescriptor { return p.desc }

func TestRegistryDescriptorAndCapabilities(t *testing.T) {
	registry := NewRegistry()
	provider := testProvider{
		kind: "SDWAN",
		desc: ProviderDescriptor{
			Kind:        "sdwan",
			DisplayName: "SD-WAN",
			Capabilities: ProviderCapabilities{
				Mesh:        true,
				EgressProxy: true,
			},
		},
	}

	if err := registry.Register(provider); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	desc, ok := registry.GetDescriptor("sdwan")
	if !ok {
		t.Fatal("expected descriptor to be found")
	}
	if desc.DisplayName != "SD-WAN" {
		t.Fatalf("unexpected display name: %q", desc.DisplayName)
	}

	capabilities, ok := registry.GetCapabilities("SDWAN")
	if !ok {
		t.Fatal("expected capabilities to be found")
	}
	if !capabilities.Mesh || !capabilities.EgressProxy {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}
}
