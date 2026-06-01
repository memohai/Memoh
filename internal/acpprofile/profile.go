package acpprofile

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	AgentCodexID        = "codex"
	AgentCodexName      = "Codex"
	AgentClaudeCodeID   = "claude-code"
	AgentClaudeCodeName = "Claude Code"

	MetadataKeyACP = "acp"

	setupModeAPIKey = "api_key"
	setupModeOAuth  = "oauth"
	setupModeSelf   = "self"
)

type Profile struct {
	ID                string
	DisplayName       string
	Description       string
	Command           string
	Args              []string
	LocalCommand      string
	LocalArgs         []string
	ManagedFields     []ManagedField
	SupportedBackends []string
	SetupModes        []string
}

type ManagedField struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type PublicProfile struct {
	ID                string         `json:"id"`
	DisplayName       string         `json:"display_name"`
	Description       string         `json:"description,omitempty"`
	ManagedFields     []ManagedField `json:"managed_fields,omitempty"`
	SupportedBackends []string       `json:"supported_backends,omitempty"`
	SetupModes        []string       `json:"setup_modes,omitempty"`
}

type ProfilesResponse struct {
	Items []PublicProfile `json:"items"`
}

type AgentSetup struct {
	AgentID string
	Enabled bool
	Mode    string
	Managed map[string]string
}

// registry holds all known ACP agent profiles keyed by NormalizeAgentID.
// It is initialised via init() in this package; downstream code should only
// access it via Lookup / List / Register so we keep the registration logic
// in a single place.
var registry = map[string]Profile{}

func init() {
	Register(codexProfile())
	Register(claudeCodeProfile())
}

// Register adds (or replaces) a profile in the registry. Intended to be
// called from package init() blocks. Profiles with an empty ID are ignored.
func Register(profile Profile) {
	id := NormalizeAgentID(profile.ID)
	if id == "" {
		return
	}
	profile.ID = id
	registry[id] = profile
}

func codexProfile() Profile {
	return Profile{
		ID:           AgentCodexID,
		DisplayName:  AgentCodexName,
		Description:  "OpenAI Codex ACP adapter",
		Command:      "codex-acp",
		LocalCommand: "npx",
		LocalArgs: []string{
			"-y",
			"@zed-industries/codex-acp@0.15.0",
		},
		ManagedFields: []ManagedField{
			{
				ID:          "api_key",
				Label:       "OpenAI API key",
				Type:        "password",
				Sensitive:   true,
				Placeholder: "sk-...",
				Help:        "Used by API key setup to authenticate Codex.",
			},
			{
				ID:          "base_url",
				Label:       "OpenAI base URL",
				Type:        "url",
				Placeholder: "https://api.openai.com/v1",
				Help:        "Optional Codex provider base URL.",
			},
		},
		SupportedBackends: []string{"local", "container"},
		SetupModes:        []string{setupModeAPIKey, setupModeOAuth, setupModeSelf},
	}
}

func claudeCodeProfile() Profile {
	return Profile{
		ID:           AgentClaudeCodeID,
		DisplayName:  AgentClaudeCodeName,
		Description:  "Claude Code ACP adapter",
		Command:      "claude-agent-acp",
		LocalCommand: "npx",
		LocalArgs: []string{
			"-y",
			"@agentclientprotocol/claude-agent-acp@0.39.0",
		},
		ManagedFields: []ManagedField{
			{
				ID:          "api_key",
				Label:       "Anthropic API key",
				Type:        "password",
				Required:    true,
				Sensitive:   true,
				Placeholder: "sk-ant-...",
				Help:        "Used by API key setup to authenticate Claude Code.",
			},
			{
				ID:          "base_url",
				Label:       "Anthropic base URL",
				Type:        "url",
				Placeholder: "https://api.anthropic.com",
				Help:        "Optional Claude Code API endpoint override.",
			},
			{
				ID:          "oauth_token",
				Label:       "Claude Code OAuth token",
				Type:        "password",
				Required:    true,
				Sensitive:   true,
				Placeholder: "Token from claude setup-token",
				Help:        "Used by OAuth setup to authenticate Claude Code.",
			},
		},
		SupportedBackends: []string{"local", "container"},
		SetupModes:        []string{setupModeAPIKey, setupModeOAuth, setupModeSelf},
	}
}

// List returns all registered public profiles, sorted by ID for stable
// API responses.
func List() []PublicProfile {
	out := make([]PublicProfile, 0, len(registry))
	for _, profile := range registry {
		out = append(out, profile.Public())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Lookup returns the registered profile for id (case-insensitive).
func Lookup(id string) (Profile, bool) {
	id = NormalizeAgentID(id)
	profile, ok := registry[id]
	return profile, ok
}

func (p Profile) Public() PublicProfile {
	return PublicProfile{
		ID:                p.ID,
		DisplayName:       p.DisplayName,
		Description:       p.Description,
		ManagedFields:     append([]ManagedField(nil), p.ManagedFields...),
		SupportedBackends: append([]string(nil), p.SupportedBackends...),
		SetupModes:        append([]string(nil), p.SetupModes...),
	}
}

func MetadataAgentEnabled(metadata map[string]any, agentID string) bool {
	setup := ParseAgentSetup(metadata, agentID)
	return setup.Enabled
}

func MetadataAgentEnabledRaw(raw []byte, agentID string) bool {
	if len(raw) == 0 {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return false
	}
	return MetadataAgentEnabled(metadata, agentID)
}

func ParseAgentSetup(metadata map[string]any, agentID string) AgentSetup {
	agentID = NormalizeAgentID(agentID)
	setup := AgentSetup{
		AgentID: agentID,
		Mode:    setupModeAPIKey,
		Managed: map[string]string{},
	}
	if agentID == "" {
		return setup
	}
	acpConfig, ok := metadataRecord(metadata[MetadataKeyACP])
	if !ok {
		return setup
	}

	if agents, ok := metadataRecord(acpConfig["agents"]); ok {
		if agentConfig, ok := metadataRecord(agents[agentID]); ok {
			if enabled, ok := metadataBool(agentConfig["enabled"]); ok {
				setup.Enabled = enabled
			}
			if mode, ok := agentConfig["setup_mode"].(string); ok && strings.TrimSpace(mode) != "" {
				setup.Mode = strings.TrimSpace(strings.ToLower(mode))
			}
			if managed, ok := metadataRecord(agentConfig["managed"]); ok {
				for key, value := range managed {
					if s, ok := value.(string); ok {
						setup.Managed[key] = s
					}
				}
			}
			setup.Mode = normalizeSetupMode(setup.Mode)
			return setup
		}
	}

	return setup
}

func normalizeSetupMode(mode string) string {
	mode = NormalizeAgentID(mode)
	switch mode {
	case setupModeOAuth, setupModeSelf:
		return mode
	case setupModeAPIKey, "":
		return setupModeAPIKey
	default:
		return mode
	}
}

func NormalizeAgentID(agentID string) string {
	return strings.ToLower(strings.TrimSpace(agentID))
}

func ScrubMetadataForResponse(metadata map[string]any) map[string]any {
	cloned := cloneMap(metadata)
	acpConfig, ok := metadataRecord(cloned[MetadataKeyACP])
	if !ok {
		return cloned
	}
	agents, ok := metadataRecord(acpConfig["agents"])
	if !ok {
		return cloned
	}
	for rawAgentID, rawAgent := range agents {
		agentConfig, ok := metadataRecord(rawAgent)
		if !ok {
			continue
		}
		managed, ok := metadataRecord(agentConfig["managed"])
		if !ok {
			continue
		}
		profile, _ := Lookup(rawAgentID)
		sensitive := sensitiveFieldSet(profile)
		for key, value := range managed {
			if !sensitive[key] && !looksSensitiveKey(key) {
				continue
			}
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				managed[key] = maskSecret(s)
			}
		}
	}
	return cloned
}

func MergeSensitiveFieldsForUpdate(existing, incoming map[string]any) map[string]any {
	merged := cloneMap(incoming)
	existingACP, okExistingACP := metadataRecord(existing[MetadataKeyACP])
	incomingACP, okIncomingACP := metadataRecord(merged[MetadataKeyACP])
	if !okExistingACP || !okIncomingACP {
		return merged
	}
	existingAgents, okExistingAgents := metadataRecord(existingACP["agents"])
	incomingAgents, okIncomingAgents := metadataRecord(incomingACP["agents"])
	if !okExistingAgents || !okIncomingAgents {
		return merged
	}

	for rawAgentID, rawIncomingAgent := range incomingAgents {
		incomingAgent, ok := metadataRecord(rawIncomingAgent)
		if !ok {
			continue
		}
		incomingManaged, ok := metadataRecord(incomingAgent["managed"])
		if !ok {
			continue
		}
		existingAgent, ok := metadataRecord(existingAgents[rawAgentID])
		if !ok {
			continue
		}
		existingManaged, ok := metadataRecord(existingAgent["managed"])
		if !ok {
			continue
		}
		profile, _ := Lookup(rawAgentID)
		sensitive := sensitiveFieldSet(profile)
		for key := range existingManaged {
			if !sensitive[key] && !looksSensitiveKey(key) {
				continue
			}
			value, exists := incomingManaged[key]
			switch {
			case !exists:
				incomingManaged[key] = existingManaged[key]
			case value == nil:
				delete(incomingManaged, key)
			case isMaskedSecretValue(value):
				incomingManaged[key] = existingManaged[key]
			case isEmptyString(value):
				incomingManaged[key] = existingManaged[key]
			}
		}
	}
	return merged
}

func sensitiveFieldSet(profile Profile) map[string]bool {
	out := map[string]bool{}
	for _, field := range profile.ManagedFields {
		if field.Sensitive || field.Type == "password" {
			out[field.ID] = true
		}
	}
	return out
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "sk-") && len(value) > 7 {
		return "sk-..." + value[len(value)-4:]
	}
	if len(value) > 4 {
		return "***" + value[len(value)-4:]
	}
	return "***"
}

func isMaskedSecretValue(value any) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	if s == "***" {
		return true
	}
	if strings.HasPrefix(s, "sk-...") {
		return len([]rune(strings.TrimPrefix(s, "sk-..."))) == 4
	}
	if strings.HasPrefix(s, "***") {
		return len([]rune(strings.TrimPrefix(s, "***"))) == 4
	}
	return false
}

func isEmptyString(value any) bool {
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) == ""
}

func looksSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "key") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password")
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for key, value := range in {
			out[key] = value
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil || out == nil {
		out = map[string]any{}
	}
	return out
}

func metadataRecord(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	default:
		return nil, false
	}
}

func metadataBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on", "enabled":
			return true, true
		case "false", "0", "no", "off", "disabled":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}
