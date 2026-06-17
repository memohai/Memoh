#!/usr/bin/env node

import { spawnSync } from 'node:child_process'

const env = {
  ...process.env,
  MEMOH_DESKTOP_RUNTIME_MODE: 'remote',
}

if (!env.MEMOH_DESKTOP_REMOTE_BASE_URL?.trim()) {
  env.MEMOH_DESKTOP_REMOTE_BASE_URL = 'http://localhost:18080'
}

console.log(`desktop remote dev api=${env.MEMOH_DESKTOP_REMOTE_BASE_URL}`)

const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'
const result = spawnSync(pnpm, ['exec', 'electron-vite', 'dev'], {
  stdio: 'inherit',
  env,
})

if (result.error) {
  console.error(result.error.message)
  process.exit(1)
}

process.exit(result.status ?? 1)
