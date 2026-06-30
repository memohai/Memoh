package containerfs

import "testing"

func TestParseRoutingKeyRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		"../etc/passwd",
		"/absolute/key",
		"bot-1/../../escape",
	} {
		if _, _, err := parseRoutingKey(key); err == nil {
			t.Errorf("parseRoutingKey(%q) expected error", key)
		}
	}
}
