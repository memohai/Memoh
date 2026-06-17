<script lang="ts" setup>
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { TextButton } from '#/components/text-button'

// A crumb is just a <TextButton> (ghost text affordance): muted at rest, fills +
// goes foreground on hover, padded hit area bigger than the text. The muted/hover
// colour is TextButton's own default, so a crumb adds nothing beyond defaulting
// `as` to <a>. Pass as-child to wrap a RouterLink/menu trigger.
const props = withDefaults(defineProps<PrimitiveProps & { class?: HTMLAttributes['class'] }>(), {
  as: 'a',
})
</script>

<template>
  <TextButton
    :as="as"
    :as-child="asChild"
    :class="props.class"
  >
    <slot />
  </TextButton>
</template>
