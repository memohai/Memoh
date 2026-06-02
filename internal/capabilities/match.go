// Package capabilities discovers model reasoning capabilities (thinking mode and
// effort levels) by matching upstream model identifiers against the LiteLLM
// model registry. It is a "trust + fill" layer: it only supplies information the
// upstream provider did not, and never overrides explicit upstream values.
//
// The hardest part is matching: the same underlying model is named differently
// across providers and gateways (provider/region prefixes, separators, dates,
// version formatting, marketing suffixes). The matcher normalizes both sides to
// a token set + version signature and matches with a version veto, so that e.g.
// "openrouter/anthropic/claude-4.8-opus" and "anthropic.claude-opus-4-8" resolve
// to the same registry entry while "claude-opus-4-8" never matches "claude-opus-4-6".
package capabilities

import (
	"regexp"
	"strings"
)

// knownPrefixSegments are vendor / gateway / region tokens that may be prepended
// to a base model name. They are stripped only when they appear as leading
// segments, so version numbers (which never match these words) are preserved.
var knownPrefixSegments = map[string]struct{}{
	"openrouter": {}, "anthropic": {}, "openai": {}, "google": {}, "gemini": {},
	"azure": {}, "azure_ai": {}, "vertex": {}, "vertex_ai": {}, "bedrock": {},
	"deepseek": {}, "fireworks": {}, "fireworks_ai": {}, "groq": {}, "mistral": {},
	"cohere": {}, "xai": {}, "together": {}, "together_ai": {}, "togethercomputer": {},
	"perplexity": {}, "deepinfra": {}, "nscale": {}, "nebius": {}, "hyperbolic": {},
	"lambda": {}, "lambda_ai": {}, "crusoe": {}, "gradient_ai": {}, "watsonx": {},
	"databricks": {}, "github_copilot": {}, "copilot": {}, "github": {}, "moonshot": {},
	"meta": {}, "meta_llama": {}, "qwen": {}, "alibaba": {}, "dashscope": {},
	// region tokens used in dotted bedrock/vertex keys
	"us": {}, "eu": {}, "au": {}, "jp": {}, "apac": {}, "global": {}, "ap": {},
	"sa": {}, "ca": {}, "me": {}, "emea": {},
}

// marketingSuffixTokens are trailing decorations that don't change the model's
// reasoning capability shape and should not block a match.
var marketingSuffixTokens = map[string]struct{}{
	"thinking": {}, "fast": {}, "latest": {}, "preview": {}, "exp": {},
	"experimental": {}, "beta": {}, "online": {}, "nonthinking": {},
}

var (
	// trailing date: -20250514 or -2025-05-14.
	reDateCompact = regexp.MustCompile(`-\d{8}$`)
	reDateDashed  = regexp.MustCompile(`-20\d{2}-\d{2}-\d{2}$`)
	// trailing vendor decorations, stripped while original delimiters are intact:
	//   @default (vertex), -v1:0 / -v2 (bedrock), :0 (revision).
	reAtSuffix   = regexp.MustCompile(`@[a-z0-9._-]+$`)
	reBedrockVer = regexp.MustCompile(`[-_]v\d+(:\d+)?$`)
	reColonVer   = regexp.MustCompile(`:\d+$`)
	// a token carries version information if it contains a digit.
	reHasDigit = regexp.MustCompile(`\d`)
)

// normalized is the canonical signature of a model name used for matching.
type normalized struct {
	canonical string              // hyphen-joined non-marketing tokens
	tokens    map[string]struct{} // full token set (incl. version tokens)
	versions  map[string]struct{} // tokens that carry version info (digit-bearing)
}

// normalize reduces a raw model identifier to its matching signature.
func normalize(raw string) normalized {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return normalized{tokens: map[string]struct{}{}, versions: map[string]struct{}{}}
	}

	// 1. Drop slash-delimited gateway/region prefixes ("openrouter/anthropic/x",
	//    "bedrock/us-east-1/x") by keeping the final path segment.
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}

	// 2. Strip leading dotted vendor/region segments ("us.anthropic.claude-..."),
	//    stopping at the first segment that is not a known prefix word. This keeps
	//    version dots like "claude-opus-4.8" intact.
	if strings.Contains(s, ".") {
		segs := strings.Split(s, ".")
		start := 0
		for start < len(segs)-1 {
			if _, ok := knownPrefixSegments[segs[start]]; ok {
				start++
				continue
			}
			break
		}
		s = strings.Join(segs[start:], ".")
	}

	// 3. Strip trailing vendor decorations while the original delimiters are
	//    still present (so "-v1:0" / "@default" / ":0" are recognized before ':'
	//    and '@' get folded into '-').
	for {
		before := s
		s = reAtSuffix.ReplaceAllString(s, "")
		s = reBedrockVer.ReplaceAllString(s, "")
		s = reColonVer.ReplaceAllString(s, "")
		if s == before {
			break
		}
	}

	// 4. Unify separators to '-'.
	s = strings.NewReplacer(".", "-", "_", "-", " ", "-", ":", "-", "@", "-").Replace(s)
	s = strings.Trim(s, "-")

	// 5. Strip trailing date suffixes (best-effort, repeated).
	for {
		before := s
		s = reDateCompact.ReplaceAllString(s, "")
		s = reDateDashed.ReplaceAllString(s, "")
		s = strings.Trim(s, "-")
		if s == before {
			break
		}
	}

	// 6. Re-strip leading vendor prefixes that only surfaced after separator
	//    unification (e.g. "anthropic-claude-..." from a dotted key).
	rawTokens := splitNonEmpty(s, "-")
	for len(rawTokens) > 1 {
		if _, ok := knownPrefixSegments[rawTokens[0]]; ok {
			rawTokens = rawTokens[1:]
			continue
		}
		break
	}

	tokens := make(map[string]struct{}, len(rawTokens))
	versions := make(map[string]struct{})
	canonicalTokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		if _, ok := marketingSuffixTokens[t]; ok {
			continue
		}
		tokens[t] = struct{}{}
		if reHasDigit.MatchString(t) {
			versions[t] = struct{}{}
		}
		canonicalTokens = append(canonicalTokens, t)
	}

	return normalized{
		canonical: strings.Join(canonicalTokens, "-"),
		tokens:    tokens,
		versions:  versions,
	}
}

func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// index is a normalized lookup table over registry keys.
type index struct {
	byCanonical map[string]string // canonical -> registry key
	entries     []indexEntry
}

type indexEntry struct {
	key  string
	norm normalized
}

func buildIndex(keys []string) *index {
	idx := &index{byCanonical: make(map[string]string, len(keys))}
	for _, k := range keys {
		n := normalize(k)
		if n.canonical == "" {
			continue
		}
		// Prefer the shortest source key for a given canonical form (less noise).
		if existing, ok := idx.byCanonical[n.canonical]; !ok || len(k) < len(existing) {
			idx.byCanonical[n.canonical] = k
		}
		idx.entries = append(idx.entries, indexEntry{key: k, norm: n})
	}
	return idx
}

// match resolves a raw model name to a registry key. Strategy, in order:
//  1. exact canonical equality;
//  2. identical token set + identical version signature;
//  3. token subset (one side ⊆ other) + identical version signature, preferring
//     the closest size.
//
// Every fuzzy tier requires the version signatures to match exactly, so models
// that differ only by version (4.8 vs 4.6) never collide.
func (idx *index) match(raw string) (string, bool) {
	n := normalize(raw)
	if n.canonical == "" {
		return "", false
	}
	if key, ok := idx.byCanonical[n.canonical]; ok {
		return key, true
	}

	bestKey := ""
	bestScore := -1
	for _, e := range idx.entries {
		if !sameSet(n.versions, e.norm.versions) {
			continue
		}
		score := setMatchScore(n.tokens, e.norm.tokens)
		if score < 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestKey = e.key
		}
	}
	if bestKey != "" {
		return bestKey, true
	}
	return "", false
}

// setMatchScore returns a relevance score for two token sets, or -1 if they are
// not match candidates. Equal sets score highest; otherwise one must be a subset
// of the other and the score is the size of the overlap (penalized by extra
// tokens on the larger side).
func setMatchScore(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return -1
	}
	if sameSet(a, b) {
		return 1_000_000
	}
	inter := 0
	for t := range a {
		if _, ok := b[t]; ok {
			inter++
		}
	}
	// require subset relation (no divergent non-version tokens)
	if inter != len(a) && inter != len(b) {
		return -1
	}
	// The extra tokens on the larger side must all be ignorable noise (vendor /
	// region prefixes or marketing suffixes). A meaningful distinguishing token
	// (pro, mini, nano, flash, lite, plus, codex, chat, coder, ...) identifies a
	// sibling/variant model with potentially different context window and
	// capabilities, so it must NOT be absorbed via subset matching. Without this
	// guard, "deepseek-v4-pro" would borrow "deepseek-v4"'s capabilities whenever
	// the registry lacks the exact variant key.
	larger, smaller := a, b
	if len(b) > len(a) {
		larger, smaller = b, a
	}
	for t := range larger {
		if _, ok := smaller[t]; ok {
			continue
		}
		if !isIgnorableToken(t) {
			return -1
		}
	}
	return inter*1000 - len(larger)
}

// isIgnorableToken reports whether a token is pure naming noise (a vendor/region
// prefix or a marketing suffix) that may legitimately differ between two names
// for the same model. Anything else is treated as model-distinguishing.
func isIgnorableToken(t string) bool {
	if _, ok := marketingSuffixTokens[t]; ok {
		return true
	}
	_, ok := knownPrefixSegments[t]
	return ok
}

func sameSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for t := range a {
		if _, ok := b[t]; !ok {
			return false
		}
	}
	return true
}
