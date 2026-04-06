package models

import "encoding/json"

// providerConfigString extracts a string value from a provider's JSONB config bytes.
func providerConfigString(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return v
}
