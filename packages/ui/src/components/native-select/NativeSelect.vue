<script setup lang="ts">
import type { AcceptableValue } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit, useVModel } from '@vueuse/core'
import { ChevronDownIcon } from 'lucide-vue-next'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<{ modelValue?: AcceptableValue | AcceptableValue[], class?: HTMLAttributes['class'], size?: 'sm' | 'default' | 'lg' }>(), {
  size: 'default',
})

const emit = defineEmits<{
  'update:modelValue': AcceptableValue
}>()

const modelValue = useVModel(props, 'modelValue', emit, {
  passive: true,
  defaultValue: '',
})

const delegatedProps = reactiveOmit(props, 'class', 'size')

// Size ladder mirrors the Select trigger (sm h-8 / default h-9 / lg h-10), tokenised
// to the same control type scale so a NativeSelect and a Select read identically.
const sizeClass = computed(() => ({
  sm: 'h-8 pl-3 text-body',
  default: 'h-9 pl-3 text-label',
  lg: 'h-10 pl-3.5 text-control',
}[props.size]))
</script>

<template>
  <div
    class="group/native-select relative w-fit has-[select:disabled]:opacity-40"
    data-slot="native-select-wrapper"
  >
    <select
      v-bind="{ ...$attrs, ...delegatedProps }"
      v-model="modelValue"
      data-slot="native-select"
      :data-size="props.size"
      :class="cn(
        'selection:bg-foreground selection:text-background w-full min-w-0 appearance-none rounded-md py-2 pr-9 tracking-[0.01em] outline-none disabled:pointer-events-none disabled:cursor-not-allowed',
        sizeClass,
        props.class,
      )"
    >
      <slot />
    </select>
    <ChevronDownIcon
      class="text-muted-foreground pointer-events-none absolute top-1/2 right-3.5 size-4 -translate-y-1/2 opacity-50 select-none"
      aria-hidden="true"
      data-slot="native-select-icon"
    />
  </div>
</template>
