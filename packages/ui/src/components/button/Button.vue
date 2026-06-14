<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '.'
import { Primitive } from 'reka-ui'
import { computed } from 'vue'
import { Spinner } from '#/components/spinner'
import { cn } from '#/lib/utils'
import { buttonVariants } from '.'

interface Props extends PrimitiveProps {
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  /** Stretch to the full width of the parent. Block primaries swap the
   *  press-scale for an instant color-press (a full-width scale would lurch). */
  block?: boolean
  /** Busy state: shows a centered spinner, blocks clicks, stays FULL color
   *  (busy ≠ disabled). Layout never shifts — the label is hidden in place and
   *  the spinner overlays it, so it works for any width without a glyph swap. */
  loading?: boolean
  /** Loading behavior copied from the contract bench:
   *  - overlay: hide label in place and center a spinner (text-only buttons)
   *  - icon: keep content visible and spin the leading icon (no glyph swap)
   *  - leading: animate a spinner slot before the label (full-width CTAs)
   *  - manual: only the busy chrome (full color, blocked clicks) — the caller
   *    renders its own loading glyph in the slot (e.g. an icon↔spinner↔result
   *    swap that must stay in place). The button draws no spinner of its own.
   */
  loadingMode?: 'overlay' | 'icon' | 'leading' | 'manual'
  /** Inert + faded. Declared as a prop (not just a fallthrough attr) so we can
   *  OR it with `loading` without one clobbering the other. */
  disabled?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'button',
  loadingMode: 'overlay',
})

// Only stamp native `disabled` on real <button>s (an <a as="a"> can't be
// disabled). Loading also disables to swallow double-clicks; busy ≠ disabled
// visually is handled in style.css ([data-loading]:disabled stays full color).
const isNativeButton = computed(() => !props.asChild && props.as === 'button')
const isDisabled = computed(() =>
  isNativeButton.value && (props.disabled || props.loading) ? true : undefined,
)
const resolvedVariant = computed<ButtonVariants['variant']>(() => props.variant ?? 'default')
const buttonClass = computed(() =>
  cn(
    buttonVariants({ variant: resolvedVariant.value, size: props.size }),
    props.block && 'w-full',
    props.class,
  ),
)
</script>

<template>
  <!-- data-button is a STABLE chrome anchor: when this Button is the child of a
       reka `as-child` trigger (DialogTrigger, DropdownMenuTrigger, …) the trigger's
       own data-slot ("dialog-trigger", …) overrides data-slot="button", which would
       otherwise strip every fill/ring/press rule (all keyed off [data-slot=button]).
       The trigger never sets data-button, so it survives the merge and the button
       chrome in style.css keys off [data-button] instead. -->
  <Primitive
    data-slot="button"
    data-button=""
    :as="as"
    :as-child="asChild"
    :data-variant="resolvedVariant"
    :data-size="size || undefined"
    :data-block="block ? '' : undefined"
    :data-loading="loading ? '' : undefined"
    :data-loading-mode="loadingMode"
    :disabled="isDisabled"
    :aria-busy="loading || undefined"
    :class="buttonClass"
  >
    <span
      v-if="loadingMode === 'leading'"
      data-button-leading-spinner
    >
      <Spinner />
    </span>
    <span
      data-button-content
      class="contents"
    >
      <slot />
    </span>
    <span
      v-if="loading && loadingMode === 'overlay'"
      data-button-spinner
      class="absolute inset-0 grid place-items-center"
    >
      <Spinner />
    </span>
  </Primitive>
</template>
