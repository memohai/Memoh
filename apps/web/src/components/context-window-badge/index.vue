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
  if (ctx >= 1_000_000) return 'bg-violet-50 text-violet-700 dark:bg-violet-950 dark:text-violet-300'
  if (ctx >= 100_000) return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
  if (ctx >= 32_000) return 'bg-sky-50 text-sky-700 dark:bg-sky-950 dark:text-sky-300'
  if (ctx >= 8_000) return 'bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
  return 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400'
})
</script>
