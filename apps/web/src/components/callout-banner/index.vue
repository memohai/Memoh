<template>
  <!-- A framed warning/alert callout: leading icon + title/description body +
       trailing action(s). Two tones. `warning` uses the soft-token triplet
       (bg/border/foreground) the codebase already uses for lifecycle notices;
       `destructive` has no soft token in the scale, so it reuses the established
       alpha convention (border-destructive/30 bg-destructive/5) that the issue
       banners already ship — keeping one look, not inventing a second.
       Stacks on narrow, becomes a row at sm. When `clickable`, the whole surface
       is a button that opens something (a diagnostics dialog); the trailing slot
       is then usually empty and a chevron leads the user in. -->
  <component
    :is="clickable ? 'button' : 'div'"
    :type="clickable ? 'button' : undefined"
    class="flex flex-col gap-3 rounded-[var(--radius-menu-shell)] border px-4 py-3 text-left sm:flex-row sm:items-center"
    :class="[toneClass, clickable ? interactiveClass : '']"
  >
    <div class="flex min-w-0 flex-1 items-start gap-3">
      <slot name="icon">
        <AlertCircle
          class="mt-0.5 size-4 shrink-0"
          :class="iconClass"
        />
      </slot>
      <div class="min-w-0">
        <p class="text-sm font-medium text-foreground">
          {{ title }}
        </p>
        <p
          v-if="description"
          class="mt-0.5 text-xs text-muted-foreground"
        >
          {{ description }}
        </p>
      </div>
    </div>

    <!-- Trailing: a caller's action button(s) when not clickable, or a lead-in
         chevron when the whole banner is the affordance. -->
    <div class="flex shrink-0 items-center gap-2 sm:self-auto">
      <slot />
      <ChevronRight
        v-if="clickable"
        class="size-4 text-muted-foreground"
      />
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircle, ChevronRight } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  tone?: 'warning' | 'destructive'
  title: string
  description?: string
  clickable?: boolean
}>(), {
  tone: 'warning',
  description: '',
  clickable: false,
})

// Full literal class strings per tone — Tailwind scans source text, so a runtime
// concat would never be generated. warning has a soft-token triplet; destructive
// does not, so it reuses the established alpha convention.
const toneClass = computed(() =>
  props.tone === 'destructive'
    ? 'border-destructive/30 bg-destructive/5'
    : 'border-warning-border bg-warning-soft',
)

const iconClass = computed(() =>
  props.tone === 'destructive' ? 'text-destructive' : 'text-warning-foreground',
)

// When the whole banner is the affordance, it gets the neutral overlay hover the
// rest of the app's clickable surfaces use — the tile's own chrome, not a page
// injection.
const interactiveClass = 'w-full transition-colors hover:bg-accent' /* ui-allow-style */
</script>
