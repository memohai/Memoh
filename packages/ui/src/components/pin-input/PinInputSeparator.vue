<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { MinusIcon } from 'lucide-vue-next'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    role="separator"
    data-slot="pin-input-separator"
    :class="cn('flex items-center justify-center text-muted-foreground', props.class)"
  >
    <slot>
      <MinusIcon class="size-4" />
    </slot>
  </div>
</template>
