import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')
const xcodeDeveloperDirCandidates = [
  process.env.DEVELOPER_DIR,
  '/Applications/Xcode_26.4.1.app/Contents/Developer',
  '/Applications/Xcode_26.4.app/Contents/Developer',
  '/Applications/Xcode_26.3.app/Contents/Developer',
  '/Applications/Xcode_26.2.app/Contents/Developer',
  '/Applications/Xcode_26.1.1.app/Contents/Developer',
  '/Applications/Xcode_26.1.app/Contents/Developer',
  '/Applications/Xcode_26.0.1.app/Contents/Developer',
  '/Applications/Xcode_26.0.app/Contents/Developer',
  '/Applications/Xcode.app/Contents/Developer',
].filter(Boolean)

const xcodeDeveloperDir = xcodeDeveloperDirCandidates.find((candidate) => (
  existsSync(resolve(candidate, 'usr/bin/actool'))
))

const marker = process.argv.indexOf('--')
const rawTarget = process.argv[2] && process.argv[2] !== '--' ? process.argv[2] : 'current'
const builderArgs = marker >= 0 ? process.argv.slice(marker + 1) : process.argv.slice(3)
const qdrantTarget = rawTarget === 'current' ? `${process.platform}-${process.arch}` : rawTarget
const macToolchainEnv = process.platform === 'darwin' && xcodeDeveloperDir
  ? { DEVELOPER_DIR: xcodeDeveloperDir }
  : {}

function quoteWindowsArg(value) {
  if (/^[A-Za-z0-9_/:=.,+\-]+$/.test(value)) {
    return value
  }
  return `"${value.replaceAll('"', '\\"')}"`
}

function runPnpm(args, extraEnv = {}) {
  if (process.platform === 'win32') {
    run('cmd.exe', ['/d', '/s', '/c', ['pnpm', ...args].map(quoteWindowsArg).join(' ')], extraEnv)
    return
  }
  run('pnpm', args, extraEnv)
}

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
runPnpm(['run', 'prepare:local-server'])
runPnpm(['exec', 'electron-vite', 'build'])
runPnpm(['exec', 'electron-builder', ...builderArgs], {
  ...macToolchainEnv,
  MEMOH_DESKTOP_QDRANT_TARGET: qdrantTarget,
})
