<script setup lang="ts">
// Section anchor + heading wrapper. Gives every wall section a consistent
// scroll target (id) and title for the left nav's IntersectionObserver.
defineProps<{
  id: string
  label: string
  description?: string
}>()
</script>

<template>
  <section
    :id="id"
    class="scroll-mt-20"
  >
    <div class="mb-4 border-b border-border pb-2">
      <h2 class="text-base font-semibold text-foreground">
        {{ label }}
      </h2>
      <p
        v-if="description"
        class="mt-0.5 text-xs text-muted-foreground"
      >
        {{ description }}
      </p>
    </div>
    <slot />
  </section>
</template>
