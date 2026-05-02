import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync, chmodSync, createWriteStream } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, resolve } from 'node:path'
import { pipeline } from 'node:stream/promises'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const desktopRoot = resolve(__dirname, '..')
const qdrantRoot = resolve(desktopRoot, 'resources', 'qdrant')

const defaultVersion = 'v1.17.1'
const qdrantVersion = process.env.QDRANT_VERSION || defaultVersion

const targetSpecs = {
  'darwin-arm64': {
    asset: 'qdrant-aarch64-apple-darwin.tar.gz',
    binary: 'qdrant',
  },
  'darwin-x64': {
    asset: 'qdrant-x86_64-apple-darwin.tar.gz',
    binary: 'qdrant',
  },
  'linux-arm64': {
    asset: 'qdrant-aarch64-unknown-linux-musl.tar.gz',
    binary: 'qdrant',
  },
  'linux-x64': {
    asset: 'qdrant-x86_64-unknown-linux-musl.tar.gz',
    binary: 'qdrant',
  },
  'win32-x64': {
    asset: 'qdrant-x86_64-pc-windows-msvc.zip',
    binary: 'qdrant.exe',
  },
}

function currentTarget() {
  return `${process.platform}-${process.arch}`
}

function releasePlatformTargets() {
  switch (process.platform) {
    case 'darwin':
      return ['darwin-arm64', 'darwin-x64']
    case 'linux':
      return ['linux-x64']
    case 'win32':
      return ['win32-x64']
    default:
      return [currentTarget()]
  }
}

function parseTargets() {
  const arg = process.argv.find(item => item.startsWith('--targets='))
  const raw = arg?.slice('--targets='.length) || process.env.QDRANT_TARGETS || 'current'
  switch (raw) {
    case 'all':
      return Object.keys(targetSpecs)
    case 'current':
      return [currentTarget()]
    case 'release-platform':
      return releasePlatformTargets()
    case 'package':
      console.warn('QDRANT target "package" is deprecated; use "release-platform".')
      return releasePlatformTargets()
    default:
      return raw.split(',').map(item => item.trim()).filter(Boolean)
  }
}

function assertSupportedTarget(target) {
  if (targetSpecs[target]) {
    return
  }
  throw new Error(`Unsupported Qdrant target "${target}". Supported targets: ${Object.keys(targetSpecs).join(', ')}`)
}

function versionPath(targetDir) {
  return resolve(targetDir, 'VERSION')
}

function isPrepared(target, spec) {
  const targetDir = resolve(qdrantRoot, target)
  const binaryPath = resolve(targetDir, spec.binary)
  const markerPath = versionPath(targetDir)
  if (!existsSync(binaryPath) || !existsSync(markerPath)) {
    return false
  }
  try {
    return readFileSync(markerPath, 'utf8').trim() === qdrantVersion
  } catch {
    return false
  }
}

async function downloadAsset(url, archivePath) {
  const response = await fetch(url, {
    headers: {
      'User-Agent': 'memoh-desktop-qdrant-preparer',
    },
  })
  if (!response.ok || !response.body) {
    throw new Error(`Failed to download ${url}: HTTP ${response.status}`)
  }
  await pipeline(response.body, createWriteStream(archivePath))
}

function extractZipArchive(archivePath, extractDir) {
  if (process.platform === 'win32') {
    execFileSync('powershell.exe', [
      '-NoProfile',
      '-Command',
      'Expand-Archive -LiteralPath $args[0] -DestinationPath $args[1] -Force',
      archivePath,
      extractDir,
    ], { stdio: 'inherit' })
    return
  }
  execFileSync('unzip', ['-q', archivePath, '-d', extractDir], { stdio: 'inherit' })
}

function findExtractedBinary(root, binaryName) {
  const entries = readdirSync(root, { withFileTypes: true })
  for (const entry of entries) {
    const child = resolve(root, entry.name)
    if (entry.isFile() && entry.name === binaryName) {
      return child
    }
    if (entry.isDirectory()) {
      const nested = findExtractedBinary(child, binaryName)
      if (nested) {
        return nested
      }
    }
  }
  return null
}

function extractArchive(archivePath, target, spec) {
  const extractDir = resolve(tmpdir(), `memoh-qdrant-${target}-${Date.now()}`)
  rmSync(extractDir, { recursive: true, force: true })
  mkdirSync(extractDir, { recursive: true })

  if (archivePath.endsWith('.zip')) {
    extractZipArchive(archivePath, extractDir)
  } else {
    execFileSync('tar', ['-xzf', archivePath, '-C', extractDir], { stdio: 'inherit' })
  }

  const extractedBinary = findExtractedBinary(extractDir, spec.binary)
  if (!extractedBinary) {
    throw new Error(`Could not find ${spec.binary} in ${basename(archivePath)}`)
  }

  const targetDir = resolve(qdrantRoot, target)
  rmSync(targetDir, { recursive: true, force: true })
  mkdirSync(targetDir, { recursive: true })
  const targetBinary = resolve(targetDir, spec.binary)
  copyFileSync(extractedBinary, targetBinary)
  if (process.platform !== 'win32') {
    chmodSync(targetBinary, 0o755)
  }
  writeFileSync(versionPath(targetDir), `${qdrantVersion}\n`, 'utf8')
  rmSync(extractDir, { recursive: true, force: true })
}

async function prepareTarget(target) {
  assertSupportedTarget(target)
  const spec = targetSpecs[target]
  const targetDir = resolve(qdrantRoot, target)
  if (!process.env.QDRANT_FORCE_DOWNLOAD && isPrepared(target, spec)) {
    console.log(`Qdrant ${qdrantVersion} already prepared for ${target}`)
    return
  }

  mkdirSync(qdrantRoot, { recursive: true })
  const url = `https://github.com/qdrant/qdrant/releases/download/${qdrantVersion}/${spec.asset}`
  const archivePath = resolve(tmpdir(), `${qdrantVersion}-${spec.asset}`)
  console.log(`Downloading Qdrant ${qdrantVersion} for ${target}`)
  await downloadAsset(url, archivePath)
  extractArchive(archivePath, target, spec)
  rmSync(archivePath, { force: true })
  console.log(`Prepared Qdrant for ${target} in ${targetDir}`)
}

const targets = [...new Set(parseTargets())]
for (const target of targets) {
  await prepareTarget(target)
}
