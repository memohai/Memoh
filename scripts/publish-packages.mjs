#!/usr/bin/env node
import { execFileSync, spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT_DIR = dirname(dirname(fileURLToPath(import.meta.url)))

// packages/ui is a git submodule, but it intentionally stays in this publish
// allowlist. Memoh releases publish the pinned UI package under the Memoh
// release version, so the npm package represents the UI sources selected by
// this host release. If @memohai/ui moves to an independent release cadence,
// remove it here and from .github/workflows/release.yml in the same change.
const CANDIDATE_DIRS = [
  'apps/desktop',
  'apps/web',
  'packages/sdk',
  'packages/ui',
  'packages/icons',
  'packages/config',
]

const log = {
  info: (msg) => console.log(`\x1b[36m[publish]\x1b[0m ${msg}`),
  skip: (msg) => console.log(`\x1b[33m[skip]\x1b[0m ${msg}`),
  ok: (msg) => console.log(`\x1b[32m[ok]\x1b[0m ${msg}`),
  fail: (msg) => console.log(`\x1b[31m[fail]\x1b[0m ${msg}`),
}

const dryRun = process.argv.includes('--dry-run') || process.env.PUBLISH_PACKAGES_DRY_RUN === '1'

function prereleaseTag(version) {
  const override = process.env.NPM_PUBLISH_TAG?.trim()
  if (override) {
    return override
  }

  const match = version.match(/^\d+\.\d+\.\d+-([^+]+)(?:\+.+)?$/)
  if (!match) {
    return null
  }

  const [identifier] = match[1].split('.')
  return /^[a-z][a-z0-9._-]*$/i.test(identifier) ? identifier : 'next'
}

function publishOptions(version) {
  const args = ['--access', 'public', '--no-git-checks']
  const tag = prereleaseTag(version)
  if (tag) {
    args.push('--tag', tag)
  }
  return args
}

function isVersionPublished(name, version) {
  try {
    const out = execFileSync('npm', ['view', `${name}@${version}`, 'version'], {
      stdio: ['ignore', 'pipe', 'ignore'],
      encoding: 'utf8',
    }).trim()
    return out === version
  } catch {
    return false
  }
}

function publish(dir, version) {
  const args = ['publish', dir, ...publishOptions(version)]
  if (dryRun) {
    log.info(`[dry-run] ${dir}: pnpm ${args.join(' ')}`)
    return true
  }

  const result = spawnSync(
    'pnpm',
    args,
    { cwd: ROOT_DIR, stdio: 'inherit' },
  )
  return result.status === 0
}

let published = 0
let skipped = 0
let failed = 0

for (const dir of CANDIDATE_DIRS) {
  let pkg
  try {
    pkg = JSON.parse(readFileSync(join(ROOT_DIR, dir, 'package.json'), 'utf8'))
  } catch {
    log.skip(`${dir} (no package.json)`)
    skipped++
    continue
  }

  const { name, version, private: isPrivate } = pkg

  if (isPrivate) {
    log.skip(`${name} (private)`)
    skipped++
    continue
  }

  if (isVersionPublished(name, version)) {
    log.skip(`${name}@${version} (already on registry)`)
    skipped++
    continue
  }

  log.info(`${name}@${version}`)
  if (publish(dir, version)) {
    log.ok(`${name}@${version}`)
    published++
  } else {
    log.fail(`${name}@${version}`)
    failed++
  }
}

console.log(`\nSummary: ${published} published, ${skipped} skipped, ${failed} failed`)
process.exit(failed > 0 ? 1 : 0)
