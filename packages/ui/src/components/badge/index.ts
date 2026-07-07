import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Badge } from './Badge.vue'
export { default as BadgeCount } from './BadgeCount.vue'

export const badgeVariants = cva(
  // A calm status chip: pill, flat soft-tint surface, NO border (only `outline` opts
  // back into a hairline). Colours come from the accent palette's soft ramp — a
  // low-saturation -soft-active fill paired with the same-hue -deep text. This auto
  // adapts in dark mode (the ramp flips to a translucent dark tint + light text), so
  // one set of tokens covers both themes. Box metrics (px/py) live in `size`; text
  // size is size-invariant (text-caption) so it sits here in the base.
  'inline-flex items-center justify-center rounded-full text-caption font-medium w-fit whitespace-nowrap shrink-0 [&>svg]:size-3 gap-1 [&>svg]:pointer-events-none transition-colors overflow-hidden',
  {
    variants: {
      variant: {
        default:
          'bg-accent-gray-soft-active text-accent-gray-deep [a&]:hover:bg-accent-gray-border',
        secondary:
          'bg-accent-gray-soft-hover text-accent-gray-deep [a&]:hover:bg-accent-gray-soft-active',
        destructive:
          'bg-accent-red-soft-active text-accent-red-deep [a&]:hover:bg-accent-red-border',
        success:
          'bg-accent-green-soft-active text-accent-green-deep [a&]:hover:bg-accent-green-border',
        warning:
          'bg-accent-yellow-soft-active text-accent-yellow-deep [a&]:hover:bg-accent-yellow-border',
        info:
          'bg-accent-blue-soft-active text-accent-blue-deep [a&]:hover:bg-accent-blue-border',
        outline:
          'border border-border bg-background text-foreground [a&]:hover:bg-accent',
      },
      size: {
        default: 'px-2 py-0.5',
        // px-[0.4375rem] (≈7px): a hair more breathing room than px-1.5 (6px) so the
        // compact chip doesn't read as cramped, without growing to the full default px-2.
        sm: 'px-[0.4375rem] py-0',
      },
      // Opt-in monospace for chips that carry a technical value — a match score,
      // a cron pattern, a platform/provider key. Orthogonal to the base font-medium
      // (that sets weight, this sets family), so mono chips stay medium-weight. Kept
      // as a variant rather than left to callers' `class` so the tool-call surface's
      // "technical metadata reads as mono" convention lands on one owner instead of
      // being hand-rolled per detail panel.
      font: {
        sans: '',
        mono: 'font-mono',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
      font: 'sans',
    },
  },
)
export type BadgeVariants = VariantProps<typeof badgeVariants>
