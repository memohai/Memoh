package main

import (
	"testing"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/registry"
)

func TestProviderBootstrapDefinitionsSkipsLLMTemplates(t *testing.T) {
	defs := []registry.ProviderDefinition{
		{
			Name:       "DeepSeek",
			ClientType: string(models.ClientTypeOpenAICompletions),
		},
		{
			Name:       "OpenAI Codex",
			ClientType: string(models.ClientTypeOpenAICodex),
		},
		{
			Name:       "OpenAI Speech",
			ClientType: string(models.ClientTypeOpenAISpeech),
		},
		{
			Name:       "Google Transcription",
			ClientType: string(models.ClientTypeGoogleTranscription),
		},
	}

	got := providerBootstrapDefinitions(defs)
	if len(got) != 2 {
		t.Fatalf("definition count = %d, want 2", len(got))
	}
	if got[0].Name != "OpenAI Speech" || got[1].Name != "Google Transcription" {
		t.Fatalf("definitions = %#v, want only non-LLM provider templates", got)
	}
}
