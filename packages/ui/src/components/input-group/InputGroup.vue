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
      'min-w-0 has-[>textarea]:h-auto',

      // Size scale — height + inner control type, mirrors Input.
      'data-[size=sm]:h-8 data-[size=default]:h-9 data-[size=lg]:h-10',
      'data-[size=sm]:[&_[data-slot=input-group-control]]:text-[12px]',
      'data-[size=lg]:[&_[data-slot=input-group-control]]:text-[14px]',

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
