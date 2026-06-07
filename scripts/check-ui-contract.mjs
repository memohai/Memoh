#!/usr/bin/env node
// UI contract guard — keeps the cross-cutting design primitives from drifting
// back into per-component free-styling (the class of bug that shipped a
// hand-rolled icon hover and stray radii). Single sources live in:
//   · packages/ui/src/style.css         (interaction contracts, type/radius tokens)
//   · packages/ui/src/components/button  (the one icon button: Button variant=ghost)
//
// Scoped to the component LIBRARY (packages/ui) — that is the single source the
// rest of the app consumes; app pages are out of scope here.
//
// HARD FAIL (exit 1):
//   1. disabled-context opacity that is not 40   → disabled is opacity-40 everywhere
//   2. arbitrary radius rounded-[Npx] / rounded-[calc(...)] → use rounded-sm/md/lg
//   5. raw color in a .vue arbitrary class       → use a palette token
//   6. raw color in the style.css COMPONENT layer → define + use a token
//   7. box-shadow with a raw color               → never invent chrome (use a token)
// WARN (exit 0):
//   3. OFF-SCALE raw text-[Npx] (not on the type scale) → use a type token
//   4. likely hand-rolled icon-button hover      → reuse <Button variant="ghost">
//   8. raw shadow utility (shadow-xs/sm/md/lg/xl/2xl) → use an elevation token / shadow-none
//   9. border-input on a control body            → use the field-edge contract
//  10. ring-offset-* (selection via offset halo) → use an indicator / --ui-selected
//
// Red lines 8/9/10 are the string-detectable § Dirty patterns (AGENTS.md). They
// are WARN for now (legacy ButtonGroup/Sidebar still ship a flat shadow-sm, and
// PinInput/InputOTP/TagsInput are mid-refactor); promote to HARD once those land.
//
// Rules 5/6/7 are HARD now that the pre-contract debt is fully migrated (the
// library has zero raw values outside token blocks). Token DEFINITION blocks
// (:root / .dark / @theme) are where raw values belong, so they're skipped.
// If a legitimate raw value ever needs to ship, promote it to a token instead.
//
// Run: node scripts/check-ui-contract.mjs   (wired into `mise run lint`)
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..')
const SCAN_DIRS = ['packages/ui/src']
const EXT = /\.(vue|ts)$/

// Exact arbitrary-radius tokens that are legitimately allowed.
//   rounded-[calc(var(--radius)-5px)] = the tuned 5px in-field small-control radius
//   (InputGroup clear/reveal + NumberField steppers). Smaller than the 8px family
//   radius on purpose so a 24px box does not read as a pill.
const RADIUS_ALLOW = new Set([
  'rounded-[inherit]',
  'rounded-[2px]',
  'rounded-[calc(var(--radius)-5px)]',
])
// The type scale (px). Raw text-[Npx] ON this scale equals a token and is
// tolerated; anything OFF the scale is a genuine one-off smell → warn.
const TYPE_SCALE_PX = new Set([11, 12, 13, 14, 16, 18, 24])

const hard = []
const warn = []

function walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    const st = statSync(full)
    if (st.isDirectory()) walk(full)
    else if (EXT.test(name)) scan(full)
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

function scan(file) {
  const rel = relative(ROOT, file)
  const src = readFileSync(file, 'utf8')
  const lines = src.split('\n')
  const codeOf = makeCodeStripper()
  lines.forEach((rawLine, i) => {
    const line = codeOf(rawLine)
    const ln = i + 1
    for (const tok of line.split(/[\s'"`]+/)) {
      if (!tok) continue
      // 1. disabled opacity must be 40
      const op = tok.match(/opacity-(\d+)$/)
      if (op && /disabled/.test(tok) && op[1] !== '40')
        hard.push(`${rel}:${ln}  disabled opacity must be 40 → ${tok}`)
      // 2. arbitrary radius
      if (/^(?:[a-z-]+:)*rounded-\[/.test(tok) && !RADIUS_ALLOW.has(tok.replace(/^(?:[a-z-]+:)*/, '')))
        hard.push(`${rel}:${ln}  arbitrary radius (use rounded-sm/md/lg) → ${tok}`)
      // 3. OFF-SCALE raw px font size (on-scale sizes equal a token → tolerated)
      const tx = tok.match(/text-\[(\d+)(?:\.\d+)?px\]/)
      if (tx && !TYPE_SCALE_PX.has(Number(tx[1])))
        warn.push(`${rel}:${ln}  off-scale font size (use a type token) → ${tok}`)
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
    if (/\[&[_>]svg/.test(line) && /hover:bg-(accent|\[)/.test(line) && !/data-slot="button"/.test(line))
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

for (const d of SCAN_DIRS) walk(join(ROOT, d))
scanCss(join(ROOT, 'packages/ui/src/style.css'))

if (warn.length) {
  console.warn(`\n⚠ UI contract — ${warn.length} warning(s):`)
  for (const w of warn) console.warn('  ' + w)
}
if (hard.length) {
  console.error(`\n✗ UI contract — ${hard.length} violation(s):`)
  for (const h of hard) console.error('  ' + h)
  console.error('\nSee packages/ui/src/style.css for the single sources.\n')
  process.exit(1)
}
console.log(`✓ UI contract OK${warn.length ? ` (${warn.length} warning(s))` : ''}`)
