package channel

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

var imErrorRedactionRegistry = struct {
	mu      sync.RWMutex
	secrets []string
	index   map[string]struct{}
}{
	index: map[string]struct{}{},
}

// RegisterIMErrorSecrets registers secrets that must be redacted when raw
// errors are sent back to IM users. This is intentionally scoped to IM error
// rendering only: logs and normal outbound messages keep their original text
// so operators can debug issues and user content is not mutated.
func RegisterIMErrorSecrets(secrets ...string) {
	variants := make([]string, 0, len(secrets)*3)
	for _, secret := range secrets {
		variants = append(variants, imErrorRedactionVariants(secret)...)
	}
	if len(variants) == 0 {
		return
	}

	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()

	changed := false
	for _, secret := range variants {
		if _, ok := imErrorRedactionRegistry.index[secret]; ok {
			continue
		}
		imErrorRedactionRegistry.index[secret] = struct{}{}
		imErrorRedactionRegistry.secrets = append(imErrorRedactionRegistry.secrets, secret)
		changed = true
	}
	if !changed {
		return
	}

	sort.Slice(imErrorRedactionRegistry.secrets, func(i, j int) bool {
		left := imErrorRedactionRegistry.secrets[i]
		right := imErrorRedactionRegistry.secrets[j]
		if len(left) == len(right) {
			return left < right
		}
		return len(left) > len(right)
	})
}

// RedactIMErrorText masks registered secrets from error text that is about to
// be rendered back into an IM conversation.
func RedactIMErrorText(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	imErrorRedactionRegistry.mu.RLock()
	secrets := append([]string(nil), imErrorRedactionRegistry.secrets...)
	imErrorRedactionRegistry.mu.RUnlock()

	result := text
	for _, secret := range secrets {
		result = strings.ReplaceAll(result, secret, strings.Repeat("*", utf8.RuneCountInString(secret)))
	}
	return result
}

func imErrorRedactionVariants(secret string) []string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}

	variants := []string{secret}
	runes := []rune(secret)
	half := len(runes) / 2
	if half > 5 {
		variants = append(variants, string(runes[:half]), string(runes[len(runes)-half:]))
	}
	return variants
}

func resetIMErrorSecretsForTest() {
	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()
	imErrorRedactionRegistry.secrets = nil
	imErrorRedactionRegistry.index = map[string]struct{}{}
}

// ResetIMErrorSecretsForTest clears the IM error redaction registry.
// It is intended for tests in other packages that need deterministic state.
func ResetIMErrorSecretsForTest() {
	resetIMErrorSecretsForTest()
}
