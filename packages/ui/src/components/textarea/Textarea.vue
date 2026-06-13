<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  defaultValue?: string | number
  modelValue?: string | number
  size?: 'sm' | 'default' | 'lg'
}>(), {
  size: 'default',
})

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  passive: true,
  defaultValue: props.defaultValue,
})

const sizeClass = computed(() => ({
  sm: 'min-h-14 px-2.5 py-1.5 text-[12px]',
  default: 'min-h-16 px-3 py-2 text-[13px]',
  lg: 'min-h-20 px-3.5 py-2.5 text-[14px]',
}[props.size]))
</script>

<template>
  <textarea
    v-model="modelValue"
    data-slot="textarea"
    :data-size="props.size"
    :class="cn(
      'flex field-sizing-content w-full rounded-md tracking-[0.01em] text-foreground',
      sizeClass,
      'outline-none resize-none',
      'disabled:cursor-not-allowed disabled:opacity-40',
      props.class
    )"
  />
</template>
