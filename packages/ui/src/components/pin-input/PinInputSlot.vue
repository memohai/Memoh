<script setup lang="ts">
import type { PinInputInputProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { PinInputInput } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<PinInputInputProps & {
  class?: HTMLAttributes['class']
}>()

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <PinInputInput
    v-bind="delegatedProps"
    data-slot="pin-input-slot"
    :class="cn(
      'size-9 bg-background text-center text-sm text-foreground shadow-none',
      'border-y border-r border-border',
      'transition-all outline-none',
      'focus:relative focus:z-10 focus:border-2 focus:border-ring',
      'first:rounded-l-lg first:border-l',
      'last:rounded-r-lg',
      'disabled:pointer-events-none disabled:opacity-50',
      'placeholder:text-muted-foreground',
      props.class,
    )"
  />
</template>
