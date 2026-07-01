package email

import "testing"

func TestSanitizeProviderConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		config   map[string]any
		want     map[string]any
	}{
		{
			name:     "removes legacy gmail oauth secrets",
			provider: "gmail",
			config: map[string]any{
				"email_address": "person@gmail.com",
				"client_id":     "legacy-client",
				"client_secret": "legacy-secret",
			},
			want: map[string]any{
				"email_address": "person@gmail.com",
			},
		},
		{
			name:     "keeps oauth-shaped fields for other providers",
			provider: "smtp",
			config: map[string]any{
				"client_id":     "smtp-client",
				"client_secret": "smtp-secret",
			},
			want: map[string]any{
				"client_id":     "smtp-client",
				"client_secret": "smtp-secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean := sanitizeProviderConfig(tt.provider, tt.config)
			for key, value := range tt.want {
				if clean[key] != value {
					t.Fatalf("%s was not preserved: %#v", key, clean)
				}
			}
			for key := range clean {
				if _, ok := tt.want[key]; !ok {
					t.Fatalf("unexpected key %s in sanitized config: %#v", key, clean)
				}
			}
		})
	}
}
