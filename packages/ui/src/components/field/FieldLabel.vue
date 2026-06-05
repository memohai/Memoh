<script setup lang="ts">
import type { LabelProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { cn } from '#/lib/utils'
import Label from '../label/Label.vue'

// Field label — same Label primitive, sized to sit on the field's 13px rhythm.
// Pass `for` to wire it to the control's id.
const props = defineProps<LabelProps & { class?: HTMLAttributes['class'] }>()

const delegated = reactiveOmit(props, 'class')
</script>

<template>
  <Label
    data-slot="field-label"
    v-bind="delegated"
    :class="cn('text-[13px]', props.class)"
  >
    <slot />
  </Label>
</template>
