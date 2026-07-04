import { createRequire } from 'node:module'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { spawnSync } from 'node:child_process'

const require = createRequire(import.meta.url)

function resolveElectronModuleDir() {
  return dirname(require.resolve('electron/package.json'))
}

function electronBinaryExists(moduleDir) {
  const pathFile = join(moduleDir, 'path.txt')
  if (!existsSync(pathFile)) {
    return false
  }

  const executablePath = readFileSync(pathFile, 'utf8').trim()
  if (!executablePath) {
    return false
  }

  return existsSync(join(moduleDir, 'dist', executablePath))
}

function installElectron(moduleDir) {
  const result = spawnSync(process.execPath, [join(moduleDir, 'install.js')], {
    stdio: 'inherit',
    env: process.env,
  })

  if (result.status !== 0) {
    throw new Error(`Electron install failed with exit code ${result.status ?? 'unknown'}`)
  }
}

const electronModuleDir = resolveElectronModuleDir()

if (!electronBinaryExists(electronModuleDir)) {
  console.log('Electron binary is missing; preparing local Electron runtime...')
  installElectron(electronModuleDir)
}

if (!electronBinaryExists(electronModuleDir)) {
  throw new Error('Electron binary is still missing after install')
}
