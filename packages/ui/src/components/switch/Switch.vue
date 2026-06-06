<script setup lang="ts">
import type { SwitchRootEmits, SwitchRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  SwitchRoot,
  SwitchThumb,
  useForwardPropsEmits,
} from 'reka-ui'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

const props = withDefaults(defineProps<SwitchRootProps & {
  class?: HTMLAttributes['class']
  size?: 'sm' | 'default'
}>(), {
  size: 'default',
})

const emits = defineEmits<SwitchRootEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'size')

const forwarded = useForwardPropsEmits(delegatedProps, emits)

// Geometry only. Both sizes are 20 tall with a 16 thumb (the 32×20 / 36×20
// references). Track is px-0.5 (2px) inset so the puck floats off the edge;
// translate = inner width − thumb. Color + transition live in style.css
// ([data-slot="switch"]) — same single source as Checkbox, so on=palette blue /
// off=gray never drifts per call site.
const trackClass = computed(() => ({
  sm: 'w-8 h-5',
  default: 'w-9 h-5',
}[props.size]))

const thumbClass = computed(() => ({
  sm: 'size-4 data-[state=checked]:translate-x-3',
  default: 'size-4 data-[state=checked]:translate-x-4',
}[props.size]))
</script>

<template>
  <SwitchRoot
    v-slot="slotProps"
    data-slot="switch"
    :data-size="props.size"
    v-bind="forwarded"
    :class="cn(
      'peer inline-flex shrink-0 cursor-pointer items-center rounded-full px-0.5 outline-none',
      'focus-visible:ring-2 focus-visible:ring-ring/20',
      'disabled:cursor-not-allowed disabled:opacity-40',
      trackClass,
      props.class,
    )"
  >
    <SwitchThumb
      data-slot="switch-thumb"
      :class="cn(
        'pointer-events-none block rounded-full data-[state=unchecked]:translate-x-0',
        thumbClass,
      )"
    >
      <slot
        name="thumb"
        v-bind="slotProps"
      />
    </SwitchThumb>
  </SwitchRoot>
</template>
