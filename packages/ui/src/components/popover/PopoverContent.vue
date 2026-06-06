<script setup lang="ts">
import type { PopoverContentEmits, PopoverContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  PopoverContent,
  PopoverPortal,
  useForwardPropsEmits,
} from 'reka-ui'
import { computed } from 'vue'
import { menuSlideClass } from '#/lib/menu'
import { cn } from '#/lib/utils'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<PopoverContentProps & {
    class?: HTMLAttributes['class']
    // `menu` turns this Popover into a host for a menu surface (Combobox = Popover +
    // Command). It drops the generic popover chrome (border/shadow/padding/width) and
    // the zoom motion, and instead plays the menu family's fade + 1-unit directional
    // slide at duration-75 — identical to SelectContent — so an anchored picker reads
    // the same whether it's a Select or a Combobox. The inner surface (Command) brings
    // its own border/shadow/radius, so this wrapper stays transparent and unpadded.
    menu?: boolean
  }>(),
  {
    align: 'center',
    sideOffset: 4,
  },
)
const emits = defineEmits<PopoverContentEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'menu')

const forwarded = useForwardPropsEmits(delegatedProps, emits)

const baseClass = computed(() => props.menu
  ? cn(
    'z-50 origin-(--reka-popover-content-transform-origin) outline-hidden',
    'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 duration-75',
    menuSlideClass,
    'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
  )
  : 'bg-popover text-popover-foreground data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 z-50 w-72 rounded-lg border p-4 shadow-md origin-(--reka-popover-content-transform-origin) outline-hidden')
</script>

<template>
  <PopoverPortal>
    <PopoverContent
      data-slot="popover-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="cn(baseClass, props.class)"
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>
