<script setup lang="ts">
import type { DialogContentEmits, DialogContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  DialogContent,
  DialogPortal,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '#/lib/utils'
import DialogCloseButton from './DialogCloseButton.vue'
import DialogOverlay from './DialogOverlay.vue'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<DialogContentProps & {
  class?: HTMLAttributes['class']
  showCloseButton?: boolean
}>(), {
  showCloseButton: true,
})
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <DialogPortal>
    <DialogOverlay />
    <DialogContent
      data-slot="dialog-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="
        cn(
          // Same modal edge as CommandDialog (--border-menu-elevated): LIGHT mode has
          // NO border (the card already separates from the dark scrim by luminance + the
          // shadow; a dark hairline the same darkness as the scrim only muddies it), DARK
          // mode keeps a white hairline so a dark card stays detached from a dark scrim.
          'bg-card border border-[color:var(--border-menu-elevated)]',
          // Enter/exit aligned with CommandDialog: a fast (100ms) fade + a subtle 2%
          // zoom (NOT the old 200ms / 5% zoom) so every modal surface opens with the
          // same snappy motion. No will-change-transform: at 2% zoom the shadow repaint
          // is cheap, and promoting the translate(-50%,-50%)-centered panel to its own
          // layer could land the 1px hairline on a half-pixel and render it blurry.
          'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-[0.98] data-[state=open]:zoom-in-[0.98]',
          'fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-4 rounded-xl p-6 shadow-[var(--shadow-modal)] duration-100 sm:max-w-md',
          props.class,
        )"
    >
      <slot />

      <DialogCloseButton v-if="showCloseButton" />
    </DialogContent>
  </DialogPortal>
</template>
