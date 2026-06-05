<script setup lang="ts">
import type { NumberFieldRootEmits, NumberFieldRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { Minus, Plus } from 'lucide-vue-next'
import {
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
  NumberFieldRoot,
  useForwardPropsEmits,
} from 'reka-ui'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

// Numeric input with stepper controls. Self-contained: decrement | input |
// increment inside the shared field edge. The edge (rest/focus/invalid) is
// driven from style.css via [data-slot="number-field"], same as the other fields.
const props = withDefaults(defineProps<NumberFieldRootProps & {
  class?: HTMLAttributes['class']
  placeholder?: string
  size?: 'sm' | 'default' | 'lg'
  // stepper button treatment:
  //   plain — bare ±, only the glyph color shifts on hover (lowest chrome)
  //   soft  — inset rounded-rect that fills on hover + presses on click
  stepper?: 'plain' | 'soft'
}>(), {
  size: 'default',
  stepper: 'plain',
})
const emits = defineEmits<NumberFieldRootEmits>()

const delegated = reactiveOmit(props, 'class', 'placeholder', 'size', 'stepper')
const forwarded = useForwardPropsEmits(delegated, emits)

const sizeClass = computed(() => ({
  sm: 'h-8 text-[12px]',
  default: 'h-9 text-[13px]',
  lg: 'h-10 text-[14px]',
}[props.size]))

const stepBase
  = 'flex shrink-0 items-center justify-center text-muted-foreground transition-[color,background-color,transform] duration-75 hover:text-foreground disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-3.5'

const decClass = computed(() => props.stepper === 'soft'
  ? cn(stepBase, 'my-1 ml-1 aspect-square h-auto rounded-[6px] hover:bg-accent active:scale-90')
  : cn(stepBase, 'h-full w-8 rounded-l-md'))

const incClass = computed(() => props.stepper === 'soft'
  ? cn(stepBase, 'my-1 mr-1 aspect-square h-auto rounded-[6px] hover:bg-accent active:scale-90')
  : cn(stepBase, 'h-full w-8 rounded-r-md'))
</script>

<template>
  <NumberFieldRoot
    v-bind="forwarded"
    data-slot="number-field"
    :data-size="props.size"
    :class="cn(
      'relative inline-flex w-full items-center rounded-md tracking-[0.01em] data-[disabled]:opacity-40',
      sizeClass,
      props.class,
    )"
  >
    <NumberFieldDecrement
      data-slot="number-field-decrement"
      :class="decClass"
    >
      <Minus />
    </NumberFieldDecrement>
    <NumberFieldInput
      data-slot="number-field-input"
      :placeholder="placeholder"
      class="w-full min-w-0 bg-transparent px-1 text-center tabular-nums text-foreground outline-none disabled:pointer-events-none"
    />
    <NumberFieldIncrement
      data-slot="number-field-increment"
      :class="incClass"
    >
      <Plus />
    </NumberFieldIncrement>
  </NumberFieldRoot>
</template>
