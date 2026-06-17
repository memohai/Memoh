// Split a string into consecutive same-script runs so the chat can give CJK and
// Latin their own font-weight in one line (MiSans reads heavier than Inter at the
// same numeric weight, so a single weight can never satisfy both). This is the
// render-time half of the fix; style.css carries the paired weight tokens that
// the emitted .chat-cjk / .chat-latin spans consume.
//
// Only ever fed PLAIN TEXT (markstream routes code / math / inline-code / emoji
// through their own node types, never the text node — see md-text.vue), so there
// is nothing here to "break" for code or formulae.
export type ScriptRun = { script: 'cjk' | 'latin'; text: string }

// Han, kana, and the CJK compatibility/extension blocks.
const CJK = /[\u2E80-\u2FDF\u3040-\u30FF\u3400-\u4DBF\u4E00-\u9FFF\uF900-\uFAFF\uFF66-\uFF9F]/
// CJK punctuation + fullwidth forms ride WITH the CJK run (a fullwidth comma is a
// CJK glyph visually), so they take the CJK weight rather than splitting a run.
const CJK_PUNCT = /[\u3000-\u303F\uFF01-\uFF60\uFFE0-\uFFEF]/
// Latin letters + digits anchor a Latin run.
const LATIN = /[A-Za-z0-9]/

function classOf(ch: string): 'cjk' | 'latin' | 'neutral' {
  if (CJK.test(ch) || CJK_PUNCT.test(ch)) return 'cjk'
  if (LATIN.test(ch)) return 'latin'
  // Spaces, ASCII punctuation, emoji, symbols — no script of their own.
  return 'neutral'
}

// Neutral chars (spaces, punctuation, emoji) attach to the run already in
// progress so we don't shatter "Hello, world." into fragments; leading neutrals
// before any scripted char wait in `lead` and join the first real run. All
// characters are preserved in order, so joining the runs' text reproduces the
// input exactly (newlines/spaces included — safe under white-space: pre-wrap).
export function splitScriptRuns(text: string): ScriptRun[] {
  if (!text) return []
  const runs: ScriptRun[] = []
  let script: 'cjk' | 'latin' | null = null
  let buf = ''
  let lead = ''
  // Iterate by code point so surrogate-pair emoji classify as one neutral char.
  for (const ch of text) {
    const c = classOf(ch)
    if (c === 'neutral') {
      if (script === null) lead += ch
      else buf += ch
      continue
    }
    if (script === null) {
      script = c
      buf = lead + ch
      lead = ''
    } else if (c === script) {
      buf += ch
    } else {
      runs.push({ script, text: buf })
      script = c
      buf = ch
    }
  }
  if (script === null) {
    // All-neutral (e.g. a lone emoji) → Latin so it rides the 400 base.
    if (lead) runs.push({ script: 'latin', text: lead })
  } else {
    runs.push({ script, text: buf })
  }
  return runs
}
