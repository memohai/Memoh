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
import { Button } from '#/components/button'
import { cn } from '#/lib/utils'

// Numeric input with stepper controls. Self-contained: − | input | + inside the
// shared field edge ([data-slot="number-field"] in style.css). The ± buttons are
// the design system's ghost Button (via reka as-child), so their hover fill and
// press live in one place and the glyph never moves on press.
const props = withDefaults(defineProps<NumberFieldRootProps & {
  class?: HTMLAttributes['class']
  placeholder?: string
  size?: 'sm' | 'default' | 'lg'
}>(), {
  size: 'default',
})
const emits = defineEmits<NumberFieldRootEmits>()

const delegated = reactiveOmit(props, 'class', 'placeholder', 'size')
const forwarded = useForwardPropsEmits(delegated, emits)

const sizeClass = computed(() => ({
  sm: 'h-8 text-[12px]',
  default: 'h-9 text-[13px]',
  lg: 'h-10 text-[14px]',
}[props.size]))

// Stepper box scales with field height; glyph stays a calm 14px. Radius is the
// shared in-field small-control radius — the same 5px (calc(--radius)-5px) the
// InputGroup clear/reveal buttons use — NOT a per-component concentric value:
// a free-floating ~28px hover chip reads too sharp at the geometric 4px, and
// keeping one value means every in-field control corner matches.
const stepClass = computed(() => cn(
  'p-0 rounded-[calc(var(--radius)-5px)] text-muted-foreground hover:text-foreground [&_svg]:size-3.5',
  { sm: 'size-6', default: 'size-7', lg: 'size-8' }[props.size],
))
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
    <NumberFieldDecrement as-child>
      <Button
        type="button"
        variant="ghost"
        tabindex="-1"
        :class="cn('ml-1', stepClass)"
      >
        <Minus />
      </Button>
    </NumberFieldDecrement>
    <NumberFieldInput
      data-slot="number-field-input"
      :placeholder="placeholder"
      class="w-full min-w-0 bg-transparent px-1 text-center tabular-nums text-foreground outline-none disabled:pointer-events-none"
    />
    <NumberFieldIncrement as-child>
      <Button
        type="button"
        variant="ghost"
        tabindex="-1"
        :class="cn('mr-1', stepClass)"
      >
        <Plus />
      </Button>
    </NumberFieldIncrement>
  </NumberFieldRoot>
</template>
