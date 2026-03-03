package browser

import "testing"

func TestValidateURL(t *testing.T) {
	t.Run("reject localhost", func(t *testing.T) {
		if err := validateURL("http://localhost/test"); err == nil {
			t.Fatalf("expected localhost to be blocked")
		}
	})

	t.Run("reject loopback ip", func(t *testing.T) {
		if err := validateURL("http://127.0.0.1/test"); err == nil {
			t.Fatalf("expected loopback to be blocked")
		}
	})

	t.Run("reject private ip", func(t *testing.T) {
		if err := validateURL("http://10.0.0.2/test"); err == nil {
			t.Fatalf("expected private ip to be blocked")
		}
	})

	t.Run("allow public https", func(t *testing.T) {
		if err := validateURL("https://example.com"); err != nil {
			t.Fatalf("expected public host allowed, got %v", err)
		}
	})
}
