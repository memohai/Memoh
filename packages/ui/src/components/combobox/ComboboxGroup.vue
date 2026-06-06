<script setup lang="ts">
import type { ComboboxGroupProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ComboboxGroup, ComboboxLabel } from 'reka-ui'
import { menuLabelClass } from '#/lib/menu'
import { cn } from '#/lib/utils'

const props = defineProps<ComboboxGroupProps & {
  class?: HTMLAttributes['class']
  heading?: string
}>()

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <ComboboxGroup
    data-slot="combobox-group"
    v-bind="delegatedProps"
    :class="cn('text-foreground flex flex-col gap-0.5 overflow-hidden', props.class)"
  >
    <ComboboxLabel
      v-if="heading"
      :class="menuLabelClass"
    >
      {{ heading }}
    </ComboboxLabel>
    <slot />
  </ComboboxGroup>
</template>
