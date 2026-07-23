package redact

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestText_RedactsFullSecretAndBothHalves(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	const secret = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	SetSecrets("test", secret)

	runes := []rune(secret)
	half := len(runes) / 2
	prefixHalf := string(runes[:half])
	suffixHalf := string(runes[len(runes)-half:])

	input := strings.Join([]string{
		"full=" + secret,
		"prefix=" + prefixHalf,
		"suffix=" + suffixHalf,
	}, " ")

	got := Text(input)
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

func TestText_DoesNotRegisterShortHalfFragments(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	const secret = "ABCDEFGHIJ"
	SetSecrets("test", secret)

	runes := []rune(secret)
	shortHalf := string(runes[:len(runes)/2])

	got := Text("partial=" + shortHalf)
	if got != "partial="+shortHalf {
		t.Fatalf("short half fragment should not be redacted: %q", got)
	}

	got = Text("full=" + secret)
	if strings.Contains(got, secret) {
		t.Fatalf("full secret should still be redacted: %q", got)
	}
}

func TestText_RedactsURLEncodedVariant(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	const secret = "123456:ABC+DEF/GHI=JKL"
	SetSecrets("test", secret)

	encoded := url.QueryEscape(secret)
	if encoded == secret {
		t.Fatal("test secret must differ when URL-encoded")
	}

	got := Text("url=" + encoded)
	if strings.Contains(got, encoded) {
		t.Fatalf("URL-encoded secret should be redacted: %q", got)
	}
}

func TestSetSecrets_ReplacesOnSameKey(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	const oldToken = "old-rotating-token-ABCDEFGHIJKLMNO"
	const newToken = "new-rotating-token-XYZXYZXYZXYZXYZ"

	SetSecrets("qq-token:app1", oldToken)

	got := Text("err: " + oldToken)
	if strings.Contains(got, oldToken) {
		t.Fatalf("old token should be redacted: %q", got)
	}

	// Simulate token rotation: same key, new value
	SetSecrets("qq-token:app1", newToken)

	got = Text("err: " + oldToken)
	if !strings.Contains(got, oldToken) {
		t.Fatalf("old token should no longer be redacted after replacement: %q", got)
	}
	got = Text("err: " + newToken)
	if strings.Contains(got, newToken) {
		t.Fatalf("new token should be redacted: %q", got)
	}
}

func TestSetSecrets_IndependentKeys(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	const secretA = "secret-AAAAAAAAAAAAAAAA"
	const secretB = "secret-BBBBBBBBBBBBBBBB"

	SetSecrets("key-a", secretA)
	SetSecrets("key-b", secretB)

	// Replacing key-a should not affect key-b
	SetSecrets("key-a", "secret-CCCCCCCCCCCCCCCC")

	got := Text("err: " + secretB)
	if strings.Contains(got, secretB) {
		t.Fatalf("secretB should still be redacted: %q", got)
	}
}
