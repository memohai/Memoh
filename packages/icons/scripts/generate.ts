import fs from 'node:fs'
import path from 'node:path'
import { manifest } from './manifest'

const ROOT = path.resolve(import.meta.dirname, '..')
const ICONS_DIR = path.resolve(ROOT, 'icons')
const SRC_ICONS_DIR = path.resolve(ROOT, 'src/icons')
const INDEX_FILE = path.resolve(ROOT, 'src/index.ts')

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toPascalCase(str: string): string {
  return str
    .split('-')
    .map(s => s.charAt(0).toUpperCase() + s.slice(1))
    .join('')
}

function extractSvgAttrs(svg: string): { viewBox: string; innerContent: string; isColor: boolean } {
  const viewBoxMatch = svg.match(/viewBox="([^"]+)"/)
  const viewBox = viewBoxMatch?.[1] ?? '0 0 24 24'

  const hasFillCurrentColor = /fill="currentColor"/.test(svg.match(/<svg[^>]*>/)?.[0] ?? '')
  const isColor = !hasFillCurrentColor

  const innerMatch = svg.match(/<svg[^>]*>([\s\S]*)<\/svg>/)
  let innerContent = innerMatch?.[1]?.trim() ?? ''

  innerContent = innerContent.replace(/<title>[^<]*<\/title>\s*/g, '')

  return { viewBox, innerContent, isColor }
}

function generateVueSFC(viewBox: string, innerContent: string, isColor: boolean): string {
  const fillAttr = isColor ? '' : '\n    fill="currentColor"'
  return `<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="${viewBox}"${fillAttr}
    v-bind="$attrs"
  >${innerContent}</svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
`
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

console.log('🎨 @memoh/icon generator\n')

fs.mkdirSync(SRC_ICONS_DIR, { recursive: true })

const exports: string[] = []
let generated = 0
let missing = 0

for (const file of manifest) {
  const svgPath = path.join(ICONS_DIR, `${file}.svg`)

  if (!fs.existsSync(svgPath)) {
    console.warn(`   ⚠ SVG not found: ${file}.svg (skipping)`)
    missing++
    continue
  }

  const svg = fs.readFileSync(svgPath, 'utf-8').trim()
  const { viewBox, innerContent, isColor } = extractSvgAttrs(svg)

  if (!innerContent) {
    console.warn(`   ⚠ Empty SVG content: ${file}.svg (skipping)`)
    missing++
    continue
  }

  const componentName = toPascalCase(file)
  const sfcContent = generateVueSFC(viewBox, innerContent, isColor)
  const outPath = path.join(SRC_ICONS_DIR, `${componentName}.vue`)

  fs.writeFileSync(outPath, sfcContent)
  exports.push(`export { default as ${componentName} } from './icons/${componentName}.vue'`)
  generated++
}

exports.sort()
fs.writeFileSync(INDEX_FILE, exports.join('\n') + '\n')

console.log(`   ✅ Generated: ${generated}, Missing: ${missing}`)
console.log(`   📄 Index: ${exports.length} exports written to src/index.ts`)
console.log('\n✨ Done!\n')
