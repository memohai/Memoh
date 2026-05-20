package docs

import "embed"

// HelpCorpus embeds the user-facing Memoh documentation used by the memoh_help
// agent tool. docs/docs remains the single source of truth; this package only
// gives Go code a legal //go:embed root for selected help sections.
//
//go:embed docs/getting-started docs/channels docs/installation docs/memory-providers docs/tts-providers docs/zh/getting-started docs/zh/channels docs/zh/installation docs/zh/memory-providers docs/zh/tts-providers
var HelpCorpus embed.FS
