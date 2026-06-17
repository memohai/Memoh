<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  size?: 'sm' | 'default' | 'lg'
}>(), {
  size: 'default',
})
</script>

<template>
  <div
    data-slot="input-group"
    role="group"
    :data-size="props.size"
    :class="cn(
      // Same field language as Input/Textarea/Select: transparent fill + a single
      // inset hairline edge (driven by --field-edge in style.css). Focus turns the
      // edge near-black in place; invalid turns it destructive. No hover, no outer
      // ring — those are intentionally gone from the design language.
      'group/input-group relative flex w-full items-center rounded-md outline-none',
      'min-w-0',

      // Multiline (textarea) groups grow with content and TOP-align: items-stretch
      // lets the textarea fill the box so its own py defines where text starts,
      // instead of items-center stranding a short field in the middle of the box.
      'has-[>textarea]:h-auto has-[>textarea]:items-stretch',

      // Size scale — fixed height for single-line groups only. Gated behind
      // :not(:has(>textarea)) so it never fights the auto-height of a textarea
      // group (otherwise [data-size] out-specifies :has() and re-pins the height).
      '[&:not(:has(>textarea))]:data-[size=sm]:h-8 [&:not(:has(>textarea))]:data-[size=default]:h-9 [&:not(:has(>textarea))]:data-[size=lg]:h-10',
      'data-[size=sm]:[&_[data-slot=input-group-control]]:text-body',
      'data-[size=lg]:[&_[data-slot=input-group-control]]:text-control',

      // Variants based on alignment.
      'has-[>[data-align=inline-start]]:[&>input]:pl-2',
      'has-[>[data-align=inline-end]]:[&>input]:pr-2',
      'has-[>[data-align=block-start]]:h-auto has-[>[data-align=block-start]]:flex-col has-[>[data-align=block-start]]:[&>input]:pb-3',
      'has-[>[data-align=block-end]]:h-auto has-[>[data-align=block-end]]:flex-col has-[>[data-align=block-end]]:[&>input]:pt-3',

      props.class,
    )"
  >
    <slot />
  </div>
</template>
