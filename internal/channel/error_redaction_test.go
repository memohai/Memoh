package channel

import (
	"strings"
	"testing"
)

func TestRedactIMErrorText_RedactsFullSecretAndBothHalves(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	RegisterIMErrorSecrets(secret)

	prefixHalf := secret[:len(secret)/2]
	suffixHalf := secret[len(secret)-len(secret)/2:]

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
	if !strings.Contains(got, strings.Repeat("*", len(secret))) {
		t.Fatalf("full secret mask missing: %q", got)
	}
}

func TestRedactIMErrorText_DoesNotRegisterShortHalfFragments(t *testing.T) {
	resetIMErrorSecretsForTest()
	t.Cleanup(resetIMErrorSecretsForTest)

	const secret = "ABCDEFGHIJ"
	RegisterIMErrorSecrets(secret)

	got := RedactIMErrorText("partial=" + secret[:len(secret)/2])
	if got != "partial="+secret[:len(secret)/2] {
		t.Fatalf("short half fragment should not be redacted: %q", got)
	}

	got = RedactIMErrorText("full=" + secret)
	if strings.Contains(got, secret) {
		t.Fatalf("full secret should still be redacted: %q", got)
	}
}
