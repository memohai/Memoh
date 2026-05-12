import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, cpSync, createWriteStream, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, resolve } from 'node:path'
import { pipeline } from 'node:stream/promises'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')
const gstreamerRoot = resolve(desktopRoot, 'resources', 'gstreamer')

const defaultVersion = '1.28.2'
const gstreamerVersion = process.env.GSTREAMER_VERSION || defaultVersion
const downloadBaseURL = process.env.GSTREAMER_DOWNLOAD_BASE_URL?.replace(/\/+$/, '')
const macOSRuntimePackages = [
  'base-system-',
  'base-crypto-',
  'gstreamer-1.0-core-',
  'gstreamer-1.0-codecs-',
  'gstreamer-1.0-codecs-gpl-restricted-',
  'gstreamer-1.0-net-',
]
const displayPluginNames = new Set([
  'libgstcoreelements.dylib',
  'libgstvideoconvertscale.dylib',
  'libgstvideorate.dylib',
  'libgstrfbsrc.dylib',
  'libgstx264.dylib',
  'libgstvideoparsersbad.dylib',
  'libgstrtp.dylib',
  'libgstrtpmanager.dylib',
  'libgstudp.dylib',
  'libgstvpx.dylib',
  'libgstjpeg.dylib',
  'libgsttypefindfunctions.dylib',
  'gstcoreelements.dll',
  'gstvideoconvertscale.dll',
  'gstvideorate.dll',
  'gstrfbsrc.dll',
  'gstx264.dll',
  'gstvideoparsersbad.dll',
  'gstrtp.dll',
  'gstrtpmanager.dll',
  'gstudp.dll',
  'gstvpx.dll',
  'gstjpeg.dll',
  'gsttypefindfunctions.dll',
])

const targetSpecs = {
  'darwin-universal': {
    asset: `gstreamer-1.0-${gstreamerVersion}-universal.pkg`,
    binary: 'bin/gst-launch-1.0',
    inspect: 'bin/gst-inspect-1.0',
    scanner: 'libexec/gstreamer-1.0/gst-plugin-scanner',
    kind: 'macos-pkg',
    officialPath: `osx/${gstreamerVersion}`,
  },
  'win32-x64': {
    asset: `gstreamer-1.0-msvc-x86_64-${gstreamerVersion}.exe`,
    binary: 'bin/gst-launch-1.0.exe',
    inspect: 'bin/gst-inspect-1.0.exe',
    scanner: 'libexec/gstreamer-1.0/gst-plugin-scanner.exe',
    kind: 'windows-nsis',
    officialPath: `windows/${gstreamerVersion}/msvc`,
  },
}

function currentTarget() {
  if (process.platform === 'darwin') {
    return 'darwin-universal'
  }
  if (process.platform === 'win32' && process.arch === 'x64' && process.env.GSTREAMER_ENABLE_WINDOWS_BUNDLE) {
    return 'win32-x64'
  }
  return null
}

function releasePlatformTargets() {
  const target = currentTarget()
  return target ? [target] : []
}

function parseTargets() {
  const arg = process.argv.find(item => item.startsWith('--targets='))
  const raw = arg?.slice('--targets='.length) || process.env.GSTREAMER_TARGETS || 'current'
  switch (raw) {
    case 'none':
      return []
    case 'all':
      return Object.keys(targetSpecs)
    case 'current':
    case 'release-platform':
    case 'package':
      return releasePlatformTargets()
    default:
      return raw.split(',').map(item => item.trim()).filter(Boolean)
  }
}

function assertSupportedTarget(target) {
  if (targetSpecs[target]) {
    return
  }
  throw new Error(`Unsupported GStreamer target "${target}". Supported targets: ${Object.keys(targetSpecs).join(', ')}`)
}

function versionPath(targetDir) {
  return resolve(targetDir, 'VERSION')
}

function isPrepared(target, spec) {
  const targetDir = resolve(gstreamerRoot, target)
  const binaryPath = resolve(targetDir, spec.binary)
  const markerPath = versionPath(targetDir)
  if (!existsSync(binaryPath) || !existsSync(markerPath)) {
    return false
  }
  try {
    return readFileSync(markerPath, 'utf8').trim() === gstreamerVersion
  } catch {
    return false
  }
}

function assetURL(spec) {
  const base = downloadBaseURL || `https://gstreamer.freedesktop.org/data/pkg/${spec.officialPath}`
  return `${base}/${spec.asset}`
}

function downloadAttempts() {
  const raw = Number.parseInt(process.env.GSTREAMER_DOWNLOAD_ATTEMPTS || '4', 10)
  return Number.isFinite(raw) && raw > 0 ? raw : 4
}

function retryDelayMs(attempt) {
  return Math.min(1000 * 2 ** (attempt - 1), 8000)
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function formatDownloadError(error) {
  if (error instanceof Error) {
    const cause = error.cause instanceof Error ? `: ${error.cause.message}` : ''
    return `${error.message}${cause}`
  }
  return String(error)
}

async function downloadAsset(url, archivePath) {
  let lastError
  const attempts = downloadAttempts()
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      rmSync(archivePath, { force: true })
      const response = await fetch(url, {
        headers: {
          'User-Agent': 'memoh-desktop-gstreamer-preparer',
        },
      })
      if (!response.ok || !response.body) {
        throw new Error(`HTTP ${response.status}`)
      }
      await pipeline(response.body, createWriteStream(archivePath))
      return
    } catch (error) {
      lastError = error
      if (attempt === attempts) {
        break
      }
      console.warn(`GStreamer download failed (${attempt}/${attempts}): ${formatDownloadError(error)}. Retrying...`)
      await sleep(retryDelayMs(attempt))
    }
  }
  throw new Error(`Failed to download ${url}: ${formatDownloadError(lastError)}`)
}

async function fetchText(url) {
  let lastError
  const attempts = downloadAttempts()
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const response = await fetch(url, {
        headers: {
          'User-Agent': 'memoh-desktop-gstreamer-preparer',
        },
      })
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }
      return response.text()
    } catch (error) {
      lastError = error
      if (attempt === attempts) {
        break
      }
      console.warn(`GStreamer checksum download failed (${attempt}/${attempts}): ${formatDownloadError(error)}. Retrying...`)
      await sleep(retryDelayMs(attempt))
    }
  }
  throw new Error(`Failed to download ${url}: ${formatDownloadError(lastError)}`)
}

async function verifyChecksum(url, archivePath) {
  const text = await fetchText(`${url}.sha256sum`)
  const expected = text.trim().split(/\s+/)[0]
  if (!/^[a-f0-9]{64}$/i.test(expected)) {
    throw new Error(`Invalid checksum file for ${basename(archivePath)}`)
  }
  const actual = createHash('sha256').update(readFileSync(archivePath)).digest('hex')
  if (actual.toLowerCase() !== expected.toLowerCase()) {
    throw new Error(`Checksum mismatch for ${basename(archivePath)}: expected ${expected}, got ${actual}`)
  }
}

function extractMacOSPackage(archivePath, targetDir) {
  if (process.platform !== 'darwin') {
    throw new Error('Preparing the macOS GStreamer runtime requires macOS pkgutil/ditto.')
  }

  const extractDir = resolve(tmpdir(), `memoh-gstreamer-pkg-${Date.now()}`)
  const stagingDir = resolve(tmpdir(), `memoh-gstreamer-runtime-${Date.now()}`)
  rmSync(extractDir, { recursive: true, force: true })
  rmSync(stagingDir, { recursive: true, force: true })
  mkdirSync(stagingDir, { recursive: true })

  execFileSync('pkgutil', ['--expand-full', archivePath, extractDir], { stdio: 'inherit' })

  for (const packageName of readdirSync(extractDir).sort()) {
    if (!macOSRuntimePackages.some(prefix => packageName.startsWith(prefix))) {
      continue
    }
    const payload = resolve(extractDir, packageName, 'Payload')
    if (existsSync(payload)) {
      execFileSync('ditto', [payload, stagingDir], { stdio: 'inherit' })
    }
  }

  rmSync(targetDir, { recursive: true, force: true })
  mkdirSync(dirname(targetDir), { recursive: true })
  execFileSync('ditto', [stagingDir, targetDir], { stdio: 'inherit' })
  rmSync(extractDir, { recursive: true, force: true })
  rmSync(stagingDir, { recursive: true, force: true })
}

function pruneDisplayPlugins(targetDir) {
  if (process.env.GSTREAMER_KEEP_ALL_PLUGINS) {
    return
  }
  const pluginDir = resolve(targetDir, 'lib', 'gstreamer-1.0')
  if (!existsSync(pluginDir)) {
    return
  }
  for (const pluginName of readdirSync(pluginDir)) {
    const isPluginLibrary = pluginName.endsWith('.dylib') || pluginName.endsWith('.dll')
    if (isPluginLibrary && !displayPluginNames.has(pluginName)) {
      rmSync(resolve(pluginDir, pluginName), { force: true })
    }
  }
}

function findFile(root, fileName) {
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const child = resolve(root, entry.name)
    if (entry.isFile() && entry.name === fileName) {
      return child
    }
    if (entry.isDirectory()) {
      const nested = findFile(child, fileName)
      if (nested) {
        return nested
      }
    }
  }
  return null
}

function extractWindowsInstaller(archivePath, targetDir, spec) {
  if (process.platform !== 'win32') {
    throw new Error('Preparing the Windows GStreamer runtime requires running the official installer on Windows.')
  }

  const stagingDir = resolve(tmpdir(), `memoh-gstreamer-runtime-${Date.now()}`)
  rmSync(stagingDir, { recursive: true, force: true })
  mkdirSync(stagingDir, { recursive: true })
  execFileSync(archivePath, ['/S', `/D=${stagingDir}`], { stdio: 'inherit' })

  const binary = findFile(stagingDir, basename(spec.binary))
  if (!binary) {
    throw new Error(`Could not find ${spec.binary} after extracting ${basename(archivePath)}`)
  }
  const runtimeRoot = resolve(dirname(binary), '..')
  rmSync(targetDir, { recursive: true, force: true })
  mkdirSync(dirname(targetDir), { recursive: true })
  cpSync(runtimeRoot, targetDir, { recursive: true })
  rmSync(stagingDir, { recursive: true, force: true })
}

function runtimeEnv(targetDir, spec) {
  return {
    ...process.env,
    PATH: `${resolve(targetDir, 'bin')}${process.platform === 'win32' ? ';' : ':'}${process.env.PATH ?? ''}`,
    GST_PLUGIN_PATH_1_0: resolve(targetDir, 'lib', 'gstreamer-1.0'),
    GST_PLUGIN_SYSTEM_PATH_1_0: '',
    GST_PLUGIN_SCANNER: resolve(targetDir, spec.scanner),
  }
}

function validateRuntime(targetDir, spec) {
  const binaryPath = resolve(targetDir, spec.binary)
  if (!existsSync(binaryPath)) {
    throw new Error(`Prepared GStreamer launcher not found: ${binaryPath}`)
  }
  if (process.platform !== 'win32') {
    chmodSync(binaryPath, 0o755)
  }
  const inspectPath = resolve(targetDir, spec.inspect)
  if (!existsSync(inspectPath)) {
    return
  }
  for (const plugin of ['x264enc', 'rfbsrc']) {
    execFileSync(inspectPath, [plugin], {
      stdio: 'ignore',
      env: runtimeEnv(targetDir, spec),
    })
  }
}

async function prepareTarget(target) {
  assertSupportedTarget(target)
  const spec = targetSpecs[target]
  const targetDir = resolve(gstreamerRoot, target)
  if (!process.env.GSTREAMER_FORCE_DOWNLOAD && isPrepared(target, spec)) {
    console.log(`GStreamer ${gstreamerVersion} already prepared for ${target}`)
    return
  }

  mkdirSync(gstreamerRoot, { recursive: true })
  const url = assetURL(spec)
  const archivePath = resolve(tmpdir(), `${gstreamerVersion}-${spec.asset}`)
  console.log(`Downloading GStreamer ${gstreamerVersion} for ${target}`)
  await downloadAsset(url, archivePath)
  await verifyChecksum(url, archivePath)

  if (spec.kind === 'macos-pkg') {
    extractMacOSPackage(archivePath, targetDir)
  } else if (spec.kind === 'windows-nsis') {
    extractWindowsInstaller(archivePath, targetDir, spec)
  } else {
    throw new Error(`Unsupported GStreamer package kind: ${spec.kind}`)
  }

  pruneDisplayPlugins(targetDir)
  validateRuntime(targetDir, spec)
  writeFileSync(versionPath(targetDir), `${gstreamerVersion}\n`, 'utf8')
  rmSync(archivePath, { force: true })
  console.log(`Prepared GStreamer for ${target} in ${targetDir}`)
}

const targets = [...new Set(parseTargets())]
mkdirSync(gstreamerRoot, { recursive: true })
if (targets.length === 0) {
  console.warn(`No bundled GStreamer runtime target for ${process.platform}-${process.arch}; desktop will use system GStreamer if available.`)
}
for (const target of targets) {
  await prepareTarget(target)
}
