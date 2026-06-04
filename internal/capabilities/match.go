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
	"sort"
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

// modelVendorSegments are the subset of knownPrefixSegments that double as a
// model-family name (the producer's brand can be part of the model identity,
// e.g. "deepseek-coder", "qwen-coder", "mistral-large", "meta-llama-3"). They
// are still treated as ignorable noise for fuzzy subset matching (so
// "anthropic/claude-opus" still matches "claude-opus"), but they must NOT be
// stripped from a *bare* hyphenated slug in the post-unification re-strip step:
// stripping them collapses distinct families to a shared generic tail (e.g.
// "deepseek-coder" and "qwen-coder" both -> "coder"), cross-matching unrelated
// models. Dotted/slashed gateway prefixes are still removed earlier where the
// segment is unambiguously a routing prefix, not part of the model name.
var modelVendorSegments = map[string]struct{}{
	"anthropic": {}, "openai": {}, "google": {}, "gemini": {},
	"deepseek": {}, "mistral": {}, "cohere": {}, "xai": {},
	"qwen": {}, "alibaba": {}, "meta": {}, "meta_llama": {}, "moonshot": {},
}

// marketingSuffixTokens are pure release-channel / alias decorations that point
// at the same underlying model and should not block a match. We deliberately do
// NOT include capability-distinguishing tokens here: the LiteLLM registry is
// comprehensive and lists variants such as "...-fast", "...-exp",
// "...-fast-reasoning" as their own keys with their own context windows and
// reasoning shapes, so those tokens must be kept and matched exactly (or MISS),
// never folded into the base model.
var marketingSuffixTokens = map[string]struct{}{
	"latest": {}, "preview": {}, "beta": {}, "online": {},
}

// thinkingVariantTokens mark a thinking/non-thinking model variant. They are
// capability-distinguishing (the registry lists e.g. reasoning-enabled
// "kimi-k2-thinking" separately from a non-thinking "kimi-k2"), so they stay in
// the canonical signature and a bare base id must NOT borrow a distinct
// "-thinking" key's reasoning. They are handled asymmetrically by
// matchThinkingBase: only when the INPUT carries the marker and the qualified
// variant has no own key do we fall back to the base model (the marker is then
// just a runtime toggle on the same base, e.g. "claude-opus-4-8-thinking").
var thinkingVariantTokens = map[string]struct{}{
	"thinking": {}, "nonthinking": {},
}

var (
	// trailing date: -20250514 or -2025-05-14.
	reDateCompact = regexp.MustCompile(`-\d{8}$`)
	reDateDashed  = regexp.MustCompile(`-20\d{2}-\d{2}-\d{2}$`)
	// trailing vendor decorations, stripped while original delimiters are intact:
	//   @default (vertex), -v1:0 (bedrock revision), :0 (revision).
	// IMPORTANT: only a bedrock-style "-vN:M" (with a colon revision) is stripped.
	// A bare "-vN" (e.g. "deepseek-v4", "titan-...-v2") carries real version
	// information and MUST survive into the version signature, otherwise distinct
	// versions collapse to one canonical and the version veto is bypassed.
	reAtSuffix   = regexp.MustCompile(`@[a-z0-9._-]+$`)
	reBedrockVer = regexp.MustCompile(`[-_]v\d+:\d+$`)
	reColonVer   = regexp.MustCompile(`:\d+$`)
	// a token carries version information if it contains a digit.
	reHasDigit = regexp.MustCompile(`\d`)
)

// latencyVariantTokens are low-latency sibling markers that share the parent
// model's reasoning shape (thinking mode + effort tiers) but may differ in
// context window. When the registry lacks an exact key for such a variant we
// fall back to the base model's reasoning shape only (see Registry.Lookup),
// never its context window.
var latencyVariantTokens = map[string]struct{}{
	"fast": {},
}

// normalized is the canonical signature of a model name used for matching.
type normalized struct {
	canonical       string              // hyphen-joined non-marketing tokens
	canonicalTokens []string            // ordered tokens behind canonical (for rebuild)
	tokens          map[string]struct{} // full token set (incl. version tokens)
	versions        map[string]struct{} // tokens that carry version info (digit-bearing)
}

// withoutTokens returns a copy of the signature with the given tokens removed,
// preserving order. Used to derive a base name from a variant (e.g. drop "fast").
func (n normalized) withoutTokens(drop map[string]struct{}) normalized {
	kept := make([]string, 0, len(n.canonicalTokens))
	for _, t := range n.canonicalTokens {
		if _, d := drop[t]; d {
			continue
		}
		kept = append(kept, t)
	}
	tokens := make(map[string]struct{}, len(kept))
	versions := make(map[string]struct{})
	for _, t := range kept {
		tokens[t] = struct{}{}
		if reHasDigit.MatchString(t) {
			versions[t] = struct{}{}
		}
	}
	return normalized{
		canonical:       strings.Join(kept, "-"),
		canonicalTokens: kept,
		tokens:          tokens,
		versions:        versions,
	}
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

	// 6. Re-strip leading gateway/region prefixes that only surfaced after
	//    separator unification (e.g. "vertex_ai-..." glued by '_'). Model-family
	//    vendor tokens (deepseek/qwen/mistral/...) are NOT stripped here: on a
	//    bare slug they are part of the model identity, and stripping them would
	//    collapse distinct families ("deepseek-coder" and "qwen-coder" -> "coder")
	//    into a single canonical and cross-match unrelated models. Same-vendor
	//    redundant prefixes are still tolerated by the fuzzy matcher, which
	//    treats these tokens as ignorable noise (see isIgnorableToken).
	rawTokens := splitNonEmpty(s, "-")
	for len(rawTokens) > 1 {
		if _, ok := knownPrefixSegments[rawTokens[0]]; !ok {
			break
		}
		if _, vendor := modelVendorSegments[rawTokens[0]]; vendor {
			break
		}
		rawTokens = rawTokens[1:]
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
		canonical:       strings.Join(canonicalTokens, "-"),
		canonicalTokens: canonicalTokens,
		tokens:          tokens,
		versions:        versions,
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
	byExact     map[string]string // lowercased/trimmed raw key -> registry key
	byCanonical map[string]string // canonical -> registry key
	entries     []indexEntry
}

type indexEntry struct {
	key  string
	norm normalized
}

// isUntrustedRegistryKey reports whether a registry key comes from a gateway
// that repackages other vendors' models without authoritative capability
// metadata. GitHub Copilot is the case in point: every github_copilot/* entry
// carries no reasoning flags and exposes Copilot's truncated context window
// (e.g. gpt-5 at 128k instead of the real 272k). Matching an official model to
// such a shell would strip its reasoning capabilities, so these keys are
// excluded from the index entirely. A model served only via Copilot then MISSes
// and falls back to safe defaults / the base reasoning shape.
func isUntrustedRegistryKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	return strings.HasPrefix(k, "github_copilot/") || strings.HasPrefix(k, "copilot/")
}

func buildIndex(keys []string) *index {
	idx := &index{
		byExact:     make(map[string]string, len(keys)),
		byCanonical: make(map[string]string, len(keys)),
	}
	// Sort first: registry keys arrive from a map (random order). Sorting makes
	// both the byCanonical representative choice and the fuzzy-tie winner in
	// matchNorm deterministic across processes.
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for _, k := range sorted {
		if isUntrustedRegistryKey(k) {
			continue
		}
		// Exact provider-qualified key match takes priority over canonical
		// folding: a fully-specified id like "perplexity/anthropic/claude-haiku-4-5"
		// (which the registry marks supports_reasoning:false) must resolve to its
		// own entry, not be folded into the native "claude-haiku-4-5" (reasoning
		// on) and silently over-enable thinking on a provider that disables it.
		if exact := strings.ToLower(strings.TrimSpace(k)); exact != "" {
			if _, exists := idx.byExact[exact]; !exists {
				idx.byExact[exact] = k
			}
		}
		n := normalize(k)
		if n.canonical == "" {
			continue
		}
		// Prefer the shortest source key for a given canonical form (less noise);
		// ties broken by the sorted order above.
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
	// Exact key match wins: it is the most specific, authoritative resolution and
	// cannot be wrong (the input string IS a registry key). This also preserves
	// provider-specific capability overrides that canonical folding would lose.
	if exact := strings.ToLower(strings.TrimSpace(raw)); exact != "" {
		if key, ok := idx.byExact[exact]; ok {
			return key, true
		}
	}
	n := normalize(raw)
	if key, ok := idx.matchNorm(n); ok {
		return key, true
	}
	// A "<base>-thinking"/"-nonthinking" input with no own registry key falls
	// back to the base model's reasoning shape. This is asymmetric on purpose:
	// the base is reached only when the marker is on the INPUT, so a bare base id
	// can never borrow a distinct "-thinking" variant's capabilities.
	return idx.matchThinkingBase(n)
}

// matchThinkingBase resolves the base model for a thinking/non-thinking input
// (e.g. "claude-opus-4-8-thinking" -> "claude-opus-4-8") when the qualified
// variant itself has no registry key. Returns false unless the input carries a
// thinking marker AND a different base canonical resolves.
func (idx *index) matchThinkingBase(n normalized) (string, bool) {
	hasMarker := false
	for t := range n.tokens {
		if _, ok := thinkingVariantTokens[t]; ok {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return "", false
	}
	base := n.withoutTokens(thinkingVariantTokens)
	if base.canonical == "" || base.canonical == n.canonical {
		return "", false
	}
	return idx.matchNorm(base)
}

// matchNorm runs the matching strategy over an already-normalized signature.
func (idx *index) matchNorm(n normalized) (string, bool) {
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

// matchLatencyBase resolves the base model for a low-latency variant (e.g.
// "claude-opus-4.8-fast" -> "claude-opus-4-8") when the variant itself has no
// registry key. It returns false unless the input carries a latency-variant
// token AND a different base resolves. Callers must use only the base model's
// reasoning shape from the result, never its context window.
func (idx *index) matchLatencyBase(raw string) (string, bool) {
	n := normalize(raw)
	hasLatency := false
	for t := range n.tokens {
		if _, ok := latencyVariantTokens[t]; ok {
			hasLatency = true
			break
		}
	}
	if !hasLatency {
		return "", false
	}
	base := n.withoutTokens(latencyVariantTokens)
	if base.canonical == "" || base.canonical == n.canonical {
		return "", false
	}
	return idx.matchNorm(base)
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
