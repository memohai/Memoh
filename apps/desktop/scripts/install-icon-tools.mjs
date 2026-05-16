#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const iconToolsDir = resolve(__dirname, '../icon-tools')
const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'

const child = spawn(pnpm, ['install', '--ignore-workspace'], {
  cwd: iconToolsDir,
  env: {
    ...process.env,
    SHARP_IGNORE_GLOBAL_LIBVIPS: '1',
  },
  stdio: 'inherit',
})

child.on('error', (error) => {
  console.error(error)
  process.exit(1)
})

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
  }

  process.exit(code ?? 1)
})
