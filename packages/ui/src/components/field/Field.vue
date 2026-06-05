<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Presentational field wrapper: stacks label / control / description / error on
// one consistent rhythm so every form row is tuned here, not ad-hoc per page.
// Compose freely:
//   <Field>
//     <FieldLabel for="x">Email</FieldLabel>
//     <Input id="x" />
//     <FieldDescription>We'll never share it.</FieldDescription>
//     <FieldError>Email is not valid.</FieldError>
//   </Field>
const props = defineProps<{
  class?: HTMLAttributes['class']
  orientation?: 'vertical' | 'horizontal'
}>()
</script>

<template>
  <div
    data-slot="field"
    :data-orientation="orientation ?? 'vertical'"
    :class="cn(
      'flex flex-col gap-1.5',
      'data-[orientation=horizontal]:flex-row data-[orientation=horizontal]:items-center data-[orientation=horizontal]:gap-3',
      props.class,
    )"
  >
    <slot />
  </div>
</template>
