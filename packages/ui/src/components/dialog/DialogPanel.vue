<script setup lang="ts">
// DialogPanel — the capped focused-dialog shell, as a COMPONENT instead of a
// recipe. Before this existed, every focused dialog had to copy a magic
// class string onto DialogContent:
//
//   max-h-[80dvh] grid-rows-[auto_minmax(0,1fr)] sm:max-w-2xl  (+ :show-close-button="false" for view-swap)
//
// and getting any fragment wrong reproduced a known bug: dropping the
// minmax(0,1fr) lets the body's min-content floor overflow the 80dvh cap;
// keeping the built-in corner close in a view-swap dialog puts it ~12px off
// the title centerline. A component library exists precisely so callers never
// hand-write that string — this component IS that string.
//
// Anatomy (the two golden forms, memoh-web SKILL § 11):
//   (a) workbench  — <DialogPanel> + DialogHeader/DialogTitle + DialogBody
//   (b) view-swap  — <DialogPanel view-swap> + DialogViewHeader + DialogBody(AutoHeight)
//
// Props are the ONLY knobs a caller should need:
// - width:   panel width rung. '2xl' is the house default for a focused
//   dialog; 'lg' for a one-or-two-field form (a 2xl panel around one field
//   reads barren); '3xl' for editor-heavy bodies (Monaco import).
// - grow:    false (default) → max-h-[80dvh], panel hugs its content and the
//   cap only bites when content is tall. true → h-[80dvh] fixed — required
//   when the body has NO intrinsic height (an editor/iframe that must be
//   GIVEN height to fill).
// - viewSwap: disables the built-in corner close because DialogViewHeader
//   renders the close inline on the title's centerline. Forgetting this pair
//   is the 12px-off-centerline bug — binding them is the point.
// - footer:  adds a third auto row for a DialogFooter. Deliberately a prop,
//   not slot-sniffing: the rows must be a static class for the CSS to exist,
//   and an empty declared row would still eat one grid gap (visible dead
//   space at the panel bottom).
import type { DialogContentEmits, DialogContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { useForwardPropsEmits } from 'reka-ui'
import { cn } from '#/lib/utils'
import DialogContent from './DialogContent.vue'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<DialogContentProps & {
  class?: HTMLAttributes['class']
  /** Panel width rung. Add rungs here deliberately — not per-page. */
  width?: 'lg' | '2xl' | '3xl'
  /** Fixed 80dvh height for bodies with no intrinsic height (editors). */
  grow?: boolean
  /** View-swap dialog: header is a DialogViewHeader, so the built-in corner
   *  close is disabled (it would sit off the title centerline). */
  viewSwap?: boolean
  /** Reserve a third grid row for a DialogFooter. */
  footer?: boolean
}>(), {
  width: '2xl',
  grow: false,
  viewSwap: false,
  footer: false,
})
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'width', 'grow', 'viewSwap', 'footer')
const forwarded = useForwardPropsEmits(delegatedProps, emits)

// Full literal strings (not interpolation) so Tailwind's scanner sees them.
const WIDTH = {
  'lg': 'sm:max-w-lg',
  '2xl': 'sm:max-w-2xl',
  '3xl': 'sm:max-w-3xl',
} as const
</script>

<template>
  <DialogContent
    v-bind="{ ...$attrs, ...forwarded }"
    :show-close-button="!viewSwap"
    :class="cn(
      grow ? 'h-[80dvh]' : 'max-h-[80dvh]',
      footer ? 'grid-rows-[auto_minmax(0,1fr)_auto]' : 'grid-rows-[auto_minmax(0,1fr)]',
      WIDTH[width],
      props.class,
    )"
  >
    <slot />
  </DialogContent>
</template>
