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
const bundleTarget = process.env.MEMOH_DESKTOP_BUNDLE_TARGET || `${process.platform}-${process.arch}`
const [bundlePlatform, bundleArch = process.arch] = bundleTarget.split('-')
const goArch = bundleArch === 'x64' ? 'amd64' : bundleArch

const serverName = bundlePlatform === 'win32' ? 'memoh-server.exe' : 'memoh-server'
const cliName = bundlePlatform === 'win32' ? 'memoh.exe' : 'memoh'
const goBuildFlags = process.env.MEMOH_DESKTOP_KEEP_GO_SYMBOLS
  ? ['build', '-trimpath']
  : ['build', '-trimpath', '-ldflags=-s -w']

function goBuild(outputPath, packagePath, env) {
  execFileSync('go', [...goBuildFlags, '-o', outputPath, packagePath], {
    cwd: repoRoot,
    stdio: 'inherit',
    env: {
      ...process.env,
      ...env,
    },
  })
}

rmSync(serverDir, { recursive: true, force: true })
rmSync(cliDir, { recursive: true, force: true })
rmSync(runtimeDir, { recursive: true, force: true })
rmSync(providersDir, { recursive: true, force: true })
mkdirSync(serverDir, { recursive: true })
mkdirSync(cliDir, { recursive: true })
mkdirSync(runtimeDir, { recursive: true })
mkdirSync(configDir, { recursive: true })
mkdirSync(providersDir, { recursive: true })

goBuild(resolve(serverDir, serverName), './cmd/agent', {
  GOOS: bundlePlatform === 'win32' ? 'windows' : bundlePlatform,
  GOARCH: goArch,
})

// CLI binary ships next to the server inside the app bundle. CLI uses
// os.Executable() to locate its own dir then walks up to find the
// sibling server binary — see internal/tui/local/paths.go.
goBuild(resolve(cliDir, cliName), './cmd/memoh', {
  GOOS: bundlePlatform === 'win32' ? 'windows' : bundlePlatform,
  GOARCH: goArch,
})

goBuild(resolve(runtimeDir, 'bridge'), './cmd/bridge', {
  GOOS: 'linux',
  GOARCH: goArch,
})
cpSync(resolve(repoRoot, 'cmd', 'bridge', 'template'), resolve(runtimeDir, 'templates'), { recursive: true })

copyFileSync(resolve(repoRoot, 'conf', 'app.local.toml'), resolve(configDir, 'app.local.toml'))
cpSync(resolve(repoRoot, 'conf', 'providers'), providersDir, { recursive: true })

console.log(`Prepared desktop local server resources in ${resourcesRoot}`)
