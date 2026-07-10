#!/usr/bin/env node

import { existsSync, readdirSync, rmSync, rmdirSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const uiPath = join(root, 'packages', 'ui')
const uiPackage = join(uiPath, 'package.json')
const checkOnly = process.argv.includes('--check')

function instruction() {
  return 'Run `mise run submodule-init` or `git submodule update --init --recursive`, then run the command again.'
}

function fail(message) {
  console.error(`\nError: ${message}`)
  console.error(instruction())
  process.exit(1)
}

function git(args, { optional = false } = {}) {
  const result = spawnSync('git', args, { cwd: root, stdio: optional ? 'ignore' : 'inherit' })
  if (!optional && result.status !== 0)
    fail('Git could not initialize the packages/ui submodule.')
  return result.status === 0
}

if (!existsSync(join(root, '.gitmodules')))
  process.exit(0)

if (checkOnly) {
  if (!existsSync(uiPackage))
    fail('packages/ui is not initialized, so the @felinic/ui workspace is missing.')
  process.exit(0)
}

if (!existsSync(uiPackage) && existsSync(uiPath)) {
  const entries = readdirSync(uiPath)
  const safeLegacyEntries = new Set(['node_modules', '.DS_Store'])
  const unexpected = entries.filter(entry => !safeLegacyEntries.has(entry))

  if (unexpected.length > 0)
    fail(`packages/ui contains files that Git must not overwrite: ${unexpected.join(', ')}`)

  for (const entry of entries)
    rmSync(join(uiPath, entry), { recursive: true, force: true })
  rmdirSync(uiPath)
}

console.log('Initializing packages/ui submodule...')
git(['submodule', 'sync', '--recursive'])
git(['submodule', 'update', '--init', '--recursive'])
git(['config', 'submodule.recurse', 'true'], { optional: true })

if (!existsSync(uiPackage))
  fail('packages/ui was not initialized successfully.')

console.log('packages/ui is ready. Future `git pull` commands will recurse into initialized submodules.')
