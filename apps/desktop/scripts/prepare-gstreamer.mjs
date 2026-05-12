import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, cpSync, createWriteStream, existsSync, lstatSync, mkdirSync, readdirSync, readFileSync, readlinkSync, rmSync, writeFileSync } from 'node:fs'
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
  'libgstudp.dylib',
  'libgstvpx.dylib',
  'libgstjpeg.dylib',
  'gstcoreelements.dll',
  'gstvideoconvertscale.dll',
  'gstvideorate.dll',
  'gstrfbsrc.dll',
  'gstx264.dll',
  'gstvideoparsersbad.dll',
  'gstrtp.dll',
  'gstudp.dll',
  'gstvpx.dll',
  'gstjpeg.dll',
])
const displayInspectionElements = [
  'rfbsrc',
  'videoconvert',
  'videoscale',
  'videorate',
  'queue',
  'capsfilter',
  'x264enc',
  'h264parse',
  'rtph264pay',
  'udpsink',
  'vp8enc',
  'rtpvp8pay',
  'jpegenc',
  'filesink',
]

const runtimeProfile = `display-${createHash('sha256')
  .update(JSON.stringify({
    packages: [...macOSRuntimePackages].sort(),
    plugins: [...displayPluginNames].sort(),
    elements: [...displayInspectionElements].sort(),
  }))
  .digest('hex')
  .slice(0, 10)}`

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

function versionMarker() {
  return `${gstreamerVersion}\nprofile=${runtimeProfile}\n`
}

function isPrepared(target, spec) {
  const targetDir = resolve(gstreamerRoot, target)
  const binaryPath = resolve(targetDir, spec.binary)
  const markerPath = versionPath(targetDir)
  if (!existsSync(binaryPath) || !existsSync(markerPath)) {
    return false
  }
  try {
    return readFileSync(markerPath, 'utf8') === versionMarker()
  } catch {
    return false
  }
}

function preparedVersion(targetDir) {
  try {
    return readFileSync(versionPath(targetDir), 'utf8').split(/\r?\n/, 1)[0].trim()
  } catch {
    return null
  }
}

function canReuseExistingRuntime(targetDir, spec) {
  return existsSync(resolve(targetDir, spec.binary)) && preparedVersion(targetDir) === gstreamerVersion
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

function dependencyName(line, targetDir) {
  const match = line.trim().match(/^(\S+)\s/)
  if (!match) {
    return null
  }
  const dependency = match[1]
  if (dependency.startsWith('/usr/lib/') || dependency.startsWith('/System/Library/')) {
    return null
  }
  if (
    dependency.startsWith('@rpath/')
    || dependency.startsWith('@loader_path/')
    || dependency.startsWith('@executable_path/')
    || dependency.startsWith(targetDir)
  ) {
    return basename(dependency)
  }
  return null
}

function syncDarwinSymlinkClosure(libDir, neededLibraries) {
  let changed = true
  while (changed) {
    changed = false
    for (const entry of readdirSync(libDir)) {
      const entryPath = resolve(libDir, entry)
      let stat
      try {
        stat = lstatSync(entryPath)
      } catch {
        continue
      }
      if (!stat.isSymbolicLink()) {
        continue
      }
      const targetName = basename(resolve(dirname(entryPath), readlinkSync(entryPath)))
      if (neededLibraries.has(entry) && !neededLibraries.has(targetName)) {
        neededLibraries.add(targetName)
        changed = true
      }
      if (neededLibraries.has(targetName) && !neededLibraries.has(entry)) {
        neededLibraries.add(entry)
        changed = true
      }
    }
  }
}

function collectDarwinLibraryDependencies(targetDir, seedPaths) {
  const libDir = resolve(targetDir, 'lib')
  const neededLibraries = new Set()
  const queue = seedPaths.filter(existsSync)

  for (let index = 0; index < queue.length; index += 1) {
    const binaryPath = queue[index]
    const output = execFileSync('otool', ['-L', binaryPath], { encoding: 'utf8' })
    for (const line of output.split(/\r?\n/).slice(1)) {
      const name = dependencyName(line, targetDir)
      if (!name || neededLibraries.has(name)) {
        continue
      }
      const libraryPath = resolve(libDir, name)
      if (!existsSync(libraryPath)) {
        continue
      }
      neededLibraries.add(name)
      queue.push(libraryPath)
    }
  }

  syncDarwinSymlinkClosure(libDir, neededLibraries)
  return neededLibraries
}

function pruneDarwinLibraries(targetDir, spec) {
  if (process.env.GSTREAMER_KEEP_ALL_LIBS) {
    return
  }
  if (process.platform !== 'darwin') {
    return
  }

  const libDir = resolve(targetDir, 'lib')
  const pluginDir = resolve(libDir, 'gstreamer-1.0')
  const seedPaths = [
    resolve(targetDir, spec.binary),
    resolve(targetDir, spec.inspect),
    resolve(targetDir, spec.scanner),
  ]
  if (existsSync(pluginDir)) {
    for (const pluginName of readdirSync(pluginDir)) {
      if (pluginName.endsWith('.dylib')) {
        seedPaths.push(resolve(pluginDir, pluginName))
      }
    }
  }

  const neededLibraries = collectDarwinLibraryDependencies(targetDir, seedPaths)
  for (const entry of readdirSync(libDir)) {
    if (entry === 'gstreamer-1.0') {
      continue
    }
    const entryPath = resolve(libDir, entry)
    if (entry.endsWith('.dylib')) {
      if (!neededLibraries.has(entry)) {
        rmSync(entryPath, { force: true })
      }
      continue
    }
    rmSync(entryPath, { recursive: true, force: true })
  }
}

function pruneRuntimeLayout(targetDir, spec) {
  if (process.env.GSTREAMER_KEEP_FULL_LAYOUT) {
    return
  }

  const topLevelKeeps = new Set(['bin', 'lib', 'libexec', 'VERSION'])
  for (const entry of readdirSync(targetDir)) {
    if (!topLevelKeeps.has(entry)) {
      rmSync(resolve(targetDir, entry), { recursive: true, force: true })
    }
  }

  const binDir = resolve(targetDir, 'bin')
  const binKeeps = new Set([basename(spec.binary), basename(spec.inspect)])
  if (existsSync(binDir)) {
    for (const entry of readdirSync(binDir)) {
      const isRuntimeLibrary = process.platform === 'win32' && entry.toLowerCase().endsWith('.dll')
      if (!binKeeps.has(entry) && !isRuntimeLibrary) {
        rmSync(resolve(binDir, entry), { recursive: true, force: true })
      }
    }
  }

  const scannerDir = dirname(resolve(targetDir, spec.scanner))
  const scannerKeeps = new Set([basename(spec.scanner)])
  if (existsSync(scannerDir)) {
    for (const entry of readdirSync(scannerDir)) {
      if (!scannerKeeps.has(entry)) {
        rmSync(resolve(scannerDir, entry), { recursive: true, force: true })
      }
    }
  }
}

function collectStripCandidates(rootDir) {
  const candidates = []
  for (const entry of readdirSync(rootDir, { withFileTypes: true })) {
    const entryPath = resolve(rootDir, entry.name)
    if (entry.isDirectory()) {
      candidates.push(...collectStripCandidates(entryPath))
      continue
    }
    if (!entry.isFile()) {
      continue
    }
    const stat = lstatSync(entryPath)
    if (entry.name.endsWith('.dylib') || (stat.mode & 0o111) !== 0) {
      candidates.push(entryPath)
    }
  }
  return candidates
}

function stripDarwinBinaries(targetDir) {
  if (process.env.GSTREAMER_KEEP_SYMBOLS || process.platform !== 'darwin') {
    return
  }
  const candidates = collectStripCandidates(targetDir)
  const chunkSize = 50
  for (let index = 0; index < candidates.length; index += chunkSize) {
    execFileSync('strip', ['-x', ...candidates.slice(index, index + chunkSize)], { stdio: 'ignore' })
  }
}

function pruneDisplayRuntime(targetDir, spec) {
  pruneDisplayPlugins(targetDir)
  pruneDarwinLibraries(targetDir, spec)
  pruneRuntimeLayout(targetDir, spec)
  stripDarwinBinaries(targetDir)
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
    throw new Error(`Prepared GStreamer inspector not found: ${inspectPath}`)
  }
  for (const element of displayInspectionElements) {
    execFileSync(inspectPath, [element], {
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
  if (!process.env.GSTREAMER_FORCE_DOWNLOAD && canReuseExistingRuntime(targetDir, spec)) {
    try {
      console.log(`Minimizing existing GStreamer ${gstreamerVersion} runtime for ${target}`)
      pruneDisplayRuntime(targetDir, spec)
      validateRuntime(targetDir, spec)
      writeFileSync(versionPath(targetDir), versionMarker(), 'utf8')
      return
    } catch (error) {
      console.warn(`Existing GStreamer runtime could not be minimized: ${formatDownloadError(error)}. Redownloading...`)
    }
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

  pruneDisplayRuntime(targetDir, spec)
  validateRuntime(targetDir, spec)
  writeFileSync(versionPath(targetDir), versionMarker(), 'utf8')
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
