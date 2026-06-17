<template>
  <span class="inline-flex items-center">
    <span
      v-for="(icon, i) in icons"
      :key="i"
      class="-ml-[5px] flex size-[19px] items-center justify-center rounded-full border border-background bg-accent text-muted-foreground first:ml-0"
    >
      <component
        :is="icon"
        class="size-[11px]"
      />
    </span>
  </span>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{ icons: Component[] }>()
</script>
