<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="item-content"
    :class="cn('flex flex-1 flex-col gap-0.5 [&+[data-slot=item-content]]:flex-none', props.class)"
  >
    <slot />
  </div>
</template>
