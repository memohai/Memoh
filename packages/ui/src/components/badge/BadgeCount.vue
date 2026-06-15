<script setup lang="ts">
import type { VariantProps } from 'class-variance-authority'
import type { HTMLAttributes } from 'vue'
import { cva } from 'class-variance-authority'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

// A count bubble is NOT a status chip: it's a reminder that has to be spotted at a
// glance against busy chrome (an icon corner, a list row). The soft low-saturation
// ramp that suits status Badges reads as "barely there" here, so counts use SOLID
// fills with a high-contrast numeral instead:
//   default     — neutral solid that flips with the theme (foreground/background),
//                 always crisp, for a plain "N items" count.
//   destructive — the saturated red alert dot (red-fill + white) for unread /
//                 attention-worthy counts; the canonical notification bubble.
// (No soft variant — if a count should whisper, it usually shouldn't be a bubble at
//  all but a plain muted numeral in the row.)
const badgeCountVariants = cva(
  // Circle sized in rem (≈15px at the default root) so the dot scales with the UI
  // font instead of staying a fixed pixel size: a single digit is a tight little dot
  // (min-w == h, no horizontal padding, so it stays a true circle, never an oval).
  // Horizontal padding is added per-content below — only multi-char values ("42",
  // "9+", "99+") get breathing room, because a fixed pad would inflate the single-digit
  // circle. font-bold keeps the tiny white numeral legible on the saturated fill;
  // leading-none + tabular-nums keep it crisp.
  'inline-flex items-center justify-center rounded-full h-[0.9375rem] min-w-[0.9375rem] text-caption font-bold font-sans tabular-nums leading-none',
  {
    variants: {
      variant: {
        default: 'bg-foreground text-background',
        destructive: 'bg-[var(--accent-red-fill)] text-[var(--accent-red-foreground)]',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

type BadgeCountVariants = VariantProps<typeof badgeCountVariants>

const props = defineProps<{
  count: number
  max?: number
  variant?: BadgeCountVariants['variant']
  class?: HTMLAttributes['class']
}>()

const displayCount = computed(() => {
  const max = props.max ?? 99
  return props.count > max ? `${max}+` : `${props.count}`
})

// Single digit stays a circle (padding would oval it); 2+ chars become a pill with
// real horizontal breathing room so "42" / "99+" don't read as cramped.
const padClass = computed(() => (displayCount.value.length > 1 ? 'px-1.5' : 'px-0'))
</script>

<template>
  <span
    data-slot="badge-count"
    :class="cn(badgeCountVariants({ variant }), padClass, props.class)"
  >
    {{ displayCount }}
  </span>
</template>
