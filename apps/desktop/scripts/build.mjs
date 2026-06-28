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

const rawArgs = process.argv.slice(2)
const marker = rawArgs.indexOf('--')
const buildOptions = marker >= 0 ? rawArgs.slice(0, marker) : rawArgs
const targetIndex = buildOptions.findIndex(arg => arg && arg !== '--' && !arg.startsWith('--'))
const rawTarget = targetIndex >= 0 ? buildOptions[targetIndex] : 'current'
const builderArgs = marker >= 0
  ? rawArgs.slice(marker + 1)
  : rawArgs.filter((arg, index) => index !== targetIndex && !arg.startsWith('--runtime=') && !arg.startsWith('--flavor='))
const runtimeMode = resolveRuntimeMode(buildOptions)
const bundleTarget = rawTarget === 'current' ? `${process.platform}-${process.arch}` : rawTarget
const gstreamerTarget = resolveGStreamerTarget(bundleTarget)
const macToolchainEnv = process.platform === 'darwin' && xcodeDeveloperDir
  ? { DEVELOPER_DIR: xcodeDeveloperDir }
  : {}

function resolveRuntimeMode(options) {
  const envRuntime = process.env.MEMOH_DESKTOP_RUNTIME_MODE?.trim()
  const envFlavor = process.env.MEMOH_DESKTOP_FLAVOR?.trim()
  const runtimeFlag = options.find(arg => arg.startsWith('--runtime='))?.slice('--runtime='.length).trim()
  const flavorFlag = options.find(arg => arg.startsWith('--flavor='))?.slice('--flavor='.length).trim()
  const value = runtimeFlag || flavorFlag || envRuntime || envFlavor || 'local'
  switch (value) {
    case 'local':
    case 'offline':
      return 'local'
    case 'remote':
    case 'online':
      return 'remote'
    default:
      throw new Error(`Unsupported desktop runtime mode: ${value}`)
  }
}

function resolveGStreamerTarget(target) {
  if (target.startsWith('darwin-')) {
    return 'darwin-universal'
  }
  if (target === 'win32-x64') {
    return 'win32-x64'
  }
  return '__none__'
}

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

if (runtimeMode === 'local') {
  if (gstreamerTarget !== '__none__') {
    run(process.execPath, ['scripts/prepare-gstreamer.mjs', `--targets=${gstreamerTarget}`])
  } else {
    run(process.execPath, ['scripts/prepare-gstreamer.mjs', '--targets=none'])
  }
  runPnpm(['run', 'prepare:local-server'], {
    MEMOH_DESKTOP_BUNDLE_TARGET: bundleTarget,
  })
}
runPnpm(['exec', 'electron-vite', 'build'], {
  MEMOH_DESKTOP_RUNTIME_MODE: runtimeMode,
  MEMOH_DESKTOP_FLAVOR: runtimeMode === 'remote' ? 'online' : 'offline',
})
runPnpm(['exec', 'electron-builder', ...builderConfigArgs(runtimeMode), ...builderArgs], {
  ...macToolchainEnv,
  MEMOH_DESKTOP_RUNTIME_MODE: runtimeMode,
  MEMOH_DESKTOP_FLAVOR: runtimeMode === 'remote' ? 'online' : 'offline',
  MEMOH_DESKTOP_GSTREAMER_TARGET: gstreamerTarget,
})

function builderConfigArgs(targetRuntimeMode) {
  if (targetRuntimeMode === 'remote') {
    return ['--config', 'electron-builder.online.yml']
  }
  return []
}
