<template>
  <!-- Single syntax-highlight render kernel shared by every code surface
       (chat fences, write/read tool details). It owns exactly two concerns:
       running the highlighter and emitting transparent <pre>/<code> so the code
       always inherits its container's background. All chrome (frame, copy
       button, scroll bounds, text size) belongs to the caller via class
       fallthrough on this root. While highlighting is pending we show the raw
       text instead of a spinner, so streaming content stays readable. -->
  <!-- eslint-disable vue/no-v-html -->
  <div
    v-if="html"
    class="min-w-0 [&_pre]:bg-transparent! [&_pre]:m-0! [&_pre]:p-0! [&_pre]:whitespace-pre [&_code]:bg-transparent! [&_code]:font-mono"
    v-html="html"
  />
  <!-- eslint-enable vue/no-v-html -->
  <pre
    v-else
    class="min-w-0 whitespace-pre font-mono text-foreground"
  >{{ code }}</pre>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { extractFilename, useShikiHighlighter } from '@/composables/useShikiHighlighter'

const props = defineProps<{
  code: string
  // Provide exactly one hint: `filename` derives the language from the file
  // extension (write/read), `lang` is an explicit fence id (markdown ```ts).
  lang?: string
  filename?: string
}>()

const { html, highlight, highlightLang } = useShikiHighlighter()

watch(
  () => [props.code, props.lang, props.filename] as const,
  ([code, lang, filename]) => {
    if (!code) {
      html.value = ''
      return
    }
    if (filename) void highlight(code, extractFilename(filename))
    else void highlightLang(code, lang || 'text')
  },
  { immediate: true },
)
</script>
