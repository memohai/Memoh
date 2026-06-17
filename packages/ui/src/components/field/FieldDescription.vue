<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { inject, onBeforeUnmount, onMounted } from 'vue'
import { cn } from '#/lib/utils'
import { FIELD_INJECTION_KEY } from './context'

// Field helper text. Inside a <Field> it registers itself so the control's
// aria-describedby links to it, and adopts the field's description id.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const field = inject(FIELD_INJECTION_KEY, null)

onMounted(() => {
  if (field)
    field.hasDescription.value = true
})
onBeforeUnmount(() => {
  if (field)
    field.hasDescription.value = false
})
</script>

<template>
  <p
    :id="field?.descriptionId"
    data-slot="field-description"
    :class="cn('text-muted-foreground text-body leading-snug', props.class)"
  >
    <slot />
  </p>
</template>
