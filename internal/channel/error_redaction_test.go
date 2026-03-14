package channel

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactIMErrorText_RedactsFullSecretAndBothHalves(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	RegisterIMErrorSecrets(secret)

	runes := []rune(secret)
	half := len(runes) / 2
	prefixHalf := string(runes[:half])
	suffixHalf := string(runes[len(runes)-half:])

	input := strings.Join([]string{
		"full=" + secret,
		"prefix=" + prefixHalf,
		"suffix=" + suffixHalf,
	}, " ")

	got := RedactIMErrorText(input)
	if strings.Contains(got, secret) {
		t.Fatalf("full secret should be redacted: %q", got)
	}
	if strings.Contains(got, prefixHalf) {
		t.Fatalf("prefix half should be redacted: %q", got)
	}
	if strings.Contains(got, suffixHalf) {
		t.Fatalf("suffix half should be redacted: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("*", utf8.RuneCountInString(secret))) {
		t.Fatalf("full secret mask missing: %q", got)
	}
}

func TestRedactIMErrorText_DoesNotRegisterShortHalfFragments(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "ABCDEFGHIJ"
	RegisterIMErrorSecrets(secret)

	runes := []rune(secret)
	shortHalf := string(runes[:len(runes)/2])

	got := RedactIMErrorText("partial=" + shortHalf)
	if got != "partial="+shortHalf {
		t.Fatalf("short half fragment should not be redacted: %q", got)
	}

	got = RedactIMErrorText("full=" + secret)
	if strings.Contains(got, secret) {
		t.Fatalf("full secret should still be redacted: %q", got)
	}
}

func TestRedactIMErrorText_RedactsURLEncodedVariant(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "123456:ABC+DEF/GHI=JKL"
	RegisterIMErrorSecrets(secret)

	encoded := url.QueryEscape(secret)
	if encoded == secret {
		t.Fatal("test secret must differ when URL-encoded")
	}

	got := RedactIMErrorText("url=" + encoded)
	if strings.Contains(got, encoded) {
		t.Fatalf("URL-encoded secret should be redacted: %q", got)
	}
}

func TestUnregisterIMErrorSecrets_RemovesSecret(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "rotating-token-ABCDEFGHIJKLMNO"
	RegisterIMErrorSecrets(secret)

	got := RedactIMErrorText("err: " + secret)
	if strings.Contains(got, secret) {
		t.Fatalf("secret should be redacted before unregister: %q", got)
	}

	UnregisterIMErrorSecrets(secret)

	got = RedactIMErrorText("err: " + secret)
	if !strings.Contains(got, secret) {
		t.Fatalf("secret should no longer be redacted after unregister: %q", got)
	}
}

func TestUnregisterIMErrorSecrets_RefCounted(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "shared-secret-ABCDEFGHIJKLMNO"
	RegisterIMErrorSecrets(secret)
	RegisterIMErrorSecrets(secret) // register twice

	UnregisterIMErrorSecrets(secret) // remove one ref

	got := RedactIMErrorText("err: " + secret)
	if strings.Contains(got, secret) {
		t.Fatalf("secret should still be redacted with remaining ref: %q", got)
	}

	UnregisterIMErrorSecrets(secret) // remove last ref

	got = RedactIMErrorText("err: " + secret)
	if !strings.Contains(got, secret) {
		t.Fatalf("secret should no longer be redacted after all refs removed: %q", got)
	}
}
