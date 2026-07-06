#!/usr/bin/env node
/**
 * check-patched-deps — verify that pnpm's patchedDependencies were actually
 * applied to the installed packages.
 *
 * WHY THIS EXISTS: pnpm names each patched package's directory in
 * node_modules/.pnpm with the patch content hash (…_patch_hash=<hash>) and
 * trusts that name on later installs. If the materialization that first
 * produced the directory was corrupted (interrupted install, a store shared
 * across filesystems/pnpm versions), the directory keeps the CORRECT hash in
 * its name but UNPATCHED content — and every subsequent `pnpm install` reuses
 * it silently, forever. The failure mode downstream is maddening: our CSS/JS
 * written against the patched behavior is correct, yet the app behaves as if
 * the patch never existed (seen in production dev-env: the dock tab-strip "+"
 * pinned to the far right because the patched DOM restructure was missing).
 *
 * WHAT IT CHECKS: for every entry in pnpm-workspace.yaml patchedDependencies,
 * parse the .patch file, and for each patched file assert that its most
 * distinctive ADDED line is present in the installed copy under
 * node_modules/.pnpm/<pkg>_patch_hash=<any>/node_modules/<pkg>/. Content is
 * the ground truth; the directory name is not trusted.
 *
 * Wired as the root `postinstall`, so a poisoned install fails loudly at
 * install time instead of surfacing as an unexplainable UI bug weeks later.
 * Recovery, printed on failure: wipe node_modules/.pnpm + the store and
 * reinstall.
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

/** Parse the patchedDependencies block of pnpm-workspace.yaml.
 * Deliberately a line-scanner, not a YAML parser: this script runs as
 * postinstall and must not depend on anything from node_modules (which it is
 * itself validating). The block's shape is a flat `  name@version: path` map. */
function readPatchedDependencies() {
  const yaml = fs.readFileSync(path.join(repoRoot, 'pnpm-workspace.yaml'), 'utf8')
  const entries = []
  let inBlock = false
  for (const line of yaml.split('\n')) {
    if (/^patchedDependencies:\s*$/.test(line)) {
      inBlock = true
      continue
    }
    if (inBlock) {
      const m = line.match(/^\s+(\S+?):\s*(\S+)\s*$/)
      if (m) {
        entries.push({ spec: m[1], patchPath: m[2] })
        continue
      }
      // Any non-indented, non-empty line ends the block.
      if (line.trim() !== '' && !/^\s/.test(line)) inBlock = false
    }
  }
  return entries
}

/** Extract, per patched file, the most distinctive added line.
 * "Most distinctive" = the longest addition: long lines (real statements) are
 * far less likely to coincidentally exist in the unpatched file than short
 * ones like `}` or `continue;`. Files whose hunks only REMOVE lines are
 * skipped — absence can't be asserted safely (the removed text may legally
 * occur elsewhere in the file). */
function extractMarkers(patchFile) {
  const text = fs.readFileSync(patchFile, 'utf8')
  const markers = []
  let currentFile = null
  let bestLine = ''
  const flush = () => {
    if (currentFile && bestLine.trim().length > 0) {
      markers.push({ file: currentFile, line: bestLine })
    }
    bestLine = ''
  }
  for (const raw of text.split('\n')) {
    const fileHeader = raw.match(/^\+\+\+ b\/(.+)$/)
    if (fileHeader) {
      flush()
      currentFile = fileHeader[1]
      continue
    }
    if (currentFile && raw.startsWith('+') && !raw.startsWith('+++')) {
      const added = raw.slice(1)
      if (added.trim().length > bestLine.trim().length) bestLine = added
    }
  }
  flush()
  return markers
}

/** Locate the installed package dir for a patched spec ("name@version").
 * Scoped names use pnpm's `+` encoding in the .pnpm directory. The glob over
 * `_patch_hash=` suffixes is intentional: we don't recompute the hash — the
 * whole point is to distrust it and read the content instead. */
function findInstalledDir(spec) {
  const at = spec.lastIndexOf('@')
  const name = spec.slice(0, at)
  const encoded = name.replace(/\//g, '+')
  const pnpmDir = path.join(repoRoot, 'node_modules', '.pnpm')
  if (!fs.existsSync(pnpmDir)) return null
  const prefix = `${encoded}@${spec.slice(at + 1)}_patch_hash=`
  const match = fs.readdirSync(pnpmDir).find((d) => d.startsWith(prefix))
  return match ? path.join(pnpmDir, match, 'node_modules', name) : null
}

const failures = []
for (const { spec, patchPath } of readPatchedDependencies()) {
  const installedDir = findInstalledDir(spec)
  if (!installedDir) {
    failures.push(`${spec}: no _patch_hash directory found in node_modules/.pnpm (patch not applied at all?)`)
    continue
  }
  for (const { file, line } of extractMarkers(path.join(repoRoot, patchPath))) {
    const target = path.join(installedDir, file)
    if (!fs.existsSync(target)) {
      failures.push(`${spec}: patched file missing: ${file}`)
      continue
    }
    if (!fs.readFileSync(target, 'utf8').includes(line)) {
      failures.push(`${spec}: ${file} does not contain the patch's added code — directory is named as patched but holds unpatched content`)
    }
  }
}

if (failures.length > 0) {
  console.error('✖ patched-dependency verification failed:\n')
  for (const f of failures) console.error(`  - ${f}`)
  console.error(
    '\nThe pnpm cache has served stale/corrupted content for a patched package.' +
      '\nRecover with:' +
      '\n  rm -rf node_modules/.pnpm node_modules/.modules.yaml .pnpm-store' +
      '\n  pnpm install' +
      '\n(inside the dev container if you develop via devenv/docker-compose).',
  )
  process.exit(1)
}
console.log('✓ patched dependencies verified against installed content')
