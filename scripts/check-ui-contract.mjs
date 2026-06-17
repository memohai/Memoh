#!/usr/bin/env node
// UI contract guard — keeps the cross-cutting design primitives from drifting
// back into per-component free-styling (the class of bug that shipped a
// hand-rolled icon hover and stray radii). Single sources live in:
//   · packages/ui/src/style.css         (interaction contracts, type/radius tokens)
//   · packages/ui/src/components/button  (the one icon button: Button variant=ghost)
//
// TWO rule families with different SCOPE:
//   · CONTRACT rules — packages/ui ONLY (raw color / arbitrary radius / invented
//     shadow / disabled opacity / field-edge / selection ring). The library is the
//     single source the app consumes; app pages are intentionally out of scope here.
//   · px-SCALING rules — packages/ui AND apps/web. The root font-size is driven by
//     --memoh-ui-font-size, so rem scales with the user's font setting / browser zoom
//     while a hardcoded px does NOT. A px on a property that gates TEXT therefore
//     stops growing while the surrounding text grows → clipped / misaligned controls.
//
// px rules — HARD FAIL (exit 1):
//   · text-[Npx]      font-size never scales        → use a --text-* token
//   · leading-[Npx]   line-height never scales       → use rem / unitless
//   · h/min-h-[Npx], p*/gap/space-[Npx]  (N >= 5)     → a text box / its spacing won't
//     grow with the font → use rem (the spacing scale, min-h, etc).
// NOT flagged: width / cap props (w / min-w / max-w / max-h-[Npx]) are a reflow &
//   sizing concern, not text-coupled, so they are left to review; and decorative
//   1–4px hairlines / indicator bars, plus border-/ring-/outline-/translate-/inset-/
//   size-/blur-/rounded-* (never a text box). Escape hatch: put `ui-allow-px` in a
//   comment on the SAME line for a sanctioned exception (e.g. a fixed chart height).
//
// px BASELINE RATCHET: scripts/ui-px-baseline.json records the px-HARD count per
//   file. A file may keep its grandfathered count, but ADDING px (count grows, or a
//   brand-new file) HARD-fails — so new code can't regress while the existing debt is
//   burned down per cluster. Regenerate after a cleanup pass with:
//     node scripts/check-ui-contract.mjs --write-baseline
//
// CONTRACT rules (packages/ui), unchanged:
//   1. disabled-context opacity that is not 40   → disabled is opacity-40 everywhere
//   2. arbitrary radius rounded-[Npx] / rounded-[calc(...)] → use rounded-sm/md/lg
//   5. raw color in a .vue arbitrary class       → use a palette token
//   6. raw color in the style.css COMPONENT layer → define + use a token
//   7. box-shadow with a raw color               → never invent chrome (use a token)
//   4. likely hand-rolled icon-button hover (WARN)
//   8. raw shadow utility (WARN)  9. border-input on a control (WARN)
//  10. ring-offset-* selection halo (WARN)
//
// Run: node scripts/check-ui-contract.mjs   (wired into `mise run lint`)
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..')
// FULL_DIRS get every rule; PX_DIRS get only the px-scaling rules (app pages keep
// their own raw-color debt out of this guard — that is a separate known gap).
const FULL_DIRS = ['packages/ui/src']
const PX_DIRS = ['apps/web/src']
const EXT = /\.(vue|ts)$/
const BASELINE_PATH = join(ROOT, 'scripts/ui-px-baseline.json')
const WRITE_BASELINE = process.argv.includes('--write-baseline')

// Exact arbitrary-radius tokens that are legitimately allowed.
//   rounded-[calc(var(--radius)-5px)] = the tuned 5px in-field small-control radius
//   (InputGroup clear/reveal + NumberField steppers). Smaller than the 8px family
//   radius on purpose so a 24px box does not read as a pill.
const RADIUS_ALLOW = new Set([
  'rounded-[inherit]',
  'rounded-[2px]',
  'rounded-[calc(var(--radius)-5px)]',
])
// Box props (height / padding / gap / space) below this px size are hairlines or
// indicator bars (a 2px tab underline, a 3px link offset), never a text container —
// so they are decorative and not flagged. 5px+ is where a real text box starts.
const MIN_BOX_PX = 5

const hard = []
const warn = []
// px-HARD candidates, collected separately so the baseline ratchet can grandfather
// existing debt before they are promoted into `hard`.
const pxHard = []

function walk(dir, full) {
  for (const name of readdirSync(dir)) {
    const fullPath = join(dir, name)
    const st = statSync(fullPath)
    if (st.isDirectory()) walk(fullPath, full)
    else if (EXT.test(name)) scan(fullPath, full)
  }
}

// Strip comments so descriptive prose ("was the old border-input / shadow-md")
// never trips a smell check. Tracks // line comments and /* */ + <!-- --> blocks
// across lines (block state lives in the caller, reset per file). Code after //
// (incl. URLs) is dropped — harmless, we never assert on URLs.
function makeCodeStripper() {
  let blockEnd = null
  return (line) => {
    let out = ''
    let i = 0
    while (i < line.length) {
      if (blockEnd) {
        const idx = line.indexOf(blockEnd, i)
        if (idx === -1) return out
        i = idx + blockEnd.length
        blockEnd = null
        continue
      }
      if (line.startsWith('//', i)) return out
      if (line.startsWith('/*', i)) { blockEnd = '*/'; i += 2; continue }
      if (line.startsWith('<!--', i)) { blockEnd = '-->'; i += 4; continue }
      out += line[i]
      i++
    }
    return out
  }
}

function scan(file, full) {
  const rel = relative(ROOT, file)
  const src = readFileSync(file, 'utf8')
  const lines = src.split('\n')
  const codeOf = makeCodeStripper()
  lines.forEach((rawLine, i) => {
    const line = codeOf(rawLine)
    const ln = i + 1
    // Same-line escape hatch for a sanctioned px (kept on the RAW line so the
    // comment survives the code-stripper).
    const allowPx = rawLine.includes('ui-allow-px')
    for (const tok of line.split(/[\s'"`]+/)) {
      if (!tok) continue

      // ── px-scaling rules (run in BOTH scopes) ────────────────────────────
      if (!allowPx) {
        // font-size / line-height in px never scale — any value is wrong.
        if (/(?:^|:)text-\[\d+(?:\.\d+)?px\]/.test(tok))
          pxHard.push({ rel, ln, msg: `px font-size never scales (use a --text-* token) → ${tok}` })
        if (/(?:^|:)leading-\[\d+(?:\.\d+)?px\]/.test(tok))
          pxHard.push({ rel, ln, msg: `px line-height never scales (use rem/unitless) → ${tok}` })
        // height / padding / gap / space: a text box (>=5px) that won't grow.
        let m
        if ((m = tok.match(/(?:^|:)(?:min-h|h)-\[(\d+(?:\.\d+)?)px\]/)) && Number(m[1]) >= MIN_BOX_PX)
          pxHard.push({ rel, ln, msg: `px height won't grow with the font (use rem / min-h) → ${tok}` })
        if ((m = tok.match(/(?:^|:)p[xytblrse]?-\[(\d+(?:\.\d+)?)px\]/)) && Number(m[1]) >= MIN_BOX_PX)
          pxHard.push({ rel, ln, msg: `px padding won't scale (use the rem spacing scale) → ${tok}` })
        if ((m = tok.match(/(?:^|:)(?:gap(?:-[xy])?|space-[xy])-\[(\d+(?:\.\d+)?)px\]/)) && Number(m[1]) >= MIN_BOX_PX)
          pxHard.push({ rel, ln, msg: `px gap/space won't scale (use the rem spacing scale) → ${tok}` })
      }

      // ── contract rules (packages/ui only) ────────────────────────────────
      if (!full) continue
      // 1. disabled opacity must be 40
      const op = tok.match(/opacity-(\d+)$/)
      if (op && /disabled/.test(tok) && op[1] !== '40')
        hard.push(`${rel}:${ln}  disabled opacity must be 40 → ${tok}`)
      // 2. arbitrary radius
      if (/^(?:[a-z-]+:)*rounded-\[/.test(tok) && !RADIUS_ALLOW.has(tok.replace(/^(?:[a-z-]+:)*/, '')))
        hard.push(`${rel}:${ln}  arbitrary radius (use rounded-sm/md/lg) → ${tok}`)
      // 5. raw color in a Tailwind arbitrary class (bg-[#..], text-[oklch(..)], …)
      if (/-\[(?:#|(?:rgba?|hsla?|oklch|oklab|lab|lch|color-mix)\()/.test(tok))
        hard.push(`${rel}:${ln}  raw color in arbitrary class (use a palette token) → ${tok}`)
      // 8. raw tailwind shadow scale utility — elevation is tokenized
      //    (shadow-[var(--shadow-*)] and shadow-none are the allowed forms).
      if (/(?:^|:)shadow-(?:2xs|xs|sm|md|lg|xl|2xl)$/.test(tok))
        warn.push(`${rel}:${ln}  raw shadow utility (use an elevation token or shadow-none) → ${tok}`)
      // 9. structural border on a control body — controls use the field-edge family
      if (/(?:^|:)border-input$/.test(tok))
        warn.push(`${rel}:${ln}  border-input on a control (use the field-edge contract) → ${tok}`)
      // 10. selection/active via an offset ring halo — use an indicator / --ui-selected
      if (/(?:^|:)ring-offset(?:-|$)/.test(tok))
        warn.push(`${rel}:${ln}  ring-offset (selection via offset halo — use an indicator) → ${tok}`)
    }
    // 4. likely hand-rolled icon-button hover (icon present + ad-hoc hover fill)
    if (full && /\[&[_>]svg/.test(line) && /hover:bg-(accent|\[)/.test(line) && !/data-slot="button"/.test(line))
      warn.push(`${rel}:${ln}  possible hand-rolled icon hover — reuse <Button variant="ghost">`)
  })
}

// style.css is where tokens are DEFINED (raw values legal in :root/.dark/@theme)
// and where component styling is AUTHORED (raw values illegal there). Scan it with
// block awareness so we only flag raw color / invented box-shadow in the
// component layer, never in the token-definition blocks.
const COLOR_FN = /(?:^|[^\w-])(?:rgba?|hsla?|oklch|oklab|lab|lch|color-mix)\s*\(/
const HEX = /#[0-9a-fA-F]{3,8}\b/
function scanCss(file) {
  const rel = relative(ROOT, file)
  const lines = readFileSync(file, 'utf8').split('\n')
  let depth = 0
  let tokenBlockDepth = -1 // depth at which the active token-definition block opened
  lines.forEach((line, i) => {
    const ln = i + 1
    const trimmed = line.trim()
    const isComment = trimmed.startsWith('/*') || trimmed.startsWith('*') || trimmed.startsWith('//')
    const inTokenBlock = tokenBlockDepth !== -1
    const opensTokenBlock = /^(?::root|\.dark|@theme)\b/.test(trimmed) && line.includes('{')
    if (!inTokenBlock && !isComment) {
      // 6. raw color literal in the component layer → must be a token
      if (HEX.test(line) || COLOR_FN.test(line))
        hard.push(`${rel}:${ln}  raw color in component CSS (define a token) → ${trimmed.slice(0, 64)}`)
      // 7. box-shadow with a RAW color = invented chrome. A shadow built purely
      //    from var() tokens (e.g. the field edge: var(--field-edge)) is fine.
      const bs = line.match(/box-shadow:\s*([^;]+);?/)
      if (bs && (HEX.test(bs[1]) || COLOR_FN.test(bs[1])))
        hard.push(`${rel}:${ln}  box-shadow with raw color (use a token) → ${bs[1].trim().slice(0, 50)}`)
    }
    if (opensTokenBlock && tokenBlockDepth === -1) tokenBlockDepth = depth
    for (const ch of line) {
      if (ch === '{') depth++
      else if (ch === '}') {
        depth--
        if (tokenBlockDepth !== -1 && depth <= tokenBlockDepth) tokenBlockDepth = -1
      }
    }
  })
}

for (const d of FULL_DIRS) walk(join(ROOT, d), true)
for (const d of PX_DIRS) walk(join(ROOT, d), false)
scanCss(join(ROOT, 'packages/ui/src/style.css'))

// ── px baseline ratchet ──────────────────────────────────────────────────────
const pxByFile = {}
for (const v of pxHard) pxByFile[v.rel] = (pxByFile[v.rel] || 0) + 1

if (WRITE_BASELINE) {
  const sorted = Object.fromEntries(Object.entries(pxByFile).sort(([a], [b]) => a.localeCompare(b)))
  writeFileSync(BASELINE_PATH, `${JSON.stringify(sorted, null, 2)}\n`)
  const total = Object.values(sorted).reduce((a, b) => a + b, 0)
  console.log(`✓ wrote px baseline: ${total} grandfathered px violation(s) across ${Object.keys(sorted).length} file(s)`)
  process.exit(0)
}

const baseline = existsSync(BASELINE_PATH) ? JSON.parse(readFileSync(BASELINE_PATH, 'utf8')) : {}
let grandfathered = 0
for (const [rel, count] of Object.entries(pxByFile)) {
  const allowed = baseline[rel] || 0
  if (count > allowed) {
    // The whole file's px lines are surfaced; the count guard is what fails (we
    // can't pin WHICH instance is new from a per-file count, but "no new px" holds).
    for (const v of pxHard) if (v.rel === rel) hard.push(`${v.rel}:${v.ln}  ${v.msg}`)
    hard.push(`${rel}  ✗ px count ${count} exceeds baseline ${allowed} — no NEW px (fix it, or mark the line with ui-allow-px)`)
  }
  else {
    grandfathered += count
  }
}

if (warn.length) {
  console.warn(`\n⚠ UI contract — ${warn.length} warning(s):`)
  for (const w of warn) console.warn(`  ${w}`)
}
if (grandfathered)
  console.log(`\nℹ px baseline: ${grandfathered} grandfathered px violation(s) remaining — burn down per cluster, then re-run with --write-baseline`)
if (hard.length) {
  console.error(`\n✗ UI contract — ${hard.length} violation(s):`)
  for (const h of hard) console.error(`  ${h}`)
  console.error('\nSee packages/ui/src/style.css for the single sources.\n')
  process.exit(1)
}
console.log(`✓ UI contract OK${warn.length ? ` (${warn.length} warning(s))` : ''}`)
