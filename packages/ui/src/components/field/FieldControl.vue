<script setup lang="ts">
import { Slot } from 'reka-ui'
import { inject } from 'vue'
import { FIELD_INJECTION_KEY } from './context'

// Binds the field's id + aria-describedby + aria-invalid onto the single slotted
// control (Input / Textarea / NumberField / Select…). No-op passthrough when used
// outside a <Field>.
const field = inject(FIELD_INJECTION_KEY, null)
</script>

<template>
  <Slot
    :id="field?.id"
    data-slot="field-control"
    :aria-describedby="field?.describedBy.value"
    :aria-invalid="field?.invalid.value ? true : undefined"
  >
    <slot />
  </Slot>
</template>
