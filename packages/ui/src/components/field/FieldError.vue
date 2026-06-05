<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { CircleAlert } from 'lucide-vue-next'
import { inject, onBeforeUnmount, onMounted } from 'vue'
import { cn } from '#/lib/utils'
import { FIELD_INJECTION_KEY } from './context'

// Field-level error line: alert glyph + red text. Pairs with any control. Inside
// a <Field> it registers itself (so the control becomes aria-invalid + described
// by it) and adopts the field's error id. For vee-validate forms use <FormMessage>,
// which renders the same shape automatically.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const field = inject(FIELD_INJECTION_KEY, null)

onMounted(() => {
  if (field)
    field.hasError.value = true
})
onBeforeUnmount(() => {
  if (field)
    field.hasError.value = false
})
</script>

<template>
  <p
    :id="field?.errorId"
    data-slot="field-error"
    :class="cn(
      'text-destructive flex items-center gap-1.5 text-[13px] leading-snug',
      props.class,
    )"
  >
    <CircleAlert class="size-3.5 shrink-0" />
    <slot />
  </p>
</template>
