#!/usr/bin/env node
// Regenerate every desktop icon asset from apps/web/public/logo.svg.
//
// Outputs:
//   build/icon.png          1024x1024 (electron-builder linux + canonical raster)
//   build/icon.icns         multi-resolution macOS bundle icon
//   build/icon.ico          multi-resolution Windows icon
//   resources/icon.png      512x512 runtime BrowserWindow.icon / dock.setIcon
//
// Run:  pnpm --filter @memohai/desktop icons
//
// The brand logo is non-square (~1.135:1); we render it centered onto a
// transparent square canvas with PADDING_RATIO of headroom on each side so
// macOS Big Sur+ squircle masks and Windows 1px borders don't clip the mark.

import { execFile } from 'node:child_process'
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

import pngToIco from 'png-to-ico'
import sharp from 'sharp'

const execFileAsync = promisify(execFile)

const __dirname = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(__dirname, '..')
const SRC_SVG = resolve(ROOT, '../web/public/logo.svg')
const BUILD_DIR = resolve(ROOT, 'build')
const RESOURCES_DIR = resolve(ROOT, 'resources')

const MASTER_SIZE = 1024
// 14% padding on each side ≈ matches Apple's ~14% safe area for icon glyphs.
const PADDING_RATIO = 0.14

const ICONSET_SIZES = [
  { name: 'icon_16x16.png', size: 16 },
  { name: 'icon_16x16@2x.png', size: 32 },
  { name: 'icon_32x32.png', size: 32 },
  { name: 'icon_32x32@2x.png', size: 64 },
  { name: 'icon_128x128.png', size: 128 },
  { name: 'icon_128x128@2x.png', size: 256 },
  { name: 'icon_256x256.png', size: 256 },
  { name: 'icon_256x256@2x.png', size: 512 },
  { name: 'icon_512x512.png', size: 512 },
  { name: 'icon_512x512@2x.png', size: 1024 },
]

const ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]

async function renderMaster() {
  const svg = await readFile(SRC_SVG)
  // Derive `inset` from `margin` (not the other way) to guarantee
  // inset + 2*margin === MASTER_SIZE exactly, avoiding off-by-one.
  const margin = Math.floor(MASTER_SIZE * PADDING_RATIO)
  const inset = MASTER_SIZE - margin * 2

  const inner = await sharp(svg, { density: 600 })
    .resize(inset, inset, {
      fit: 'contain',
      background: { r: 0, g: 0, b: 0, alpha: 0 },
    })
    .png()
    .toBuffer()

  return sharp(inner)
    .extend({
      top: margin,
      bottom: margin,
      left: margin,
      right: margin,
      background: { r: 0, g: 0, b: 0, alpha: 0 },
    })
    .png()
    .toBuffer()
}

async function writeResized(master, size, outPath) {
  await sharp(master).resize(size, size, { fit: 'contain' }).png().toFile(outPath)
}

async function buildIcns(master) {
  const iconset = resolve(BUILD_DIR, 'icon.iconset')
  await rm(iconset, { recursive: true, force: true })
  await mkdir(iconset, { recursive: true })

  await Promise.all(
    ICONSET_SIZES.map(({ name, size }) =>
      writeResized(master, size, resolve(iconset, name)),
    ),
  )

  await execFileAsync('iconutil', [
    '-c', 'icns',
    iconset,
    '-o', resolve(BUILD_DIR, 'icon.icns'),
  ])
  await rm(iconset, { recursive: true, force: true })
}

async function buildIco(master) {
  const buffers = await Promise.all(
    ICO_SIZES.map(size =>
      sharp(master).resize(size, size, { fit: 'contain' }).png().toBuffer(),
    ),
  )
  const ico = await pngToIco(buffers)
  await writeFile(resolve(BUILD_DIR, 'icon.ico'), ico)
}

async function main() {
  await mkdir(BUILD_DIR, { recursive: true })
  await mkdir(RESOURCES_DIR, { recursive: true })

  console.log(`source: ${SRC_SVG}`)
  const master = await renderMaster()

  await Promise.all([
    sharp(master).png().toFile(resolve(BUILD_DIR, 'icon.png')),
    sharp(master).resize(512, 512, { fit: 'contain' }).png()
      .toFile(resolve(RESOURCES_DIR, 'icon.png')),
  ])
  console.log('  -> build/icon.png (1024)')
  console.log('  -> resources/icon.png (512)')

  if (process.platform === 'darwin') {
    await buildIcns(master)
    console.log('  -> build/icon.icns (16…1024 + @2x)')
  } else {
    console.warn('  ! skipping icon.icns: iconutil only available on macOS')
  }

  await buildIco(master)
  console.log(`  -> build/icon.ico (${ICO_SIZES.join('/')})`)
}

main().catch(err => {
  console.error(err)
  process.exit(1)
})
