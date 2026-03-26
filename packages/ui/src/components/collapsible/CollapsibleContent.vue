<script setup lang="ts">
import type { CollapsibleContentProps } from 'reka-ui'
import { CollapsibleContent } from 'reka-ui'

const props = defineProps<CollapsibleContentProps>()
</script>

<template>
  <CollapsibleContent
    data-slot="collapsible-content"
    v-bind="props"
    class="*:border-b *:border-x *:first-of-type:border-t *:first-of-type:rounded-t-md *:last-of-type:rounded-b-md"
  >
    <slot />
  </CollapsibleContent>
</template>
