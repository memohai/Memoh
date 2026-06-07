<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { useForwardProps } from 'reka-ui'
import { computed } from 'vue'
import { useVueOTPContext } from 'vue-input-otp'
import { cn } from '#/lib/utils'

const props = defineProps<{ index: number, class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardProps(delegatedProps)

const context = useVueOTPContext()

const slot = computed(() => context?.value.slots[props.index])
</script>

<template>
  <div
    v-bind="forwarded"
    data-slot="input-otp-slot"
    :data-active="slot?.isActive"
    :class="cn(
      // Each cell is its OWN mini field box: the edge is the shared --field-edge
      // inset hairline from style.css (pixel-identical to <Input> — same mechanism,
      // so no border-miter corner darkening, no doubled dividers). Cells are spaced
      // by the group's gap, so every corner is rounded like a small input and the
      // active cell deepens its whole edge in place (data-active drives --field-edge).
      'relative flex h-8 w-8 items-center justify-center rounded-md bg-transparent text-body text-foreground',
      'outline-none',
      props.class,
    )"
  >
    {{ slot?.char }}
    <div
      v-if="slot?.hasFakeCaret"
      class="pointer-events-none absolute inset-0 flex items-center justify-center"
    >
      <div class="animate-caret-blink bg-foreground h-4 w-px duration-1000" />
    </div>
  </div>
</template>
