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
          'bg-[var(--accent-gray-soft-active)] text-[var(--accent-gray-deep)] [a&]:hover:bg-[var(--accent-gray-border)]',
        secondary:
          'bg-[var(--accent-gray-soft-hover)] text-[var(--accent-gray-deep)] [a&]:hover:bg-[var(--accent-gray-soft-active)]',
        destructive:
          'bg-[var(--accent-red-soft-active)] text-[var(--accent-red-deep)] [a&]:hover:bg-[var(--accent-red-border)]',
        success:
          'bg-[var(--accent-green-soft-active)] text-[var(--accent-green-deep)] [a&]:hover:bg-[var(--accent-green-border)]',
        warning:
          'bg-[var(--accent-yellow-soft-active)] text-[var(--accent-yellow-deep)] [a&]:hover:bg-[var(--accent-yellow-border)]',
        info:
          'bg-[var(--accent-blue-soft-active)] text-[var(--accent-blue-deep)] [a&]:hover:bg-[var(--accent-blue-border)]',
        outline:
          'border border-border bg-background text-foreground [a&]:hover:bg-accent',
      },
      size: {
        default: 'px-2 py-0.5',
        // px-[7px]: a hair more breathing room than px-1.5 (6px) so the compact chip
        // doesn't read as cramped, without growing to the full default px-2.
        sm: 'px-[7px] py-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
export type BadgeVariants = VariantProps<typeof badgeVariants>
