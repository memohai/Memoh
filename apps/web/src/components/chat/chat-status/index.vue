<template>
  <section class="relative rounded-full p-0.5 border flex-none">
    <img
      src="/logo.svg"
      alt="logo.svg"
      class="w-6.5"
    >
    <LoaderCircle
      v-if="isLoading"
      class="absolute bottom-0 right-0 bg-card p-0.5 rounded-full size-[13px] text-primary animate-spin"
    />
    <CircleAlert
      v-if="error"
      class="absolute bottom-0 right-0 bg-card p-0.5 rounded-full size-[13px] text-muted-foreground"
    />
  </section>
</template>

<script setup lang="ts">
import { CircleAlert, LoaderCircle } from 'lucide-vue-next'

withDefaults(defineProps<{ isLoading?: boolean; error?: boolean }>(), {
  isLoading: true,
  error: false,
})
</script>