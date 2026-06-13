<script setup lang="ts">
// Renders one token family as a labeled grid of swatches.
import TokenSwatch from './TokenSwatch.vue'
import type { TokenGroup } from '../lib/token-catalog'

defineProps<{
  group: TokenGroup
}>()
</script>

<template>
  <div class="flex flex-col gap-2">
    <h3 class="text-xs font-medium text-muted-foreground">
      {{ group.label }}
    </h3>
    <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      <TokenSwatch
        v-for="token in group.tokens"
        :key="token"
        :name="token"
      />
    </div>
  </div>
</template>
