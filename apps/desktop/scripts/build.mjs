import { execFileSync } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')

const marker = process.argv.indexOf('--')
const rawTarget = process.argv[2] && process.argv[2] !== '--' ? process.argv[2] : 'current'
const builderArgs = marker >= 0 ? process.argv.slice(marker + 1) : process.argv.slice(3)
const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'
const qdrantTarget = rawTarget === 'current' ? `${process.platform}-${process.arch}` : rawTarget

function run(command, args, extraEnv = {}) {
  execFileSync(command, args, {
    cwd: desktopRoot,
    stdio: 'inherit',
    env: {
      ...process.env,
      ...extraEnv,
    },
  })
}

run(process.execPath, ['scripts/prepare-qdrant.mjs', `--targets=${qdrantTarget}`])
run(pnpm, ['run', 'prepare:local-server'])
run(pnpm, ['exec', 'electron-vite', 'build'])
run(pnpm, ['exec', 'electron-builder', ...builderArgs], {
  MEMOH_DESKTOP_QDRANT_TARGET: qdrantTarget,
})
