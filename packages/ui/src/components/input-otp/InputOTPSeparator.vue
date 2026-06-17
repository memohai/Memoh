<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { MinusIcon } from 'lucide-vue-next'
import { reactiveOmit } from '@vueuse/core'
import { useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<{ class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <div
    data-slot="input-otp-separator"
    role="separator"
    v-bind="forwarded"
    :class="cn('flex items-center justify-center text-muted-foreground', props.class)"
  >
    <slot>
      <MinusIcon class="size-4" />
    </slot>
  </div>
</template>
