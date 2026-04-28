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

// Collect all standalone `id="X"` attributes declared inside the SVG inner
// content. Skips compound attribute names like `p-id` whose suffix happens
// to spell `id`.
function collectIds(innerContent: string): Set<string> {
  const ids = new Set<string>()
  for (const m of innerContent.matchAll(/(?<=[\s<])id="([^"]+)"/g)) {
    if (m[1]) ids.add(m[1])
  }
  return ids
}

// Rewrite SVG inner content so all internal id references become unique
// per-instance using a `${uidPrefix}-` prefix. The result must be embeddable
// in a Vue <template>: any attribute whose value contains `${...}` is converted
// from `attr="..."` to `:attr="`...`"` form so Vue evaluates the interpolation.
function scopeIdsForTemplate(innerContent: string, ids: Set<string>): string {
  const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  let out = innerContent

  // Helper: replace within attribute values, marking them as bound.
  // We walk all attributes and inspect their value; if the value references
  // `#<id>` (in url(#...), href="#...", or starts with #...), we convert the
  // attribute to a Vue binding using a template literal.
  for (const id of ids) {
    const idRe = escapeRe(id)

    // 1. id="X" → :id="`${uid}-X`" (declaration site, attribute-boundary aware)
    out = out.replace(
      new RegExp(`(?<=[\\s<])id="${idRe}"`, 'g'),
      `:id="\`\${uid}-${id}\`"`,
    )

    // 2. url(#X) inside any attribute value → convert that attribute to :binding
    //    Match attribute="…url(#X)…" (non-greedy) and rewrite to :attribute="`…url(#${uid}-X)…`"
    out = out.replace(
      new RegExp(
        `(?<=[\\s<])([a-zA-Z][a-zA-Z0-9:-]*)="([^"]*url\\(#${idRe}\\)[^"]*)"`,
        'g',
      ),
      (_match, attr: string, value: string) => {
        const bound = value.replace(
          new RegExp(`url\\(#${idRe}\\)`, 'g'),
          `url(#\${uid}-${id})`,
        )
        return `:${attr}="\`${bound}\`"`
      },
    )

    // 3. href="#X" / xlink:href="#X" → :href="`#${uid}-X`"
    out = out.replace(
      new RegExp(`(?<=[\\s<])(xlink:href|href)="#${idRe}"`, 'g'),
      (_match, attr: string) => `:${attr}="\`#\${uid}-${id}\`"`,
    )
  }

  return out
}

function generateVueSFC(viewBox: string, innerContent: string, isColor: boolean): string {
  const fillAttr = isColor ? '' : '\n    fill="currentColor"'
  const ids = collectIds(innerContent)
  const hasIds = ids.size > 0

  if (!hasIds) {
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

  const scoped = scopeIdsForTemplate(innerContent, ids)
  return `<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="${viewBox}"${fillAttr}
    v-bind="$attrs"
  >${scoped}</svg>
</template>

<script setup lang="ts">
import { useId } from 'vue'
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })

const uid = useId()
</script>
`
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

console.log('🎨 @memohai/icon generator\n')

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

// Include any hand-authored icon SFCs that live in src/icons/ but are not
// listed in the manifest (e.g. Misskey, which has no source SVG).
const knownComponents = new Set(
  exports.map(line => line.match(/from '\.\/icons\/([^']+)\.vue'/)?.[1]).filter(Boolean) as string[],
)
const orphanExports: string[] = []
for (const fname of fs.readdirSync(SRC_ICONS_DIR)) {
  if (!fname.endsWith('.vue')) continue
  const componentName = fname.replace(/\.vue$/, '')
  if (knownComponents.has(componentName)) continue
  orphanExports.push(`export { default as ${componentName} } from './icons/${componentName}.vue'`)
}

const allExports = [...exports, ...orphanExports]
allExports.sort()
fs.writeFileSync(INDEX_FILE, allExports.join('\n') + '\n')

console.log(`   ✅ Generated: ${generated}, Missing: ${missing}`)
if (orphanExports.length) {
  console.log(`   📌 Preserved ${orphanExports.length} hand-authored icon(s) not in manifest`)
}
console.log(`   📄 Index: ${allExports.length} exports written to src/index.ts`)
console.log('\n✨ Done!\n')
