<template>
  <span
    v-for="cap in compatibilities"
    :key="cap"
    :title="$t(`models.compatibility.${cap}`, cap)"
    class="inline-flex items-center justify-center rounded-md border-0 size-5 shrink-0"
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
  'tool-call': 'bg-capability-tool-soft text-capability-tool-foreground',
  'vision': 'bg-capability-vision-soft text-capability-vision-foreground',
  'image-output': 'bg-capability-image-soft text-capability-image-foreground',
  'reasoning': 'bg-capability-reasoning-soft text-capability-reasoning-foreground',
}

function iconOf(cap: string): Component {
  return ICONS[cap] ?? Wrench
}

function styleOf(cap: string): string {
  return CLASSES[cap] ?? 'bg-accent text-foreground'
}
</script>
