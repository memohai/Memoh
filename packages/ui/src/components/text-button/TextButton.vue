<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '#/components/button'
import { Button } from '#/components/button'
import { cn } from '#/lib/utils'

// TextButton — the canonical "clickable text with a hover chip" affordance:
// text-scale type with a padded, rounded hit area bigger than the glyphs, that
// fills on hover and presses like any control. It is NOT a new interaction — it
// is the ghost <Button> at the compact `text` size, so all hover/press/focus
// chrome stays single-sourced in style.css (the contract's "never hand-roll a
// hover, reuse <Button>" rule). Use it for breadcrumb crumbs, inline dropdown
// triggers, low-emphasis labels/actions, etc. For an underlined inline link
// inside running text use <Button variant="link"> instead.
//
// Clickable text is low-emphasis by default: muted at rest, foreground on hover
// (the ghost chip fills underneath). Pass a text-* class to override the resting
// colour for the rare high-emphasis case.
//
// Defaults to a real <button>; pass `as="a"` (+ href) or `as-child` to wrap a
// RouterLink / menu trigger. `variant` / `size` stay overridable for the rare
// case that needs a different emphasis at text scale.
const props = withDefaults(defineProps<PrimitiveProps & {
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
}>(), {
  as: 'button',
  variant: 'ghost',
  size: 'text',
})
</script>

<template>
  <Button
    data-slot="text-button"
    :as="as"
    :as-child="asChild"
    :variant="variant"
    :size="size"
    :class="cn('text-muted-foreground hover:text-foreground', props.class)"
  >
    <slot />
  </Button>
</template>
