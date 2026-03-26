package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultExtensionsConfigFile is the default filename for extensions config
	DefaultExtensionsConfigFile = "extensions_config.json"
	// DefaultPublicCategory is the category for public/built-in skills
	DefaultPublicCategory = "public"
	// DefaultCustomCategory is the category for custom/user-defined skills
	DefaultCustomCategory = "custom"
)

// ExtensionsConfig manages the state of skills and MCP servers.
// This is aligned with DeerFlow's extensions_config.json format.
type ExtensionsConfig struct {
	// Version is the config format version
	Version string `json:"version"`

	// UpdatedAt is the last modification timestamp
	UpdatedAt time.Time `json:"updated_at"`

	// Skills maps skill name to its state
	Skills map[string]SkillState `json:"skills"`

	// MCPServers maps server name to its configuration
	MCPServers map[string]MCPState `json:"mcp_servers,omitempty"`

	// Settings contains global configuration
	Settings Settings `json:"settings,omitempty"`

	// Internal fields (not serialized)
	mu       sync.RWMutex `json:"-"`
	filePath string       `json:"-"`
}

// SkillState represents the runtime state of a skill.
type SkillState struct {
	// Enabled indicates whether the skill is active
	Enabled bool `json:"enabled"`

	// AutoLoad indicates whether the skill should be automatically loaded into prompts
	AutoLoad bool `json:"auto_load"`

	// Category is either "public" or "custom"
	Category string `json:"category"`

	// InstalledAt records when the skill was first installed
	InstalledAt time.Time `json:"installed_at"`

	// UpdatedAt records when the skill state was last modified
	UpdatedAt time.Time `json:"updated_at"`

	// Source records where the skill was installed from (e.g., "upload", "import", "builtin")
	Source string `json:"source,omitempty"`

	// Version of the installed skill
	Version string `json:"version,omitempty"`
}

// MCPState represents the state of an MCP server configuration.
type MCPState struct {
	// Enabled indicates whether the MCP server is active
	Enabled bool `json:"enabled"`

	// Type is the transport type: "stdio", "sse", or "http"
	Type string `json:"type"`

	// Command for stdio transport
	Command string `json:"command,omitempty"`

	// Args for stdio transport
	Args []string `json:"args,omitempty"`

	// Env for stdio transport
	Env map[string]string `json:"env,omitempty"`

	// URL for sse/http transport
	URL string `json:"url,omitempty"`

	// Headers for http/sse transport
	Headers map[string]string `json:"headers,omitempty"`

	// Description of the MCP server
	Description string `json:"description,omitempty"`
}

// Settings contains global extensions configuration.
type Settings struct {
	// DefaultCategory is the default category for new skills
	DefaultCategory string `json:"default_category,omitempty"`

	// LoadOrder defines the order in which skills are loaded
	LoadOrder []string `json:"load_order,omitempty"`

	// MaxSkillContentSize is the maximum allowed size for skill content
	MaxSkillContentSize int `json:"max_skill_content_size,omitempty"`
}

// NewExtensionsConfig creates a new ExtensionsConfig with defaults.
func NewExtensionsConfig() *ExtensionsConfig {
	return &ExtensionsConfig{
		Version:    "1.0.0",
		UpdatedAt:  time.Now().UTC(),
		Skills:     make(map[string]SkillState),
		MCPServers: make(map[string]MCPState),
		Settings: Settings{
			DefaultCategory:     DefaultCustomCategory,
			MaxSkillContentSize: 10 * 1024 * 1024, // 10MB
		},
	}
}

// LoadExtensionsConfigFromBytes parses extensions config from bytes.
func LoadExtensionsConfigFromBytes(data []byte) (*ExtensionsConfig, error) {
	config := NewExtensionsConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse extensions config: %w", err)
	}
	// Ensure maps are initialized
	if config.Skills == nil {
		config.Skills = make(map[string]SkillState)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPState)
	}
	return config, nil
}

// LoadExtensionsConfig loads the extensions configuration from a file.
// If the file doesn't exist, it creates a new empty config.
func LoadExtensionsConfig(filePath string) (*ExtensionsConfig, error) {
	config := NewExtensionsConfig()
	config.filePath = filePath

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty config
			return config, nil
		}
		return nil, fmt.Errorf("failed to read extensions config: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse extensions config: %w", err)
	}

	// Ensure maps are initialized
	if config.Skills == nil {
		config.Skills = make(map[string]SkillState)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPState)
	}

	return config, nil
}

// Save persists the configuration to disk.
func (ec *ExtensionsConfig) Save() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(ec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal extensions config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(ec.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(ec.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write extensions config: %w", err)
	}

	return nil
}

// SaveTo persists the configuration to a specific file path.
func (ec *ExtensionsConfig) SaveTo(filePath string) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(ec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal extensions config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write extensions config: %w", err)
	}

	ec.filePath = filePath
	return nil
}

// IsSkillEnabled returns whether a skill is enabled.
func (ec *ExtensionsConfig) IsSkillEnabled(name string) bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	state, exists := ec.Skills[name]
	if !exists {
		// Default to enabled for new skills
		return true
	}
	return state.Enabled
}

// IsSkillAutoLoad returns whether a skill should be auto-loaded.
func (ec *ExtensionsConfig) IsSkillAutoLoad(name string) bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	state, exists := ec.Skills[name]
	if !exists {
		return false
	}
	return state.AutoLoad
}

// GetSkillState returns the state of a skill.
func (ec *ExtensionsConfig) GetSkillState(name string) (SkillState, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	state, exists := ec.Skills[name]
	return state, exists
}

// SetSkillState updates or creates a skill state.
func (ec *ExtensionsConfig) SetSkillState(name string, state SkillState) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now().UTC()
	if existing, exists := ec.Skills[name]; exists {
		state.InstalledAt = existing.InstalledAt
	} else {
		state.InstalledAt = now
	}
	state.UpdatedAt = now

	ec.Skills[name] = state
	return nil
}

// SetSkillEnabled updates the enabled state of a skill.
func (ec *ExtensionsConfig) SetSkillEnabled(name string, enabled bool) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	state, exists := ec.Skills[name]
	if !exists {
		state = SkillState{
			Enabled:     enabled,
			InstalledAt: time.Now().UTC(),
		}
	} else {
		state.Enabled = enabled
	}
	state.UpdatedAt = time.Now().UTC()

	ec.Skills[name] = state
	return nil
}

// SetSkillAutoLoad updates the auto_load state of a skill.
func (ec *ExtensionsConfig) SetSkillAutoLoad(name string, autoLoad bool) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	state, exists := ec.Skills[name]
	if !exists {
		state = SkillState{
			AutoLoad:    autoLoad,
			InstalledAt: time.Now().UTC(),
		}
	} else {
		state.AutoLoad = autoLoad
	}
	state.UpdatedAt = time.Now().UTC()

	ec.Skills[name] = state
	return nil
}

// RemoveSkill removes a skill from the configuration.
func (ec *ExtensionsConfig) RemoveSkill(name string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	delete(ec.Skills, name)
}

// GetEnabledSkills returns a list of all enabled skill names.
func (ec *ExtensionsConfig) GetEnabledSkills() []string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	var names []string
	for name, state := range ec.Skills {
		if state.Enabled {
			names = append(names, name)
		}
	}
	return names
}

// GetAutoLoadSkills returns a list of skills that should be auto-loaded.
func (ec *ExtensionsConfig) GetAutoLoadSkills() []string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	var names []string
	for name, state := range ec.Skills {
		if state.Enabled && state.AutoLoad {
			names = append(names, name)
		}
	}
	return names
}

// SyncWithSkills updates the config to match the actual skills on disk.
// - Removes config entries for skills that no longer exist
// - Adds default entries for new skills
func (ec *ExtensionsConfig) SyncWithSkills(skills []*SkillV2) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now().UTC()

	// Build set of existing skill names
	skillNames := make(map[string]bool)
	for _, skill := range skills {
		skillNames[skill.Name] = true

		// Add entry for new skills
		if _, exists := ec.Skills[skill.Name]; !exists {
			ec.Skills[skill.Name] = SkillState{
				Enabled:     true,
				AutoLoad:    false,
				Category:    skill.CategoryDir,
				InstalledAt: now,
				UpdatedAt:   now,
				Version:     skill.Version,
			}
		}
	}

	// Remove entries for skills that no longer exist
	for name := range ec.Skills {
		if !skillNames[name] {
			delete(ec.Skills, name)
		}
	}
}

// MCP Server operations

// IsMCPSEnabled returns whether an MCP server is enabled.
func (ec *ExtensionsConfig) IsMCPSEnabled(name string) bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	state, exists := ec.MCPServers[name]
	if !exists {
		return false
	}
	return state.Enabled
}

// GetMCPServer returns the configuration of an MCP server.
func (ec *ExtensionsConfig) GetMCPServer(name string) (MCPState, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	state, exists := ec.MCPServers[name]
	return state, exists
}

// SetMCPServer updates or creates an MCP server configuration.
func (ec *ExtensionsConfig) SetMCPServer(name string, state MCPState) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.MCPServers[name] = state
}

// RemoveMCPServer removes an MCP server configuration.
func (ec *ExtensionsConfig) RemoveMCPServer(name string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	delete(ec.MCPServers, name)
}

// ExtensionsConfigStore provides persistent storage for extensions config.
type ExtensionsConfigStore struct {
	configPath string
	config     *ExtensionsConfig
	mu         sync.RWMutex
}

// NewExtensionsConfigStore creates a new store.
func NewExtensionsConfigStore(configPath string) *ExtensionsConfigStore {
	return &ExtensionsConfigStore{
		configPath: configPath,
	}
}

// Load loads or creates the extensions config.
func (s *ExtensionsConfigStore) Load(ctx context.Context) (*ExtensionsConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config != nil {
		return s.config, nil
	}

	config, err := LoadExtensionsConfig(s.configPath)
	if err != nil {
		return nil, err
	}

	s.config = config
	return config, nil
}

// Get returns the current config (must call Load first).
func (s *ExtensionsConfigStore) Get() *ExtensionsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

// Save persists the current config.
func (s *ExtensionsConfigStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.config == nil {
		return errors.New("config not loaded")
	}

	return s.config.SaveTo(s.configPath)
}

// Reload reloads the config from disk.
func (s *ExtensionsConfigStore) Reload() (*ExtensionsConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config, err := LoadExtensionsConfig(s.configPath)
	if err != nil {
		return nil, err
	}

	s.config = config
	return config, nil
}
