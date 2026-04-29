#!/usr/bin/env node
import { execFileSync, spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const CANDIDATE_DIRS = [
  'apps/browser',
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

function publish(dir) {
  const result = spawnSync(
    'pnpm',
    ['publish', '--access', 'public', '--no-git-checks'],
    { cwd: dir, stdio: 'inherit' },
  )
  return result.status === 0
}

let published = 0
let skipped = 0
let failed = 0

for (const dir of CANDIDATE_DIRS) {
  let pkg
  try {
    pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'))
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
  if (publish(dir)) {
    log.ok(`${name}@${version}`)
    published++
  } else {
    log.fail(`${name}@${version}`)
    failed++
  }
}

console.log(`\nSummary: ${published} published, ${skipped} skipped, ${failed} failed`)
process.exit(failed > 0 ? 1 : 0)
