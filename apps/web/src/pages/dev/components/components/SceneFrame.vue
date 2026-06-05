<script setup lang="ts">
// Applies one candidate palette as CSS custom properties on a scoped wrapper.
// Everything inside reads the role vars (--surface, --hover, --fg, --brand, …);
// the shipping core tokens (--background, --card, --border, …) are also
// aliased onto the same element, so dropping a real @memohai/ui component inside
// later inherits the candidate palette with zero extra wiring.
//
// Self-contained: the scene controls its own light/dark via the `dark` prop, so
// it is independent of the wall's app-theme toolbar.
import { computed } from 'vue'
import type { Palette } from '../lib/palettes'

const props = defineProps<{
  palette: Palette
  dark: boolean
}>()

const vars = computed<Record<string, string>>(() => {
  const r = props.dark ? props.palette.dark : props.palette.light
  const b = props.dark ? props.palette.brand.dark : props.palette.brand.light
  return {
    // role vars — what the scene markup reads directly
    '--surface': r.surface,
    '--surface-sunken': r.surfaceSunken,
    '--hover': r.hover,
    '--selected': r.selected,
    '--border-role': r.border,
    '--fg': r.foreground,
    '--muted-fg': r.mutedForeground,
    '--brand': b.brand,
    '--brand-hover': b.brandHover,
    '--brand-soft': b.brandSoft,
    '--brand-foreground': b.brandForeground,
    // alias the shipping core tokens onto the roles (surfaces collapse to one)
    '--background': 'var(--surface)',
    '--card': 'var(--surface)',
    '--popover': 'var(--surface)',
    '--input': 'var(--border-role)',
    '--sidebar': 'var(--surface-sunken)',
    '--secondary': 'var(--hover)',
    '--muted': 'var(--hover)',
    '--accent': 'var(--hover)',
    '--border': 'var(--border-role)',
    '--foreground': 'var(--fg)',
    '--muted-foreground': 'var(--muted-fg)',
    '--ring': 'var(--brand)',
  }
})
</script>

<template>
  <div
    class="overflow-hidden rounded-xl border border-border"
    :class="dark ? 'dark' : ''"
    :style="{ ...vars, background: 'var(--surface)', color: 'var(--fg)' }"
  >
    <slot />
  </div>
</template>
