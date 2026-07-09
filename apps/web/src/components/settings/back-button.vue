<template>
  <!-- THE back affordance for in-page detail surfaces (DetailPane and the
       bot-detail tabs that can't use DetailPane). One component so the
       geometry below is tuned in exactly one place — this used to be
       copy-pasted into every caller and drifted.

       Geometry (both values coupled, derived together):
         px-4  — a roomier chip: 16px of air on each side of the
                 chevron+label, replacing the default svg padding (12px).
         -ml-4 — shifts the button left by exactly that padding, so the
                 CHEVRON's left pixel lands on the wrapper's content edge —
                 the same x as the cards below (optical alignment: the glyph
                 aligns, the hover chip overhangs into the gutter).
       Keep them equal-and-opposite; re-derive both if one changes. -->
  <Button
    variant="ghost"
    :class="buttonClass"
    @click="emit('click')"
  >
    <ChevronLeft class="size-4 shrink-0" />
    <span class="min-w-0 truncate">{{ label }}</span>
  </Button>
</template>

<script setup lang="ts">
import { Button } from '@felinic/ui'
import { ChevronLeft } from 'lucide-vue-next'

defineProps<{
  label: string
}>()

const emit = defineEmits<{ click: [] }>()

// Geometry rationale in the template comment above. The /85 dim is tuned for
// this sole owner — one consumer doesn't earn a global -soft token.
const buttonClass = '-ml-4 max-w-full px-4 text-foreground/85' /* ui-allow-alpha: sole owner, see comment above */
</script>
