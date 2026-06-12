<template>
  <section class="space-y-2.5">
    <div class="flex min-h-7 items-center justify-between gap-2 px-2">
      <h2 class="text-[13px] font-medium text-muted-foreground">
        {{ title }}
      </h2>
      <slot name="action" />
    </div>
    <div class="rounded-[var(--radius-menu-shell)] border border-border bg-card p-4">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  title: string
}>()
</script>
