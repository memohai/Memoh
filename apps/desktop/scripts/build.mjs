import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const desktopRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')

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

const rawArgs = process.argv.slice(2)
const marker = rawArgs.indexOf('--')
const builderArgs = marker >= 0
  ? rawArgs.slice(marker + 1)
  : rawArgs.filter(arg => arg !== 'current' && !/^(darwin|linux|win32)-/.test(arg))
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

runPnpm(['exec', 'electron-vite', 'build'])
runPnpm(['exec', 'electron-builder', ...builderArgs], macToolchainEnv)
