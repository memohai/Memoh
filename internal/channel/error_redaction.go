package channel

import (
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

var imErrorRedactionRegistry = struct {
	mu     sync.RWMutex
	groups map[string][]string // key → variants
	cache  []string            // deduplicated, sorted longest-first
}{
	groups: map[string][]string{},
}

// SetIMErrorSecrets associates a set of secrets with the given key.
// Calling again with the same key replaces the previous secrets,
// so rotating credentials (e.g. access tokens) are handled naturally
// without explicit unregistration.
//
// The key should identify the credential scope, e.g. "qq-token:<appID>"
// or "telegram:<configID>". For multiple instances of the same adapter,
// include a stable instance identifier in the key.
//
// This is intentionally scoped to IM error rendering only: logs and
// normal outbound messages keep their original text so operators can
// debug issues and user content is not mutated.
func SetIMErrorSecrets(key string, secrets ...string) {
	var variants []string
	for _, secret := range secrets {
		variants = append(variants, imErrorRedactionVariants(secret)...)
	}

	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()

	if len(variants) == 0 {
		if _, exists := imErrorRedactionRegistry.groups[key]; !exists {
			return
		}
		delete(imErrorRedactionRegistry.groups, key)
	} else {
		if slices.Equal(imErrorRedactionRegistry.groups[key], variants) {
			return
		}
		imErrorRedactionRegistry.groups[key] = variants
	}
	imErrorRedactionRegistry.cache = rebuildSecretCache(imErrorRedactionRegistry.groups)
}

// RedactIMErrorText masks registered secrets from error text that is about to
// be rendered back into an IM conversation.
func RedactIMErrorText(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	imErrorRedactionRegistry.mu.RLock()
	cache := imErrorRedactionRegistry.cache
	imErrorRedactionRegistry.mu.RUnlock()

	result := text
	for _, secret := range cache {
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
	if encoded := url.QueryEscape(secret); encoded != secret {
		variants = append(variants, encoded)
	}
	return variants
}

func rebuildSecretCache(groups map[string][]string) []string {
	seen := make(map[string]struct{})
	var all []string
	for _, variants := range groups {
		for _, v := range variants {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			all = append(all, v)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		li := utf8.RuneCountInString(all[i])
		lj := utf8.RuneCountInString(all[j])
		if li == lj {
			return all[i] < all[j]
		}
		return li > lj
	})
	return all
}

func resetIMErrorSecretsForTest() {
	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()
	imErrorRedactionRegistry.groups = map[string][]string{}
	imErrorRedactionRegistry.cache = nil
}

// ResetIMErrorSecretsForTest clears the IM error redaction registry.
// It is intended for tests in other packages that need deterministic state.
func ResetIMErrorSecretsForTest() {
	resetIMErrorSecretsForTest()
}
