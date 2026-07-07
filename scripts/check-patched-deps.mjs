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
        // pnpm quotes keys that start with @ ('@scope/pkg@1.0.0': …) — strip
        // the quotes, or the spec never matches any .pnpm directory name and
        // every install fails the moment the first scoped patch lands.
        entries.push({
          spec: m[1].replace(/^['"]|['"]$/g, ''),
          patchPath: m[2].replace(/^['"]|['"]$/g, ''),
        })
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
  // Versionless specs are legal pnpm keys: `pkg` has no `@` at all and
  // `@scope/pkg` has its only `@` at index 0 — in both cases the whole spec
  // is the name and ANY version may carry the patch.
  const at = spec.lastIndexOf('@')
  const name = at > 0 ? spec.slice(0, at) : spec
  const encoded = name.replace(/\//g, '+')
  const pnpmDir = path.join(repoRoot, 'node_modules', '.pnpm')
  if (!fs.existsSync(pnpmDir)) return []
  const prefix = at > 0 ? `${encoded}@${spec.slice(at + 1)}_patch_hash=` : `${encoded}@`
  return fs
    .readdirSync(pnpmDir)
    .filter((d) => d.startsWith(prefix) && d.includes('_patch_hash='))
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
    let markers
    try {
      markers = extractMarkers(path.join(repoRoot, patchPath))
    } catch (err) {
      failures.push(`${spec}: cannot read patch file ${patchPath}: ${err.message}`)
      continue
    }
    // Refuse to certify what we cannot check: a patch that yields no
    // verifiable added-line marker (corrupted file, exotic diff shape) must
    // fail loudly — passing on directory presence alone would hollow out the
    // "exit 0 = installed content carries the patch" invariant.
    const verifiable = markers.filter((m) => m.added.trim())
    if (verifiable.length === 0) {
      failures.push(`${spec}: no verifiable added-line marker could be extracted from ${patchPath} — refusing to certify the install`)
      continue
    }
    for (const installedDir of installedDirs) {
      for (const { file, added } of verifiable) {
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
    // Unreadable patch → verifyInstalled already failed the run with a clear
    // message; the sweep just skips it instead of crashing twice.
    let extracted
    try {
      extracted = extractMarkers(path.join(repoRoot, patchPath))
    } catch {
      continue
    }
    for (const { removed } of extracted) {
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
      // The .vite dirs sit on the bind mount, shared by host vite, container
      // vite and desktop electron-vite: another process may rotate temp files
      // mid-scan. Transient IO errors (ENOENT, broken symlink named *.js)
      // must skip the entry, not crash the guard — the sweep is a self-heal
      // step, so a file it couldn't read is simply not evidence of staleness.
      let dirEntries
      try {
        dirEntries = fs.readdirSync(dir, { withFileTypes: true })
      } catch {
        continue
      }
      for (const entry of dirEntries) {
        const p = path.join(dir, entry.name)
        if (entry.isDirectory()) {
          stack.push(p)
        } else if (entry.name.endsWith('.js')) {
          let content
          try {
            content = squash(fs.readFileSync(p, 'utf8'))
          } catch {
            continue
          }
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
      '\n  rm -rf node_modules/.pnpm node_modules/.pnpm-store node_modules/.modules.yaml \\' +
      '\n         node_modules/.vite apps/*/node_modules/.vite .pnpm-store' +
      '\n  pnpm install' +
      '\n(inside the dev container if you develop via devenv/docker-compose;' +
      '\nfor Docker image builds the pnpm store is a BuildKit cache mount —' +
      '\nclear it with `docker builder prune` instead).' +
      '\nnode_modules/.pnpm-store must go too: the poison lives in the STORE' +
      '\nentry, so reinstalling from the same store just re-links the bad copy.' +
      "\nClearing the .vite dirs matters: the dep optimizer's cache key is" +
      '\nthe lockfile, which a cache-poisoning incident does NOT change, so a' +
      '\nbundle built from the poisoned install would otherwise be served even' +
      '\nafter the reinstall fixes node_modules.',
  )
  process.exit(1)
}

// The sweep is a self-heal step, not a certification: an unexpected error
// here (permissions, races on the shared bind mount) downgrades to a warning
// instead of failing a dev whose INSTALL was just verified healthy above.
let sweptDirs = []
try {
  sweptDirs = sweepStaleViteCaches()
} catch (err) {
  console.warn(`⚠ stale dep-optimizer cache sweep skipped: ${err.message}`)
}
for (const { cacheDir, spec } of sweptDirs) {
  console.warn(
    `⚠ removed stale dep-optimizer cache ${path.relative(repoRoot, cacheDir)} — ` +
      `it still contained pre-patch code of ${spec} (built before the patch was applied); ` +
      'vite will re-optimize on next start',
  )
}
console.log('✓ patched dependencies verified against installed content')
