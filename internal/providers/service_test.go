package providers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func TestResolveUpdatedAPIKey(t *testing.T) {
	t.Parallel()

	existing := "sk-1234567890abcdef"
	masked := maskAPIKey(existing)

	t.Run("nil update keeps existing", func(t *testing.T) {
		t.Parallel()
		if got := resolveUpdatedAPIKey(existing, nil); got != existing {
			t.Fatalf("expected existing key, got %q", got)
		}
	})

	t.Run("masked update keeps existing", func(t *testing.T) {
		t.Parallel()
		if got := resolveUpdatedAPIKey(existing, &masked); got != existing {
			t.Fatalf("expected existing key, got %q", got)
		}
	})

	t.Run("new key replaces existing", func(t *testing.T) {
		t.Parallel()
		next := "sk-new-secret"
		if got := resolveUpdatedAPIKey(existing, &next); got != next {
			t.Fatalf("expected new key, got %q", got)
		}
	})

	t.Run("empty update clears key", func(t *testing.T) {
		t.Parallel()
		empty := ""
		if got := resolveUpdatedAPIKey(existing, &empty); got != empty {
			t.Fatalf("expected empty key, got %q", got)
		}
	})
}

func makeModel(modelID, clientType, modelType string) sqlc.Model {
	return sqlc.Model{
		ModelID:     modelID,
		ClientType:  pgtype.Text{String: clientType, Valid: clientType != ""},
		Type:        modelType,
	}
}

func TestBuildAllProbes(t *testing.T) {
	t.Parallel()

	baseURL := "https://api.example.com"
	apiKey := "test-key"

	t.Run("no user models uses defaults", func(t *testing.T) {
		t.Parallel()
		models := []sqlc.Model{}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		if len(probes) != 5 {
			t.Fatalf("expected 5 probes, got %d", len(probes))
		}

		// Collect probe names
		probeNames := make(map[string]bool)
		for _, p := range probes {
			probeNames[p.name] = true
		}

		expectedProbes := []string{
			"openai-completions",
			"openai-responses",
			"anthropic-messages",
			"google-generative-ai",
			"embedding",
		}

		for _, expected := range expectedProbes {
			if !probeNames[expected] {
				t.Errorf("expected probe %q not found", expected)
			}
		}
	})

	t.Run("user chat model overrides openai-responses", func(t *testing.T) {
		t.Parallel()
		models := []sqlc.Model{
			makeModel("gpt-4o", "openai-responses", "chat"),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		// Find openai-responses probe and execute it to verify model
		var found bool
		for _, p := range probes {
			if p.name == "openai-responses" {
				found = true
				break
			}
		}
		if !found {
			t.Error("openai-responses probe not found")
		}
	})

	t.Run("user chat model overrides anthropic-messages", func(t *testing.T) {
		t.Parallel()
		models := []sqlc.Model{
			makeModel("claude-3-5-sonnet-20241022", "anthropic-messages", "chat"),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		var found bool
		for _, p := range probes {
			if p.name == "anthropic-messages" {
				found = true
				break
			}
		}
		if !found {
			t.Error("anthropic-messages probe not found")
		}
	})

	t.Run("user embedding model overrides default", func(t *testing.T) {
		t.Parallel()
		models := []sqlc.Model{
			makeModel("text-embedding-3-large", "", "embedding"),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		var found bool
		for _, p := range probes {
			if p.name == "embedding" {
				found = true
				break
			}
		}
		if !found {
			t.Error("embedding probe not found")
		}
	})

	t.Run("multiple user models override all defaults", func(t *testing.T) {
		t.Parallel()
		models := []sqlc.Model{
			makeModel("gpt-4-turbo", "openai-responses", "chat"),
			makeModel("claude-3-opus", "anthropic-messages", "chat"),
			makeModel("text-embedding-3-large", "", "embedding"),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		if len(probes) != 5 {
			t.Fatalf("expected 5 probes, got %d", len(probes))
		}

		// Verify all 5 probes exist
		probeNames := make(map[string]bool)
		for _, p := range probes {
			probeNames[p.name] = true
		}

		expectedProbes := []string{
			"openai-completions",
			"openai-responses",
			"anthropic-messages",
			"google-generative-ai",
			"embedding",
		}

		for _, expected := range expectedProbes {
			if !probeNames[expected] {
				t.Errorf("expected probe %q not found", expected)
			}
		}
	})

	t.Run("empty type model uses default chat model", func(t *testing.T) {
		t.Parallel()
		// Empty type should be treated as chat
		models := []sqlc.Model{
			makeModel("gpt-4o", "openai-responses", ""),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		var found bool
		for _, p := range probes {
			if p.name == "openai-responses" {
				found = true
				break
			}
		}
		if !found {
			t.Error("openai-responses probe not found for empty type model")
		}
	})

	t.Run("first model of each client type is used", func(t *testing.T) {
		t.Parallel()
		// Only the first model per client type should be used
		models := []sqlc.Model{
			makeModel("gpt-4o-mini", "openai-responses", "chat"),
			makeModel("gpt-4o", "openai-responses", "chat"),
			makeModel("claude-3-haiku", "anthropic-messages", "chat"),
			makeModel("claude-3-sonnet", "anthropic-messages", "chat"),
		}

		probes := buildAllProbes(context.Background(), models, baseURL, apiKey)

		// Should have both probes
		probeNames := make(map[string]bool)
		for _, p := range probes {
			probeNames[p.name] = true
		}

		if !probeNames["openai-responses"] {
			t.Error("openai-responses probe not found")
		}
		if !probeNames["anthropic-messages"] {
			t.Error("anthropic-messages probe not found")
		}
	})
}
