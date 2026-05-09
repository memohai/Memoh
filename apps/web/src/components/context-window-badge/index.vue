<template>
  <span
    v-if="contextWindow"
    class="inline-flex items-center gap-1 rounded-md border-0 px-2 py-0.5 text-xs font-medium shrink-0"
    :class="badgeClass"
  >
    {{ formatted }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  contextWindow: number | undefined
}>()

const formatted = computed(() => {
  const ctx = props.contextWindow
  if (!ctx) return ''
  if (ctx >= 1_000_000) return `${Math.round(ctx / 1_000_000)}M`
  if (ctx >= 1000) return `${Math.round(ctx / 1000)}k`
  return String(ctx)
})

const badgeClass = computed(() => {
  const ctx = props.contextWindow ?? 0
  if (ctx >= 1_000_000) return 'bg-context-window-xl text-context-window-foreground'
  if (ctx >= 100_000) return 'bg-context-window-lg text-context-window-foreground'
  if (ctx >= 32_000) return 'bg-context-window-md text-context-window-foreground'
  if (ctx >= 8_000) return 'bg-context-window-sm text-context-window-foreground'
  return 'bg-context-window-xs text-muted-foreground'
})
</script>
