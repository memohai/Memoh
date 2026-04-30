<template>
  <span
    v-for="cap in compatibilities"
    :key="cap"
    :title="$t(`models.compatibility.${cap}`, cap)"
    class="inline-flex items-center justify-center rounded-md border size-5 shrink-0 bg-transparent"
    :class="styleOf(cap)"
  >
    <component
      :is="iconOf(cap)"
      class="size-3"
    />
  </span>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { Wrench, Eye, Image, Brain } from 'lucide-vue-next'

defineProps<{
  compatibilities: string[]
}>()

const ICONS: Record<string, Component> = {
  'tool-call': Wrench,
  'vision': Eye,
  'image-output': Image,
  'reasoning': Brain,
}

const CLASSES: Record<string, string> = {
  'tool-call': 'border-blue-500/30 text-blue-600 dark:border-blue-400/30 dark:text-blue-400',
  'vision': 'border-purple-500/30 text-purple-600 dark:border-purple-400/30 dark:text-purple-400',
  'image-output': 'border-pink-500/30 text-pink-600 dark:border-pink-400/30 dark:text-pink-400',
  'reasoning': 'border-amber-500/30 text-amber-600 dark:border-amber-400/30 dark:text-amber-400',
}

function iconOf(cap: string): Component {
  return ICONS[cap] ?? Wrench
}

function styleOf(cap: string): string {
  return CLASSES[cap] ?? 'border-border text-foreground'
}
</script>
