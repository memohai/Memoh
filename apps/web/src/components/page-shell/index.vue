<template>
  <div :class="rootClass">
    <!-- One owner for the title, the right-side actions, and the body, so every
         surface lands on the same left and right edges. The title is inset (pl-2)
         to the section-label edge; the actions group carries no right inset, so it
         meets the body's right edge. That is what removes the 8px "actions don't
         line up with the cards below" drift — never hand-roll a <header> again. -->
    <div class="mb-6 flex items-center justify-between gap-4">
      <div class="min-w-0 pl-2">
        <h1 class="truncate text-lg font-semibold">
          {{ title }}
        </h1>
        <p
          v-if="description"
          class="mt-0.5 text-sm text-muted-foreground"
        >
          {{ description }}
        </p>
      </div>
      <div
        v-if="$slots.actions"
        class="flex shrink-0 items-center gap-2"
      >
        <slot name="actions" />
      </div>
    </div>

    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title?: string
  description?: string
  // 'page' is a standalone surface that owns the full gutter. 'tab' lives inside the
  // bot-detail tab container (which already adds px-6 pt-4 pb-4), so it only adds the
  // remainder to reach the same pt-10/pb-12 vertical rhythm.
  variant?: 'page' | 'tab'
}>(), {
  variant: 'page',
})

const rootClass = computed(() =>
  props.variant === 'tab'
    ? 'mx-auto max-w-3xl pt-6 pb-8'
    : 'mx-auto max-w-3xl px-6 pt-10 pb-12',
)
</script>
