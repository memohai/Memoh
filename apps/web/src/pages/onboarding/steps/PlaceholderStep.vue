<script setup lang="ts">
defineProps<{
  heading?: boolean
  titleKey: string
  descKey: string
}>()
</script>

<template>
  <div class="text-center">
    <h1 v-if="heading" class="text-2xl font-bold mb-3">
      {{ $t(titleKey) }}
    </h1>
    <h2 v-else class="text-xl font-semibold mb-4">
      {{ $t(titleKey) }}
    </h2>
    <p class="text-muted-foreground text-sm">
      {{ $t(descKey) }}
    </p>
  </div>
</template>
