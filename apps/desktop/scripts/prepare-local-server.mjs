import { execFileSync } from 'node:child_process'
import { copyFileSync, mkdirSync, rmSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')
const repoRoot = resolve(desktopRoot, '..', '..')
const resourcesRoot = resolve(desktopRoot, 'resources')
const serverDir = resolve(resourcesRoot, 'server')
const runtimeDir = resolve(resourcesRoot, 'runtime')
const configDir = resolve(resourcesRoot, 'config')

const serverName = process.platform === 'win32' ? 'memoh-server.exe' : 'memoh-server'

rmSync(serverDir, { recursive: true, force: true })
rmSync(runtimeDir, { recursive: true, force: true })
mkdirSync(serverDir, { recursive: true })
mkdirSync(runtimeDir, { recursive: true })
mkdirSync(configDir, { recursive: true })

execFileSync('go', ['build', '-o', resolve(serverDir, serverName), './cmd/agent'], {
  cwd: repoRoot,
  stdio: 'inherit',
})

execFileSync('go', ['build', '-o', resolve(runtimeDir, 'bridge'), './cmd/bridge'], {
  cwd: repoRoot,
  stdio: 'inherit',
})

copyFileSync(resolve(repoRoot, 'conf', 'app.local.toml'), resolve(configDir, 'app.local.toml'))

console.log(`Prepared desktop local server resources in ${resourcesRoot}`)
