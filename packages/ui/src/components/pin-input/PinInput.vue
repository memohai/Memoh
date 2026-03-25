<script setup lang="ts">
import type { PinInputRootEmits, PinInputRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { PinInputRoot, useForwardPropsEmits } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<PinInputRootProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<PinInputRootEmits>()

const delegatedProps = reactiveOmit(props, 'class')
const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <PinInputRoot
    v-bind="forwarded"
    data-slot="pin-input"
    :class="cn('flex items-center gap-2', props.class)"
  >
    <slot />
  </PinInputRoot>
</template>
