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
import { Wrench, Eye, Image, Brain, Pencil } from 'lucide-vue-next'

defineProps<{
  compatibilities: string[]
}>()

const ICONS: Record<string, Component> = {
  'tool-call': Wrench,
  'vision': Eye,
  'image-output': Image,
  'generate': Image,
  'edit': Pencil,
  'reasoning': Brain,
}

const CLASSES: Record<string, string> = {
  'tool-call': 'bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300',
  'vision': 'bg-purple-50 text-purple-700 dark:bg-purple-950 dark:text-purple-300',
  'image-output': 'bg-pink-50 text-pink-700 dark:bg-pink-950 dark:text-pink-300',
  'generate': 'bg-pink-50 text-pink-700 dark:bg-pink-950 dark:text-pink-300',
  'edit': 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300',
  'reasoning': 'bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300',
}

function iconOf(cap: string): Component {
  return ICONS[cap] ?? Wrench
}

function styleOf(cap: string): string {
  return CLASSES[cap] ?? 'bg-accent text-foreground'
}
</script>
