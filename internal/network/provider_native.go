package network

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

type nativeProvider struct {
	kind         string
	displayName  string
	description  string
	configSchema ConfigSchema
	capabilities ProviderCapabilities
	runtime      nativeClientRuntime
	stateRoot    string
}

func RegisterBuiltinProviders(registry *Registry, runtime nativeClientRuntime, stateRoot string) error {
	if registry == nil {
		return nil
	}
	providers := []Provider{
		newTailscaleProvider(runtime, stateRoot),
		newNetbirdProvider(runtime, stateRoot),
	}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProvider) Kind() string { return p.kind }

func (p *nativeProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Kind:         p.kind,
		DisplayName:  p.displayName,
		Description:  p.description,
		ConfigSchema: p.configSchema,
		Capabilities: p.capabilities,
		Actions: []ProviderAction{
			{
				ID:      "test_connection",
				Type:    ActionTypeTestConnection,
				Label:   "Test Connection",
				Primary: true,
			},
			{
				ID:    "logout",
				Label: "Log Out",
			},
		},
	}
}

func (p *nativeProvider) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	config := NormalizeConfigBySchema(p.configSchema, raw)
	if err := ValidateConfigBySchema(p.configSchema, config); err != nil {
		return nil, fmt.Errorf("invalid %s config: %w", p.kind, err)
	}
	return config, nil
}

func (p *nativeProvider) Status(_ context.Context, cfg BotNetworkConfig) (ProviderStatus, error) {
	if !cfg.Enabled {
		return ProviderStatus{
			State:       StatusStateNeedsConfig,
			Title:       "Disabled",
			Description: "This network provider is disabled.",
		}, nil
	}
	if _, err := p.NormalizeConfig(cfg.Config); err != nil {
		return ProviderStatus{
			State:       StatusStateNeedsConfig,
			Title:       "Config Required",
			Description: err.Error(),
		}, nil
	}
	if err := p.validateConfig(cfg); err != nil {
		return ProviderStatus{
			State:       StatusStateNeedsConfig,
			Title:       "Config Required",
			Description: err.Error(),
		}, nil
	}
	return ProviderStatus{
		State:       StatusStateReady,
		Title:       "Ready",
		Description: "Provider configuration is valid.",
	}, nil
}

func (p *nativeProvider) ExecuteAction(ctx context.Context, cfg BotNetworkConfig, actionID string, _ map[string]any) (ProviderActionExecution, error) {
	switch actionID {
	case "test_connection":
		status, err := p.Status(ctx, cfg)
		return ProviderActionExecution{
			ActionID: actionID,
			Status:   status,
			Output: map[string]any{
				"provider": cfg.Provider,
			},
		}, err
	default:
		return ProviderActionExecution{}, fmt.Errorf("unsupported network action %q", actionID)
	}
}

func (p *nativeProvider) ListNodes(ctx context.Context, botID string, cfg BotNetworkConfig) ([]NodeOption, error) {
	if err := p.validateConfig(cfg); err != nil {
		return nil, err
	}
	switch p.kind {
	case "tailscale":
		driver := &nativeClientDriver{
			kind:      p.kind,
			config:    cfg,
			runtime:   p.runtime,
			stateRoot: filepath.Join(p.stateRoot, "network"),
		}
		return driver.listTailscaleNodes(ctx, botID)
	default:
		return nil, fmt.Errorf("%s does not expose exit node discovery", p.kind)
	}
}

func (p *nativeProvider) BuildDriver(cfg BotNetworkConfig) (OverlayDriver, error) {
	config, err := p.NormalizeConfig(cfg.Config)
	if err != nil {
		return nil, err
	}
	cfg.Config = config
	if err := p.validateConfig(cfg); err != nil {
		return nil, err
	}
	return &nativeClientDriver{
		kind:      p.kind,
		config:    cfg,
		runtime:   p.runtime,
		stateRoot: filepath.Join(p.stateRoot, "network"),
	}, nil
}

func newTailscaleProvider(runtime nativeClientRuntime, stateRoot string) Provider {
	return &nativeProvider{
		kind:        "tailscale",
		displayName: "Tailscale",
		description: "Hidden native tailscaled sidecar attached to the workspace netns.",
		configSchema: ConfigSchema{
			Version: 1,
			Title:   "Tailscale",
			Fields: []ConfigField{
				{Key: "hostname", Type: FieldTypeString, Title: "Hostname", Order: 1},
				// Collapsed — auth & advanced options
				{Key: "auth_key", Type: FieldTypeSecret, Title: "Auth Key", Collapsed: true, Description: "Leave empty to use interactive browser login.", Order: 10},
				{Key: "control_url", Type: FieldTypeString, Title: "Control URL", Collapsed: true, Description: "Leave empty for official Tailscale. Set to your Headscale URL for self-hosted.", Order: 11},
				{Key: "accept_routes", Type: FieldTypeBool, Title: "Accept Routes", Collapsed: true, Default: false, Order: 11},
				{Key: "accept_dns", Type: FieldTypeBool, Title: "Accept DNS", Collapsed: true, Default: true, Order: 12},
				{Key: "advertise_routes", Type: FieldTypeString, Title: "Advertise Routes", Collapsed: true, Order: 13},
				{Key: "userspace", Type: FieldTypeBool, Title: "Userspace Networking", Collapsed: true, Default: false, Order: 14},
				{Key: "socks5_enabled", Type: FieldTypeBool, Title: "Enable SOCKS5 Proxy", Collapsed: true, Default: false, Order: 15},
				{Key: "socks5_port", Type: FieldTypeNumber, Title: "SOCKS5 Port", Collapsed: true, Default: float64(1055), Order: 16},
				{Key: "http_proxy_enabled", Type: FieldTypeBool, Title: "Enable HTTP Proxy", Collapsed: true, Default: false, Order: 17},
				{Key: "http_proxy_port", Type: FieldTypeNumber, Title: "HTTP Proxy Port", Collapsed: true, Default: float64(1056), Order: 18},
				{Key: "state_dir", Type: FieldTypeString, Title: "State Directory", Collapsed: true, Default: "/var/lib/tailscale", Order: 19},
				{Key: "extra_args", Type: FieldTypeTextarea, Title: "Extra Args", Collapsed: true, Order: 20},
			},
		},
		capabilities: ProviderCapabilities{
			Mesh:              true,
			PrivateDNS:        true,
			EgressProxy:       true,
			TransparentProxy:  true,
			ExitNode:          true,
			Userspace:         true,
			KernelTUN:         true,
			NativeClient:      true,
			SharedNetNSWorker: true,
		},
		runtime:   runtime,
		stateRoot: stateRoot,
	}
}

func (p *nativeProvider) validateConfig(cfg BotNetworkConfig) error {
	if p.kind == "tailscale" && boolConfig(cfg.Config, "userspace") && stringConfig(cfg.Config, "exit_node") != "" {
		return errors.New("tailscale transparent egress via exit node requires userspace=false")
	}
	return nil
}

func newNetbirdProvider(runtime nativeClientRuntime, stateRoot string) Provider {
	return &nativeProvider{
		kind:        "netbird",
		displayName: "NetBird",
		description: "Hidden native NetBird sidecar attached to the workspace netns.",
		configSchema: ConfigSchema{
			Version: 1,
			Title:   "NetBird",
			Fields: []ConfigField{
				{Key: "hostname", Type: FieldTypeString, Title: "Hostname", Order: 1},
				// Collapsed — auth & advanced options
				{Key: "setup_key", Type: FieldTypeSecret, Title: "Setup Key", Collapsed: true, Description: "Leave empty to use interactive SSO login.", Order: 10},
				{Key: "management_url", Type: FieldTypeString, Title: "Management URL", Collapsed: true, Description: "Leave empty for official NetBird Cloud.", Order: 11},
				{Key: "admin_url", Type: FieldTypeString, Title: "Admin URL", Collapsed: true, Order: 12},
				{Key: "disable_dns", Type: FieldTypeBool, Title: "Disable DNS", Collapsed: true, Default: false, Order: 12},
				{Key: "userspace", Type: FieldTypeBool, Title: "Userspace Networking", Collapsed: true, Default: false, Order: 13},
				{Key: "state_dir", Type: FieldTypeString, Title: "State Directory", Collapsed: true, Default: "/var/lib/netbird", Order: 14},
				{Key: "extra_args", Type: FieldTypeTextarea, Title: "Extra Args", Collapsed: true, Order: 15},
			},
		},
		capabilities: ProviderCapabilities{
			Mesh:              true,
			PrivateDNS:        true,
			Userspace:         true,
			KernelTUN:         true,
			NativeClient:      true,
			SharedNetNSWorker: true,
		},
		runtime:   runtime,
		stateRoot: stateRoot,
	}
}
