#!/usr/bin/env node
/**
 * check-patched-deps — verify that pnpm's patchedDependencies were actually
 * applied, and heal caches that still carry pre-patch code.
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
 * TWO CHECKS, ONE SCRIPT:
 *
 * 1. VERIFY (fail loud): for every entry in pnpm-workspace.yaml
 *    patchedDependencies, parse the .patch file and assert each patched
 *    file's most distinctive ADDED line is present in the installed copy
 *    under node_modules/.pnpm/<pkg>_patch_hash=<any>/node_modules/<pkg>/.
 *    Content is the ground truth; the directory name is not trusted.
 *
 * 2. SWEEP (self-heal): Vite's dep-optimizer cache key is the LOCKFILE,
 *    which a cache-poisoning incident does not change — so a bundle built
 *    from a poisoned install keeps being served even after node_modules is
 *    fixed. For each patch, take its most distinctive REMOVED line (code
 *    that must no longer exist anywhere post-patch) and scan the optimizer
 *    caches; a hit means the cache was built from unpatched code, so the
 *    whole cache dir is deleted (vite just re-optimizes on next start).
 *    Matching is whitespace-insensitive because esbuild reprints code; dev
 *    prebundles are not minified, so identifier/property text survives.
 *
 * ENTRY POINTS — both must stay wired or a poisoned state can slip through:
 *  - root `postinstall`: catches poisoned installs at install time (also
 *    covers the production web image build, which COPYs this script).
 *  - apps/web vite plugin (configResolved): catches the pull-without-install
 *    path — the lockfile rarely changes, so a colleague may never trigger an
 *    install, but every dev/build run starts vite.
 *
 * Dependency-free by design: it runs as postinstall, i.e. it cannot rely on
 * the very node_modules it validates (hence the YAML line-scanner too).
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

/** Parse the patchedDependencies block of pnpm-workspace.yaml.
 * Deliberately a line-scanner, not a YAML parser (see header). The block's
 * shape is a flat `  name@version: path` map. */
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

/** Extract, per patched file, the most distinctive added AND removed line.
 * "Most distinctive" = the longest: long lines (real statements) are far less
 * likely to coincidentally exist elsewhere than short ones like `}`.
 *  - added → must EXIST in a correctly patched install (verify).
 *  - removed → must NOT exist in anything built from patched code (sweep).
 * Files whose hunks only remove lines get no `added` marker — presence can't
 * be asserted; their `removed` marker still feeds the sweep. */
function extractMarkers(patchFile) {
  const text = fs.readFileSync(patchFile, 'utf8')
  const markers = []
  let current = null
  const flush = () => {
    if (current && (current.added || current.removed)) markers.push(current)
    current = null
  }
  for (const raw of text.split('\n')) {
    const fileHeader = raw.match(/^\+\+\+ b\/(.+)$/)
    if (fileHeader) {
      flush()
      current = { file: fileHeader[1], added: '', removed: '' }
      continue
    }
    if (!current) continue
    if (raw.startsWith('+') && !raw.startsWith('+++')) {
      const line = raw.slice(1)
      if (line.trim().length > current.added.trim().length) current.added = line
    } else if (raw.startsWith('-') && !raw.startsWith('---')) {
      const line = raw.slice(1)
      if (line.trim().length > current.removed.trim().length) current.removed = line
    }
  }
  flush()
  return markers
}

/** Locate every installed dir for a patched spec ("name@version").
 * Scoped names use pnpm's `+` encoding in the .pnpm directory. Plural on
 * purpose: peer-dependency variants materialize as separate `_patch_hash=…_…`
 * directories and EACH copy must carry the patch. The glob over the hash
 * suffix is intentional: we don't recompute the hash — the whole point is to
 * distrust it and read the content instead. */
function findInstalledDirs(spec) {
  const at = spec.lastIndexOf('@')
  const name = spec.slice(0, at)
  const encoded = name.replace(/\//g, '+')
  const pnpmDir = path.join(repoRoot, 'node_modules', '.pnpm')
  if (!fs.existsSync(pnpmDir)) return []
  const prefix = `${encoded}@${spec.slice(at + 1)}_patch_hash=`
  return fs
    .readdirSync(pnpmDir)
    .filter((d) => d.startsWith(prefix))
    .map((d) => path.join(pnpmDir, d, 'node_modules', name))
}

function verifyInstalled() {
  const failures = []
  for (const { spec, patchPath } of readPatchedDependencies()) {
    const installedDirs = findInstalledDirs(spec)
    if (installedDirs.length === 0) {
      failures.push(`${spec}: no _patch_hash directory found in node_modules/.pnpm (patch not applied at all?)`)
      continue
    }
    const markers = extractMarkers(path.join(repoRoot, patchPath))
    for (const installedDir of installedDirs) {
      for (const { file, added } of markers) {
        if (!added.trim()) continue
        const target = path.join(installedDir, file)
        if (!fs.existsSync(target)) {
          failures.push(`${spec}: patched file missing: ${file}`)
          continue
        }
        if (!fs.readFileSync(target, 'utf8').includes(added)) {
          failures.push(`${spec}: ${file} does not contain the patch's added code — directory is named as patched but holds unpatched content`)
        }
      }
    }
  }
  return failures
}

/** Delete dep-optimizer caches that still contain pre-patch code.
 * Scans every apps/<x>/node_modules/.vite and the root node_modules/.vite
 * (vitest) for whitespace-stripped REMOVED-line markers. Deleting the whole
 * cache dir is the safe action: the optimizer rebuilds it on next start, and
 * a false positive only costs one re-optimization. Markers shorter than 20
 * significant chars are skipped — too generic to trust as evidence. */
function sweepStaleViteCaches() {
  const squash = (s) => s.replace(/\s+/g, '')
  const markers = []
  for (const { spec, patchPath } of readPatchedDependencies()) {
    for (const { removed } of extractMarkers(path.join(repoRoot, patchPath))) {
      const m = squash(removed)
      if (m.length >= 20) markers.push({ spec, m })
    }
  }
  if (markers.length === 0) return []

  const cacheDirs = []
  const appsDir = path.join(repoRoot, 'apps')
  if (fs.existsSync(appsDir)) {
    for (const app of fs.readdirSync(appsDir)) {
      cacheDirs.push(path.join(appsDir, app, 'node_modules', '.vite'))
    }
  }
  cacheDirs.push(path.join(repoRoot, 'node_modules', '.vite'))

  const removedDirs = []
  for (const cacheDir of cacheDirs) {
    if (!fs.existsSync(cacheDir)) continue
    // .vite/deps* holds the optimizer output; scan every .js beneath the
    // cache dir (dev deps, vitest deps) but skip sourcemaps for speed.
    const stack = [cacheDir]
    let stale = null
    while (stack.length > 0 && !stale) {
      const dir = stack.pop()
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const p = path.join(dir, entry.name)
        if (entry.isDirectory()) {
          stack.push(p)
        } else if (entry.name.endsWith('.js')) {
          const content = squash(fs.readFileSync(p, 'utf8'))
          const hit = markers.find(({ m }) => content.includes(m))
          if (hit) {
            stale = { file: p, spec: hit.spec }
            break
          }
        }
      }
    }
    if (stale) {
      fs.rmSync(cacheDir, { recursive: true, force: true })
      removedDirs.push({ cacheDir, ...stale })
    }
  }
  return removedDirs
}

const failures = verifyInstalled()
if (failures.length > 0) {
  console.error('✖ patched-dependency verification failed:\n')
  for (const f of failures) console.error(`  - ${f}`)
  console.error(
    '\nThe pnpm cache has served stale/corrupted content for a patched package.' +
      '\nRecover with:' +
      '\n  rm -rf node_modules/.pnpm node_modules/.modules.yaml .pnpm-store apps/*/node_modules/.vite' +
      '\n  pnpm install' +
      '\n(inside the dev container if you develop via devenv/docker-compose).' +
      "\nClearing apps/*/node_modules/.vite matters: the dep optimizer's cache key is" +
      '\nthe lockfile, which a cache-poisoning incident does NOT change, so a bundle' +
      '\nbuilt from the poisoned install would otherwise be served even after the' +
      '\nreinstall fixes node_modules.',
  )
  process.exit(1)
}

const sweptDirs = sweepStaleViteCaches()
for (const { cacheDir, spec } of sweptDirs) {
  console.warn(
    `⚠ removed stale dep-optimizer cache ${path.relative(repoRoot, cacheDir)} — ` +
      `it still contained pre-patch code of ${spec} (built before the patch was applied); ` +
      'vite will re-optimize on next start',
  )
}
console.log('✓ patched dependencies verified against installed content')
