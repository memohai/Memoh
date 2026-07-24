package redact

import (
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

var registry = struct {
	mu     sync.RWMutex
	groups map[string][]string // key → variants
	cache  []string            // deduplicated, sorted longest-first
}{
	groups: map[string][]string{},
}

// SetSecrets associates a set of secrets with the given key.
// Calling again with the same key replaces the previous secrets,
// so rotating credentials (e.g. access tokens) are handled naturally
// without explicit unregistration.
//
// The key should identify the credential scope, e.g. "qq-token:<appID>"
// or "telegram:<configID>". For multiple instances of the same adapter,
// include a stable instance identifier in the key.
//
// Callers decide where redaction is appropriate. Logs and normal outbound
// messages should retain their original text unless their own contract says
// otherwise.
func SetSecrets(key string, secrets ...string) {
	var variants []string
	for _, secret := range secrets {
		variants = append(variants, variantsFor(secret)...)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if len(variants) == 0 {
		if _, exists := registry.groups[key]; !exists {
			return
		}
		delete(registry.groups, key)
	} else {
		if slices.Equal(registry.groups[key], variants) {
			return
		}
		registry.groups[key] = variants
	}
	registry.cache = rebuildSecretCache(registry.groups)
}

// Text masks registered secrets in text selected by the caller.
func Text(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	registry.mu.RLock()
	cache := registry.cache
	registry.mu.RUnlock()

	result := text
	for _, secret := range cache {
		result = strings.ReplaceAll(result, secret, strings.Repeat("*", utf8.RuneCountInString(secret)))
	}
	return result
}

func variantsFor(secret string) []string {
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

func resetForTest() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.groups = map[string][]string{}
	registry.cache = nil
}

// ResetForTest clears the error redaction registry.
// It is intended for tests in other packages that need deterministic state.
func ResetForTest() {
	resetForTest()
}
