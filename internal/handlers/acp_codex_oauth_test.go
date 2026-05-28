package handlers

import (
	"strings"
	"testing"
)

func TestGenerateACPCodexOAuthStateUsesCallbackPrefix(t *testing.T) {
	state, err := generateACPCodexOAuthState()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(state, acpCodexOAuthStatePrefix) {
		t.Fatalf("state = %q, want prefix %q", state, acpCodexOAuthStatePrefix)
	}
	handler := &ACPCodexOAuthHandler{}
	if !handler.HandlesCallbackState(state) {
		t.Fatalf("handler did not recognize ACP Codex OAuth state")
	}
	if handler.HandlesCallbackState("provider-state") {
		t.Fatalf("handler should not recognize normal provider OAuth state")
	}
}

func TestParseCodexOAuthAuthRequiresDistinctIDToken(t *testing.T) {
	valid := parseCodexOAuthAuth(`{
  "auth_mode": "chatgpt",
  "tokens": {
    "id_token": "id.jwt.token",
    "access_token": "access.jwt.token",
    "refresh_token": "refresh-token",
    "account_id": "account-123"
  }
}`)
	if !valid.Valid {
		t.Fatalf("valid auth.json was not accepted")
	}
	if valid.AccountID != "account-123" {
		t.Fatalf("account id = %q, want account-123", valid.AccountID)
	}

	legacyBad := parseCodexOAuthAuth(`{
  "auth_mode": "chatgpt",
  "tokens": {
    "id_token": "same.jwt.token",
    "access_token": "same.jwt.token",
    "refresh_token": "refresh-token",
    "account_id": "account-123"
  }
}`)
	if legacyBad.Valid {
		t.Fatalf("legacy auth.json with id_token equal to access_token should not be accepted")
	}
}
