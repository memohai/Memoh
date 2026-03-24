<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
  defaultValue?: string | number
  modelValue?: string | number
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
  <textarea
    v-model="modelValue"
    data-slot="textarea"
    :class="cn(
      'flex field-sizing-content min-h-16 w-full rounded-lg border border-border bg-background px-3 py-2 text-[16px] text-foreground',
      'placeholder:text-muted-foreground',
      'transition-all outline-none resize-none',
      'focus:border-ring focus:ring-2 focus:ring-ring/20',
      'disabled:cursor-not-allowed disabled:opacity-50',
      props.class
    )"
  />
</template>
