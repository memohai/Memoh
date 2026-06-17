<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

const props = withDefaults(defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes['class']
  // Edge emphasis on focus:
  //  - 'solid' (default): the hairline itself turns near-black (forms, dialogs)
  //  - 'subtle': the hairline barely deepens (search, low-chrome fields)
  emphasis?: 'solid' | 'subtle'
  // Height/padding/type scale. 'default' is the everyday form field; 'sm' is for
  // dense toolbars/tables; 'lg' is for prominent single-field moments.
  size?: 'sm' | 'default' | 'lg'
}>(), {
  size: 'default',
})

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  defaultValue: props.defaultValue,
})

const sizeClass = computed(() => ({
  sm: 'h-8 px-2.5 text-body',
  default: 'h-9 px-3 text-label',
  lg: 'h-10 px-3.5 text-control',
}[props.size]))
</script>

<template>
  <input
    v-model="modelValue"
    data-slot="input"
    :data-size="props.size"
    :data-emphasis="props.emphasis && props.emphasis !== 'solid' ? props.emphasis : undefined"
    :class="cn(
      'w-full min-w-0 rounded-md tracking-[0.01em] py-2 text-foreground',
      sizeClass,
      'outline-none',
      '[&:read-only:not(:disabled)]:bg-muted [&:read-only:not(:disabled)]:text-muted-foreground [&:read-only:not(:disabled)]:cursor-not-allowed',
      'disabled:pointer-events-none disabled:opacity-40',
      'file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-body file:font-medium',
      props.class,
    )"
  >
</template>
