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
// WARN (exit 0):
//   3. OFF-SCALE raw text-[Npx] (not on the type scale) → use a type token
//   4. likely hand-rolled icon-button hover      → reuse <Button variant="ghost">
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

function scan(file) {
  const rel = relative(ROOT, file)
  const src = readFileSync(file, 'utf8')
  const lines = src.split('\n')
  lines.forEach((line, i) => {
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
    }
    // 4. likely hand-rolled icon-button hover (icon present + ad-hoc hover fill)
    if (/\[&[_>]svg/.test(line) && /hover:bg-(accent|\[)/.test(line) && !/data-slot="button"/.test(line))
      warn.push(`${rel}:${ln}  possible hand-rolled icon hover — reuse <Button variant="ghost">`)
  })
}

for (const d of SCAN_DIRS) walk(join(ROOT, d))

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
