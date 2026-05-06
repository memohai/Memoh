package sso

import "testing"

func TestGenerateLoginCodeHashesOnlyCode(t *testing.T) {
	code, hash, err := GenerateLoginCode()
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if code == "" || hash == "" {
		t.Fatal("expected code and hash")
	}
	if code == hash {
		t.Fatal("hash must not equal raw code")
	}
	if got := HashLoginCode(code); got != hash {
		t.Fatalf("hash mismatch: %s != %s", got, hash)
	}
	if len(hash) != 64 {
		t.Fatalf("sha256 hex length = %d", len(hash))
	}
}
