<template>
  <div :class="rootClass">
    <!-- One owner for the title, the right-side actions, and the body, so every
         surface lands on the same left and right edges. The title is inset (pl-2)
         to the section-label edge; the actions group carries no right inset, so it
         meets the body's right edge. That is what removes the 8px "actions don't
         line up with the cards below" drift — never hand-roll a <header> again.
         The title row reserves min-h-9 (the action-button height) even when there
         are no actions, so the title's vertical position is identical on every
         page — title-only, title+actions, or title+description all agree, and the
         title never "jumps" when you switch tabs. The description is a sibling
         BELOW that row (not inside it), so adding a subtitle grows the header
         downward without nudging the title off its shared baseline. -->
    <div class="mb-6">
      <div class="flex min-h-9 items-center justify-between gap-4">
        <h1 class="min-w-0 truncate pl-2 text-lg font-semibold">
          {{ title }}
        </h1>
        <div
          v-if="$slots.actions"
          class="flex shrink-0 items-center gap-2"
        >
          <slot name="actions" />
        </div>
      </div>
      <p
        v-if="description"
        class="mt-0.5 pl-2 text-sm text-muted-foreground"
      >
        {{ description }}
      </p>
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
