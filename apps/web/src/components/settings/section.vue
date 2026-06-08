<template>
  <section class="space-y-2.5">
    <h2
      v-if="title"
      class="px-2 text-[13px] font-medium text-muted-foreground"
    >
      {{ title }}
    </h2>
    <div class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  title?: string
}>(), {
  title: '',
})
</script>
