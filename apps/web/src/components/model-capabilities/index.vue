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
  'tool-call': 'border-capability-tool-border text-capability-tool',
  'vision': 'border-capability-vision-border text-capability-vision',
  'image-output': 'border-capability-image-border text-capability-image',
  'reasoning': 'border-capability-reasoning-border text-capability-reasoning',
}

function iconOf(cap: string): Component {
  return ICONS[cap] ?? Wrench
}

function styleOf(cap: string): string {
  return CLASSES[cap] ?? 'border-border text-foreground'
}
</script>
