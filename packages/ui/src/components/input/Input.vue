<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { cn } from '#/lib/utils'

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes['class']
}>()

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <input
    v-model="modelValue"
    data-slot="input"
    :class="cn(
      'h-9 w-full min-w-0 rounded-lg border border-border bg-background text-base px-3 py-2  text-foreground shadow-sm!',
      'placeholder:text-muted-foreground',
      'transition-all outline-none',
      'focus:border-ring focus:ring-2 focus:ring-ring/20',
      'read-only:bg-muted read-only:text-muted-foreground read-only:cursor-not-allowed',
      'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
      'file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium',
      props.class,
    )"
  >
</template>
