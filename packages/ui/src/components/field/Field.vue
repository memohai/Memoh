<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { FieldContext } from './context'
import { useId } from 'reka-ui'
import { computed, provide, ref, toRef } from 'vue'
import { cn } from '#/lib/utils'
import { FIELD_INJECTION_KEY } from './context'

// Field wrapper: stacks label / control / description / error on one consistent
// rhythm AND wires them for accessibility. Children (FieldLabel, FieldControl,
// FieldDescription, FieldError) auto-link ids + aria-* via provided context.
//   <Field :invalid="!!err">
//     <FieldLabel>Email</FieldLabel>
//     <FieldControl><Input /></FieldControl>
//     <FieldDescription>We'll never share it.</FieldDescription>
//     <FieldError v-if="err">{{ err }}</FieldError>
//   </Field>
const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  orientation?: 'vertical' | 'horizontal'
  invalid?: boolean
  id?: string
}>(), {
  orientation: 'vertical',
  invalid: false,
})

const reactId = useId()
const baseId = computed(() => props.id ?? reactId)

const hasDescription = ref(false)
const hasError = ref(false)

const invalid = computed(() => props.invalid || hasError.value)

const descriptionId = computed(() => `${baseId.value}-description`)
const errorId = computed(() => `${baseId.value}-error`)

const describedBy = computed(() => {
  const ids: string[] = []
  if (hasDescription.value)
    ids.push(descriptionId.value)
  if (hasError.value)
    ids.push(errorId.value)
  return ids.length ? ids.join(' ') : undefined
})

provide<FieldContext>(FIELD_INJECTION_KEY, {
  id: baseId.value,
  descriptionId: descriptionId.value,
  errorId: errorId.value,
  invalid,
  describedBy,
  hasDescription,
  hasError,
})

// keep a stable string ref for template binding
const orientation = toRef(props, 'orientation')
</script>

<template>
  <div
    data-slot="field"
    :data-orientation="orientation"
    :data-invalid="invalid ? '' : undefined"
    :class="cn(
      'flex flex-col gap-1.5',
      'data-[orientation=horizontal]:flex-row data-[orientation=horizontal]:items-center data-[orientation=horizontal]:gap-3',
      props.class,
    )"
  >
    <slot />
  </div>
</template>
