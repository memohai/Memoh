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
	SetIMErrorSecrets("test", secret)

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
	SetIMErrorSecrets("test", secret)

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
	SetIMErrorSecrets("test", secret)

	encoded := url.QueryEscape(secret)
	if encoded == secret {
		t.Fatal("test secret must differ when URL-encoded")
	}

	got := RedactIMErrorText("url=" + encoded)
	if strings.Contains(got, encoded) {
		t.Fatalf("URL-encoded secret should be redacted: %q", got)
	}
}

func TestSetIMErrorSecrets_ReplacesOnSameKey(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const oldToken = "old-rotating-token-ABCDEFGHIJKLMNO"
	const newToken = "new-rotating-token-XYZXYZXYZXYZXYZ"

	SetIMErrorSecrets("qq-token:app1", oldToken)

	got := RedactIMErrorText("err: " + oldToken)
	if strings.Contains(got, oldToken) {
		t.Fatalf("old token should be redacted: %q", got)
	}

	// Simulate token rotation: same key, new value
	SetIMErrorSecrets("qq-token:app1", newToken)

	got = RedactIMErrorText("err: " + oldToken)
	if !strings.Contains(got, oldToken) {
		t.Fatalf("old token should no longer be redacted after replacement: %q", got)
	}
	got = RedactIMErrorText("err: " + newToken)
	if strings.Contains(got, newToken) {
		t.Fatalf("new token should be redacted: %q", got)
	}
}

func TestSetIMErrorSecrets_IndependentKeys(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secretA = "secret-AAAAAAAAAAAAAAAA"
	const secretB = "secret-BBBBBBBBBBBBBBBB"

	SetIMErrorSecrets("key-a", secretA)
	SetIMErrorSecrets("key-b", secretB)

	// Replacing key-a should not affect key-b
	SetIMErrorSecrets("key-a", "secret-CCCCCCCCCCCCCCCC")

	got := RedactIMErrorText("err: " + secretB)
	if strings.Contains(got, secretB) {
		t.Fatalf("secretB should still be redacted: %q", got)
	}
}
