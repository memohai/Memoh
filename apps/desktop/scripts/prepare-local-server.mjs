import { execFileSync } from 'node:child_process'
import { cpSync, copyFileSync, mkdirSync, rmSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')
const repoRoot = resolve(desktopRoot, '..', '..')
const resourcesRoot = resolve(desktopRoot, 'resources')
const serverDir = resolve(resourcesRoot, 'server')
const cliDir = resolve(resourcesRoot, 'cli')
const runtimeDir = resolve(resourcesRoot, 'runtime')
const configDir = resolve(resourcesRoot, 'config')
const providersDir = resolve(resourcesRoot, 'providers')

const serverName = process.platform === 'win32' ? 'memoh-server.exe' : 'memoh-server'
const cliName = process.platform === 'win32' ? 'memoh.exe' : 'memoh'
const dockerBridgeArch = process.arch === 'x64' ? 'amd64' : process.arch

rmSync(serverDir, { recursive: true, force: true })
rmSync(cliDir, { recursive: true, force: true })
rmSync(runtimeDir, { recursive: true, force: true })
rmSync(providersDir, { recursive: true, force: true })
mkdirSync(serverDir, { recursive: true })
mkdirSync(cliDir, { recursive: true })
mkdirSync(runtimeDir, { recursive: true })
mkdirSync(configDir, { recursive: true })
mkdirSync(providersDir, { recursive: true })

execFileSync('go', ['build', '-o', resolve(serverDir, serverName), './cmd/agent'], {
  cwd: repoRoot,
  stdio: 'inherit',
})

// CLI binary ships next to the server inside the app bundle. CLI uses
// os.Executable() to locate its own dir then walks up to find the
// sibling server binary — see internal/tui/local/paths.go.
execFileSync('go', ['build', '-o', resolve(cliDir, cliName), './cmd/memoh'], {
  cwd: repoRoot,
  stdio: 'inherit',
})

execFileSync('go', ['build', '-o', resolve(runtimeDir, 'bridge'), './cmd/bridge'], {
  cwd: repoRoot,
  stdio: 'inherit',
  env: {
    ...process.env,
    GOOS: 'linux',
    GOARCH: dockerBridgeArch,
  },
})
cpSync(resolve(repoRoot, 'cmd', 'bridge', 'template'), resolve(runtimeDir, 'templates'), { recursive: true })

copyFileSync(resolve(repoRoot, 'conf', 'app.local.toml'), resolve(configDir, 'app.local.toml'))
cpSync(resolve(repoRoot, 'conf', 'providers'), providersDir, { recursive: true })

console.log(`Prepared desktop local server resources in ${resourcesRoot}`)
