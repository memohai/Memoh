<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { CircleAlert } from 'lucide-vue-next'
import { cn } from '#/lib/utils'

// Standalone field-level error line: alert glyph + red text.
// Pairs with any control (Input/Textarea/Select…) when you need to warn the
// user inline. For vee-validate forms use <FormMessage>, which renders the
// same shape automatically.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <p
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
