package channel

import (
	"testing"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/db"
)

func TestDecodeConfigMap(t *testing.T) {
	t.Parallel()

	cfg, err := DecodeConfigMap([]byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg["a"] == nil {
		t.Fatalf("expected key in map")
	}
	cfg, err = DecodeConfigMap([]byte(`null`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg == nil || len(cfg) != 0 {
		t.Fatalf("expected empty map")
	}
}

func TestReadString(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"bot_token": 123,
	}
	got := ReadString(raw, "bot_token")
	if got != "123" {
		t.Fatalf("unexpected value: %s", got)
	}
}

func TestParseUUID(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()
	if _, err := db.ParseUUID(id); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := db.ParseUUID("invalid"); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestBindingCriteriaFromIdentity(t *testing.T) {
	t.Parallel()

	criteria := BindingCriteriaFromIdentity(Identity{
		SubjectID:  "u1",
		Attributes: map[string]string{"username": "alice"},
	})
	if criteria.SubjectID != "u1" {
		t.Fatalf("unexpected subject id: %s", criteria.SubjectID)
	}
	if criteria.Attribute("username") != "alice" {
		t.Fatalf("unexpected username: %s", criteria.Attribute("username"))
	}
}

func TestNormalizeChannelConfigStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default pending", input: "", want: "pending"},
		{name: "pending passthrough", input: "pending", want: "pending"},
		{name: "verified passthrough", input: "verified", want: "verified"},
		{name: "disabled passthrough", input: "disabled", want: "disabled"},
		{name: "active alias", input: "active", want: "verified"},
		{name: "inactive alias", input: "inactive", want: "disabled"},
		{name: "unknown status", input: "paused", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeConfigStatus(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected status: got %s, want %s", got, tt.want)
			}
		})
	}
}
