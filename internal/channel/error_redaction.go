package channel

import (
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

var imErrorRedactionRegistry = struct {
	mu      sync.RWMutex
	secrets []string
	refs    map[string]int
}{
	refs: map[string]int{},
}

// RegisterIMErrorSecrets registers secrets that must be redacted when raw
// errors are sent back to IM users. This is intentionally scoped to IM error
// rendering only: logs and normal outbound messages keep their original text
// so operators can debug issues and user content is not mutated.
func RegisterIMErrorSecrets(secrets ...string) {
	variants := make([]string, 0, len(secrets)*4)
	for _, secret := range secrets {
		variants = append(variants, imErrorRedactionVariants(secret)...)
	}
	if len(variants) == 0 {
		return
	}

	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()

	changed := false
	for _, v := range variants {
		imErrorRedactionRegistry.refs[v]++
		if imErrorRedactionRegistry.refs[v] == 1 {
			imErrorRedactionRegistry.secrets = append(imErrorRedactionRegistry.secrets, v)
			changed = true
		}
	}
	if changed {
		sortSecrets(imErrorRedactionRegistry.secrets)
	}
}

// UnregisterIMErrorSecrets removes previously registered secrets from the
// redaction registry. Reference-counted: a secret is only removed when every
// corresponding RegisterIMErrorSecrets call has been balanced.
func UnregisterIMErrorSecrets(secrets ...string) {
	variants := make([]string, 0, len(secrets)*4)
	for _, secret := range secrets {
		variants = append(variants, imErrorRedactionVariants(secret)...)
	}
	if len(variants) == 0 {
		return
	}

	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()

	changed := false
	for _, v := range variants {
		if imErrorRedactionRegistry.refs[v] <= 0 {
			continue
		}
		imErrorRedactionRegistry.refs[v]--
		if imErrorRedactionRegistry.refs[v] == 0 {
			delete(imErrorRedactionRegistry.refs, v)
			changed = true
		}
	}
	if !changed {
		return
	}

	rebuilt := make([]string, 0, len(imErrorRedactionRegistry.refs))
	for s := range imErrorRedactionRegistry.refs {
		rebuilt = append(rebuilt, s)
	}
	sortSecrets(rebuilt)
	imErrorRedactionRegistry.secrets = rebuilt
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
	if encoded := url.QueryEscape(secret); encoded != secret {
		variants = append(variants, encoded)
	}
	return variants
}

func sortSecrets(s []string) {
	sort.Slice(s, func(i, j int) bool {
		li := utf8.RuneCountInString(s[i])
		lj := utf8.RuneCountInString(s[j])
		if li == lj {
			return s[i] < s[j]
		}
		return li > lj
	})
}

func resetIMErrorSecretsForTest() {
	imErrorRedactionRegistry.mu.Lock()
	defer imErrorRedactionRegistry.mu.Unlock()
	imErrorRedactionRegistry.secrets = nil
	imErrorRedactionRegistry.refs = map[string]int{}
}

// ResetIMErrorSecretsForTest clears the IM error redaction registry.
// It is intended for tests in other packages that need deterministic state.
func ResetIMErrorSecretsForTest() {
	resetIMErrorSecretsForTest()
}
