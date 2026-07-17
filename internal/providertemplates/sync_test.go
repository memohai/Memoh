package providertemplates

import "testing"

func TestNormalizeDefinitionProducesStableCatalogEntry(t *testing.T) {
	t.Parallel()

	raw := Definition{
		Key:    " OpenAI ",
		Domain: DomainLLM,
		Name:   " OpenAI ",
		Driver: " openai-responses ",
		Source: " openai.yaml ",
		Models: []ModelDefinition{{
			ModelID: " gpt-test ",
			Name:    " GPT Test ",
		}},
	}

	first, firstHash, err := normalizeDefinition(raw, 7)
	if err != nil {
		t.Fatalf("normalizeDefinition() error = %v", err)
	}
	second, secondHash, err := normalizeDefinition(raw, 7)
	if err != nil {
		t.Fatalf("normalizeDefinition() second error = %v", err)
	}

	if firstHash == "" || firstHash != secondHash {
		t.Fatalf("content hash is not stable: %q != %q", firstHash, secondHash)
	}
	if first.Key != "openai" || first.Name != "OpenAI" || first.Driver != "openai-responses" || first.Source != "openai.yaml" {
		t.Fatalf("normalized template = %#v", first)
	}
	if first.SortOrder != 7 {
		t.Fatalf("sort order = %d, want 7", first.SortOrder)
	}
	if len(first.Models) != 1 || first.Models[0].ModelID != "gpt-test" || first.Models[0].Type != "chat" {
		t.Fatalf("normalized models = %#v", first.Models)
	}
	if first.ConfigSchema == nil || first.DefaultConfig == nil || first.Metadata == nil {
		t.Fatal("normalized JSON objects must not be nil")
	}
	if second.Key != first.Key {
		t.Fatalf("second normalized key = %q, want %q", second.Key, first.Key)
	}
}

func TestNormalizeDefinitionRejectsInvalidCatalogEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		definition Definition
	}{
		{
			name: "invalid domain",
			definition: Definition{
				Key: "provider", Domain: Domain("search"), Name: "Provider", Driver: "driver",
			},
		},
		{
			name: "missing driver",
			definition: Definition{
				Key: "provider", Domain: DomainLLM, Name: "Provider",
			},
		},
		{
			name: "empty model id",
			definition: Definition{
				Key: "provider", Domain: DomainLLM, Name: "Provider", Driver: "driver",
				Models: []ModelDefinition{{Name: "Missing ID"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := normalizeDefinition(tt.definition, 0); err == nil {
				t.Fatal("normalizeDefinition() error = nil")
			}
		})
	}
}
