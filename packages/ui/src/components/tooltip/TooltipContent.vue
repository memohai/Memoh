<script setup lang="ts">
import type { TooltipContentEmits, TooltipContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TooltipContent, TooltipPortal, useForwardPropsEmits } from 'reka-ui'
import { cn } from '#/lib/utils'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<TooltipContentProps & { class?: HTMLAttributes['class'] }>(), {
  sideOffset: 4,
})

const emits = defineEmits<TooltipContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')
const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <TooltipPortal>
    <!-- A terse hint, NOT a content surface: an inverted TRUE-black bubble (--tooltip,
         a standalone near-pure-black ink that flips to near-white in dark mode) clearly
         separates it from the light Popover / HoverCard cards. No border (the fill is
         its own edge), no arrow and no shadow, for a clean flat pill. Small radius +
         tight padding keep the hint compact. -->
    <TooltipContent
      data-slot="tooltip-content"
      v-bind="{ ...forwarded, ...$attrs }"
      :class="cn(
        'z-50 w-fit max-w-80 rounded-sm px-2 py-1 text-body font-medium whitespace-normal break-words',
        'bg-[color:var(--tooltip)] text-[color:var(--tooltip-foreground)]',
        props.class
      )"
    >
      <slot />
    </TooltipContent>
  </TooltipPortal>
</template>
