import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Toggle } from './Toggle.vue'

// Layout/focus/icon only — the hover/selected/press COLORS live in style.css as a
// gray-ladder keyed off [data-slot="toggle"][data-state] (single source = the
// promoted --ui-* tokens), so toggles can never drift from the rest of the system.
export const toggleVariants = cva(
  'inline-flex items-center justify-center gap-2 rounded-md text-body font-medium whitespace-nowrap select-none cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring/20 disabled:pointer-events-none disabled:opacity-40 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive [&_svg]:pointer-events-none [&_svg:not([class*=\'size-\'])]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        // gray-ladder fill: rest → hover 249 → press 237 → on 243 → on+press 231
        default: '',
        // color-only: active TINTS the icon, hover paints a calm 243 gray
        tint: '',
        outline: 'border border-border',
      },
      size: {
        default: 'h-9 px-2 min-w-9',
        sm: 'h-8 px-1.5 min-w-8',
        lg: 'h-10 px-2.5 min-w-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export const toggleVariantKeys = ['default', 'tint', 'outline'] as const satisfies readonly NonNullable<ToggleVariants['variant']>[]
export const toggleSizeKeys = ['default', 'sm', 'lg'] as const satisfies readonly NonNullable<ToggleVariants['size']>[]

export type ToggleVariants = VariantProps<typeof toggleVariants>
