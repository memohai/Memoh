<template>
  <div
    class="mx-4 flex min-h-[3.75rem] border-border py-3 last:border-b-0"
    :class="[rootClass, dividerClass]"
  >
    <!-- Leading media: an icon, avatar, channel mark, or a load skeleton that
         sits before the text. Only renders (and only claims its gap) when a
         caller fills it, so a plain label row is untouched. gap-3 to the body —
         media hugs its text; this is a different relationship from the wider
         gap-4 the body keeps from the trailing control, so the two don't share
         one value. -->
    <div
      v-if="$slots.leading"
      class="mr-3 flex shrink-0 items-center"
    >
      <slot name="leading" />
    </div>

    <!-- Body: either a fully custom block (#content, for status lines, nested
         meta, anything that isn't label+description) or the default
         label/description pair. min-w-0 so long values truncate instead of
         shoving the trailing control off the row. -->
    <div class="min-w-0 flex-1">
      <slot name="content">
        <div class="truncate text-sm font-medium text-foreground">
          {{ label }}
        </div>
        <p
          v-if="description"
          class="mt-0.5 text-xs text-muted-foreground"
        >
          {{ description }}
        </p>
      </slot>
    </div>

    <!-- Trailing control: the default slot. gap-4 (ml-4) from the body, the
         original row rhythm, so every existing caller keeps its spacing. When
         the row stacks on narrow screens, the control drops below the body and
         self-aligns to the left, so ml-4 only applies once side-by-side. -->
    <div
      v-if="$slots.default"
      class="shrink-0"
      :class="trailingClass"
    >
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label?: string
  description?: string
  // Cross-axis alignment of the row's columns. 'center' is the default settings
  // rhythm; 'start' top-aligns when the body is tall (multi-line object rows) so
  // a leading avatar and a trailing control pin to the first line.
  align?: 'center' | 'start'
  // Stacking of body vs. control. 'never' keeps the row horizontal at every
  // width (the default). 'sm' drops to a column below the sm breakpoint so a
  // label and its control don't cramp on a narrow pane, then rejoins one line
  // at sm. 'always' stays a column at every width — the shape a multi-line
  // field (a full-width Textarea under its label) needs, where the control is
  // never a beside-the-label affordance.
  stack?: 'never' | 'sm' | 'always'
  // A row can visually continue into the next row (for example, a mode picker
  // followed by fields owned by that mode) without drawing a divider between them.
  divider?: boolean
}>(), {
  label: '',
  description: '',
  align: 'center',
  stack: 'never',
  divider: true,
})

const dividerClass = computed(() => (props.divider ? 'border-b' : 'border-b-0'))

const rootClass = computed(() => {
  // Full literal class strings only — Tailwind scans source text, so a runtime
  // `sm:${align}` concat would never be generated. Enumerate every combination.
  if (props.stack === 'always') {
    // A permanently stacked field: label row over a full-width control. gap-3
    // is the label→control rhythm the house form column uses when a field
    // stacks, wider than the sm-collapse gap-2 because here it's the resting
    // state, not a narrow-screen fallback.
    return 'flex-col gap-3'
  }
  if (props.stack === 'sm') {
    return props.align === 'start'
      ? 'flex-col gap-2 sm:flex-row sm:items-start'
      : 'flex-col gap-2 sm:flex-row sm:items-center'
  }
  return props.align === 'start' ? 'items-start' : 'items-center'
})

// When stacked, the trailing control sits on its own line: no left inset, and
// it spans the body's full width so a Textarea/Input fills the row instead of
// hugging its content. ml-4 only makes sense once it's beside the body.
const trailingClass = computed(() => {
  switch (props.stack) {
    case 'always': return 'w-full'
    case 'sm': return 'sm:ml-4'
    default: return 'ml-4'
  }
})
</script>
