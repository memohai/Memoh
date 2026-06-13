<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

interface SkeletonProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<SkeletonProps>()
</script>

<template>
  <!-- bg-muted is the reduced-motion / fallback base; the sliding two-gray shimmer
       gradient is driven by the [data-slot=skeleton] rule in style.css. -->
  <div
    data-slot="skeleton"
    :class="cn('rounded-lg bg-muted', props.class)"
  />
</template>
