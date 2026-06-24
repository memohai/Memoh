<script setup lang="ts">
import type { LabelProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { computed, inject } from 'vue'
import { cn } from '#/lib/utils'
import Label from '../label/Label.vue'
import { FIELD_INJECTION_KEY } from './context'

// Field label — same Label primitive, sized to the field's 13px rhythm. Inside a
// <Field> it auto-targets the control via `for`; pass `for` to override. Required
// `*` and "Optional" affordances are i18n-agnostic by design: pass `requiredText`
// / `optionalText` for a localized string, or slot the affordance directly. This
// keeps @memohai/ui free of any vue-i18n dependency (cf. Toaster's contract).
const props = defineProps<LabelProps & {
  class?: HTMLAttributes['class']
  required?: boolean
  optional?: boolean
  requiredText?: string
  optionalText?: string
}>()

const field = inject(FIELD_INJECTION_KEY, null)

const delegated = reactiveOmit(props, 'class', 'required', 'optional', 'requiredText', 'optionalText', 'for')
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
    <slot name="affordance">
      <span
        v-if="required"
        aria-hidden="true"
        class="text-destructive"
      >{{ requiredText ?? '*' }}</span>
      <span
        v-else-if="optional"
        class="text-muted-foreground text-caption font-normal"
      >{{ optionalText ?? '(optional)' }}</span>
    </slot>
  </Label>
</template>
