<script setup lang="ts">
import type { ProgressRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ProgressIndicator, ProgressRoot } from 'reka-ui'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

const props = withDefaults(
  defineProps<ProgressRootProps & { class?: HTMLAttributes['class'] }>(),
  { modelValue: 0 },
)

const delegatedProps = reactiveOmit(props, 'class')

// reka leaves modelValue null while indeterminate; clamp to a number for the
// translate so the bar simply sits empty until a real value arrives.
const value = computed(() => props.modelValue ?? 0)
</script>

<template>
  <!-- Track + fill reuse the slider language: the neutral rail (--slider-track,
       which carries its own dark-mode value) and the one selection blue
       (--accent-blue-fill) shared by every selection control, so a progress bar
       reads as the same family as Slider/Switch/Checkbox. h-1.5 matches the slider
       rail height. -->
  <ProgressRoot
    data-slot="progress"
    v-bind="delegatedProps"
    :class="cn('relative h-1.5 w-full overflow-hidden rounded-full bg-[color:var(--slider-track)]', props.class)"
  >
    <ProgressIndicator
      data-slot="progress-indicator"
      class="size-full flex-1 rounded-full bg-[color:var(--accent-blue-fill)] transition-transform duration-300 ease-out will-change-transform"
      :style="`transform: translateX(-${100 - value}%);`"
    />
  </ProgressRoot>
</template>
