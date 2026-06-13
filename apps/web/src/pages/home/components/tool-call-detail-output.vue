<template>
  <pre
    v-if="text"
    class="font-mono text-[12px] leading-relaxed text-foreground whitespace-pre overflow-x-auto max-h-72 overflow-y-auto"
  >{{ text }}</pre>
  <p
    v-else
    class="text-xs text-muted-foreground italic"
  >
    {{ t('chat.tools.detail.noOutput') }}
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

// Render the real tool output (file text / listing), handling every shape the
// backend uses: a plain `content` string, an MCP `content[].text` array, a
// `structuredContent` object, or a bare string result.
const text = computed(() => {
  const result = props.block.result
  if (result == null) return ''
  if (typeof result === 'string') return result
  const r = result as Record<string, unknown>
  if (typeof r.content === 'string') return r.content
  if (Array.isArray(r.content)) {
    const joined = (r.content as Array<Record<string, unknown>>)
      .filter(c => c.type === 'text')
      .map(c => c.text as string)
      .filter(Boolean)
      .join('\n')
    if (joined) return joined
  }
  const sc = r.structuredContent as Record<string, unknown> | undefined
  if (sc && typeof sc.content === 'string') return sc.content
  if (sc && typeof sc.output === 'string') return sc.output
  try {
    return JSON.stringify(sc ?? r, null, 2)
  }
  catch {
    return String(result)
  }
})
</script>
