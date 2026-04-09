package providers

import "testing"

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("short key is fully masked", func(t *testing.T) {
		t.Parallel()
		if got := maskAPIKey("sk-12"); got != "*****" {
			t.Fatalf("expected fully masked, got %q", got)
		}
	})

	t.Run("long key preserves prefix", func(t *testing.T) {
		t.Parallel()
		key := "sk-1234567890abcdef"
		masked := maskAPIKey(key)
		if masked == key {
			t.Fatal("masked key should differ from original")
		}
		if len(masked) != len(key) {
			t.Fatalf("masked length %d != original length %d", len(masked), len(key))
		}
		if masked[:8] != key[:8] {
			t.Fatalf("prefix mismatch: %q vs %q", masked[:8], key[:8])
		}
	})

	t.Run("empty key returns empty", func(t *testing.T) {
		t.Parallel()
		if got := maskAPIKey(""); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})
}
