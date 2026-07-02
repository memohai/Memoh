package main

import (
	"testing"

	"github.com/memohai/memoh/internal/channel/inbound"
	"github.com/memohai/memoh/internal/registry"
)

func TestProviderBootstrapDefinitionsKeepsAllProviderFiles(t *testing.T) {
	defs := []registry.ProviderDefinition{
		{
			Name:       "DeepSeek",
			ClientType: "openai-completions",
		},
		{
			Name:       "OpenAI",
			ClientType: "openai-responses",
		},
		{
			Name:       "OpenAI Speech",
			ClientType: "openai-speech",
		},
		{
			Name:       "Google Transcription",
			ClientType: "google-transcription",
		},
	}

	got := providerBootstrapDefinitions(defs)
	if len(got) != len(defs) {
		t.Fatalf("definition count = %d, want %d", len(got), len(defs))
	}
	for i := range defs {
		if got[i].Name != defs[i].Name {
			t.Fatalf("definition %d = %#v, want %#v", i, got[i], defs[i])
		}
	}
}

func TestNewSessionCreatedByUserIDPrefersCreator(t *testing.T) {
	got := newSessionCreatedByUserID(inbound.NewSessionSpec{
		CreatedByUserID:       "creator-user",
		RuntimeOwnerAccountID: "runtime-owner",
	})
	if got != "creator-user" {
		t.Fatalf("created_by_user_id = %q, want creator-user", got)
	}

	got = newSessionCreatedByUserID(inbound.NewSessionSpec{
		RuntimeOwnerAccountID: "runtime-owner",
	})
	if got != "runtime-owner" {
		t.Fatalf("created_by_user_id fallback = %q, want runtime-owner", got)
	}
}
