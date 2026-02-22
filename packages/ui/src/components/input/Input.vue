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
      'h-9 w-full min-w-0 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100',
      'placeholder:text-gray-400 dark:placeholder:text-gray-500',
      'shadow-sm transition-all outline-none',
      'focus:border-primary focus:ring-2 focus:ring-primary/20',
      'read-only:bg-gray-50 dark:read-only:bg-gray-900/40 read-only:text-gray-500 dark:read-only:text-gray-400 read-only:cursor-not-allowed',
      'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
      'file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium',
      props.class,
    )"
  >
</template>
