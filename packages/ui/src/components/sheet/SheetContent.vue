<script setup lang="ts">
// DESIGN NOTE — use sparingly. An edge-anchored sheet that flies over the whole app
// is a heavy, disorienting transition; prefer an IN-PAGE push panel (content that
// slides a sibling region in beside what the user is already looking at) for most
// "show more detail / quick edit" needs. Reserve Sheet for genuine full-context
// secondary surfaces (mobile nav, a deep filter/settings drawer) where leaving the
// current layout is acceptable.
import type { DialogContentEmits, DialogContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  DialogContent,
  DialogPortal,
  useForwardPropsEmits,
} from 'reka-ui'
import { DialogCloseButton } from '#/components/dialog'
import { cn } from '#/lib/utils'
import SheetOverlay from './SheetOverlay.vue'

interface SheetContentProps extends DialogContentProps {
  class?: HTMLAttributes['class']
  side?: 'top' | 'right' | 'bottom' | 'left'
}

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<SheetContentProps>(), {
  side: 'right',
})
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'side')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <DialogPortal>
    <SheetOverlay />
    <DialogContent
      data-slot="sheet-content"
      :class="cn(
        'fixed z-50 flex flex-col gap-4 shadow-[var(--shadow-modal)] transition ease-in-out',
        'bg-card border-border',
        'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:duration-150 data-[state=open]:duration-150',
        side === 'right'
          && 'data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right inset-y-0 right-0 h-full w-3/4 sm:max-w-sm',
        side === 'left'
          && 'data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left inset-y-0 left-0 h-full w-3/4 sm:max-w-sm',
        side === 'top'
          && 'data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top inset-x-0 top-0 h-auto',
        side === 'bottom'
          && 'data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom inset-x-0 bottom-0 h-auto',
        props.class)"
      v-bind="{ ...$attrs, ...forwarded }"
    >
      <slot />

      <DialogCloseButton />
    </DialogContent>
  </DialogPortal>
</template>
