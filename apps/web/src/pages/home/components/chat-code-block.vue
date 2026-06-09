<template>
  <!-- One uniform code block for every language (no per-language label / no
       terminal "run" chrome): a hairline divider frame and pure-white body.
       Layout is a flex row — the code scrolls inside its own column while the
       copy button is a stable trailing sibling (top-aligned). This keeps the
       block fit-to-content without the awkward reserved gap, and the button no
       longer jitters as the width grows during streaming. -->
  <div class="my-2 flex w-fit max-w-full items-start gap-1.5 overflow-hidden rounded-lg border border-border/60 bg-white py-1 pl-3.5 pr-1.5 dark:bg-card">
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-if="html && !loading"
      class="min-w-0 overflow-x-auto py-1.5 text-[13px] leading-relaxed [&_pre]:bg-transparent! [&_pre]:m-0! [&_pre]:p-0! [&_code]:bg-transparent!"
      v-html="html"
    />
    <!-- eslint-enable vue/no-v-html -->
    <pre
      v-else
      class="min-w-0 overflow-x-auto whitespace-pre py-1.5 font-mono text-[13px] leading-relaxed text-foreground"
    >{{ code }}</pre>
    <Button
      variant="ghost"
      size="icon-sm"
      class="shrink-0 text-muted-foreground focus-visible:ring-0 hover:text-foreground"
      :aria-label="t('common.copy', 'Copy')"
      @click="copy"
    >
      <Check
        v-if="copied"
        class="size-3.5"
      />
      <Copy
        v-else
        class="size-3.5"
      />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Copy } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useShikiHighlighter } from '@/composables/useShikiHighlighter'

interface CodeFenceNode {
  type: string
  language?: string
  code?: string
  raw?: string
  loading?: boolean
}

const props = defineProps<{ node: CodeFenceNode }>()
const { t } = useI18n()
const { html, loading, highlightLang } = useShikiHighlighter()

const code = computed(() => props.node.code ?? props.node.raw ?? '')
const language = computed(() => (props.node.language ?? '').trim().toLowerCase())

watch(
  [code, language],
  ([c, l]) => {
    if (c) void highlightLang(c, l || 'text')
  },
  { immediate: true },
)

const copied = ref(false)
let resetTimer: ReturnType<typeof setTimeout> | null = null
async function copy() {
  try {
    await navigator.clipboard.writeText(code.value)
    copied.value = true
    if (resetTimer) clearTimeout(resetTimer)
    resetTimer = setTimeout(() => { copied.value = false }, 1500)
  }
  catch {
    // ignore clipboard failures
  }
}
</script>
