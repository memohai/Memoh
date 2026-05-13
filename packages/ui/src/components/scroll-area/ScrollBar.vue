<script setup lang="ts">
import type { ScrollAreaScrollbarProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ScrollAreaScrollbar, ScrollAreaThumb } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = withDefaults(defineProps<ScrollAreaScrollbarProps & { class?: HTMLAttributes['class'] }>(), {
  orientation: 'vertical',
})

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <ScrollAreaScrollbar
    data-slot="scroll-area-scrollbar"
    v-bind="delegatedProps"
    :class="
      cn('flex touch-none p-0.5 transition-colors select-none justify-center',
         orientation === 'vertical'
           && 'h-full w-2 border-l border-l-transparent',
         orientation === 'horizontal'
           && 'h-2 flex-col border-t border-t-transparent',
         props.class)"
  >
    <ScrollAreaThumb
      data-slot="scroll-area-thumb"
      class="bg-border/40 hover:bg-border/80 transition-colors relative flex-1 rounded-full"
    />
  </ScrollAreaScrollbar>
</template>
