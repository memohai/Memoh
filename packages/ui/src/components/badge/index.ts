import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Badge } from './Badge.vue'
export { default as BadgeCount } from './BadgeCount.vue'

export const badgeVariants = cva(
  'inline-flex items-center justify-center rounded-sm border px-2.5 py-0.5 text-xs font-medium w-fit whitespace-nowrap shrink-0 [&>svg]:size-3 gap-1 [&>svg]:pointer-events-none transition-all overflow-hidden',
  {
    variants: {
      variant: {
        default:
          'border-border bg-secondary text-secondary-foreground [a&]:hover:bg-secondary/80',
        secondary:
          'border-transparent bg-accent text-foreground [a&]:hover:bg-accent/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground [a&]:hover:bg-destructive/90',
        success:
          'border-transparent bg-success text-success-solid-foreground [a&]:hover:bg-success/90',
        warning:
          'border-transparent bg-warning text-warning-solid-foreground [a&]:hover:bg-warning/90',
        outline:
          'text-foreground border-border bg-background [a&]:hover:bg-accent',
      },
      size: {
        default: 'px-2.5 py-0.5 text-xs',
        sm: 'px-2 py-0 text-[11px]',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
export type BadgeVariants = VariantProps<typeof badgeVariants>
