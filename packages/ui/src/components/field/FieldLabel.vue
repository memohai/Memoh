<script setup lang="ts">
import type { LabelProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { computed, inject } from 'vue'
import { cn } from '#/lib/utils'
import Label from '../label/Label.vue'
import { FIELD_INJECTION_KEY } from './context'

// Field label — same Label primitive, sized to the field's 13px rhythm. Inside a
// <Field> it auto-targets the control via `for`; pass `for` to override. Optional
// required `*` / "Optional" affordances.
const props = defineProps<LabelProps & {
  class?: HTMLAttributes['class']
  required?: boolean
  optional?: boolean
}>()

const field = inject(FIELD_INJECTION_KEY, null)

const delegated = reactiveOmit(props, 'class', 'required', 'optional', 'for')
const forAttr = computed(() => props.for ?? field?.id)
</script>

<template>
  <Label
    :for="forAttr"
    data-slot="field-label"
    v-bind="delegated"
    :class="cn('gap-1 text-label', props.class)"
  >
    <slot />
    <span
      v-if="required"
      aria-hidden="true"
      class="text-destructive"
    >*</span>
    <span
      v-else-if="optional"
      class="text-muted-foreground text-caption font-normal"
    >(optional)</span>
  </Label>
</template>
