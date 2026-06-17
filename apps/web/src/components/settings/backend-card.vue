<template>
  <button
    type="button"
    class="group/card flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card p-3.5 text-left transition-colors hover:bg-accent/30 dark:hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    <span class="relative shrink-0">
      <slot name="leading" />
      <span
        v-if="enabled"
        class="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full bg-success ring-2 ring-card"
      />
    </span>

    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium text-foreground">
        {{ name }}
      </span>
      <span
        v-if="subtitle"
        class="mt-0.5 block truncate text-xs text-muted-foreground"
      >
        {{ subtitle }}
      </span>
    </span>

    <slot name="trailing">
      <ChevronRight class="size-4 shrink-0 text-muted-foreground/60" />
    </slot>
  </button>
</template>

<script setup lang="ts">
import { ChevronRight } from 'lucide-vue-next'

withDefaults(defineProps<{
  name: string
  subtitle?: string
  enabled?: boolean
}>(), {
  subtitle: '',
  enabled: false,
})
</script>
