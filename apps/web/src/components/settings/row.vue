<template>
  <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
    <div class="min-w-0">
      <div class="text-sm font-medium text-foreground">
        {{ label }}
      </div>
      <p
        v-if="description"
        class="mt-0.5 text-xs text-muted-foreground"
      >
        {{ description }}
      </p>
    </div>
    <div class="shrink-0">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  label: string
  description?: string
}>(), {
  description: '',
})
</script>
