import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Button } from './Button.vue'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-control font-medium transition-all disabled:pointer-events-none disabled:opacity-40 data-[loading]:opacity-100 [&_svg]:pointer-events-none [&_svg:not([class*=\'size-\'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer',
  {
    variants: {
      variant: {
        // DEFAULT is the app-wide high-emphasis button. Keep it visually identical
        // to `primary` so existing <Button> / variant="default" call sites do not
        // keep the old bg-secondary button around.
        default:
          'text-background',
        destructive:
          'text-foreground transition-colors duration-150 hover:bg-[#FAE2E1] hover:text-[#c0271e]',
        // OUTLINE is the app-wide bordered/standalone action. Keep it visually
        // identical to `secondary`; the ring/fill/press model lives in style.css.
        outline:
          'bg-transparent text-foreground',
        // SECONDARY = ported verbatim from the tuned bench: transparent at rest
        // with a 1px inset ring; hover fills + ring drops; press scales. ALL of
        // that lives on the ::before shell in style.css (data-variant="secondary")
        // — the old bg-accent/border library look is intentionally cut.
        secondary: 'bg-transparent text-foreground',
        // GHOST = ported verbatim: no chrome at rest; hover fills gray-3, press
        // scales. Interaction on ::before (style.css, data-variant="ghost").
        ghost: 'text-foreground',
        // LINK variants mirror the contract bench:
        // - link: fade-in underline on hover
        // - link-static: underline always visible, only color brightens
        // - link-draw: underline draws left-to-right, reserved for rare landing CTAs
        link:
          'relative gap-1 text-muted-foreground transition-colors hover:text-foreground',
        'link-static':
          'gap-1 text-muted-foreground underline underline-offset-4 decoration-muted-foreground transition-colors hover:text-foreground hover:decoration-foreground',
        'link-draw':
          'relative gap-1 text-muted-foreground hover:text-foreground',
        // PRIMARY = high-emphasis CHARCOAL CTA (the tuned star). Fill + ALL
        // interaction (hover lighten, press scale / block color-press, loading
        // hold) live on a ::before shell in style.css, keyed off
        // data-variant="primary", so the press-scale never moves the label.
        primary: 'text-background',
        // BRAND = the scheme brand color. This used to be `primary`; it's now an
        // explicit variant for the rare brand CTA (e.g. chat Send) per the
        // brand-scarcity rule. Migrate brand call sites to variant="brand".
        brand: 'bg-brand text-brand-foreground hover:bg-brand-hover',
      },
      size: {
        default: 'h-9 px-4 py-2 has-[>svg]:px-3',
        sm: 'h-8 rounded-md gap-1.5 px-3 has-[>svg]:px-2.5',
        lg: 'h-10 rounded-md px-6 has-[>svg]:px-4',
        icon: 'size-9',
        'icon-sm': 'size-8',
        'icon-lg': 'size-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
export type ButtonVariants = VariantProps<typeof buttonVariants>

// Single source of truth for the variant/size axes. cva 0.7.1 does not expose
// its `.config` at runtime, so the keys are mirrored here next to the cva call
// (keep them in sync). Consumed by Storybook stories and the dev component wall
// so neither hand-maintains its own list.
export const buttonVariantKeys = [
  'default',
  'primary',
  'secondary',
  'outline',
  'ghost',
  'destructive',
  'link',
  'link-static',
  'link-draw',
  'brand',
] as const satisfies readonly NonNullable<ButtonVariants['variant']>[]

export const buttonSizeKeys = [
  'default',
  'sm',
  'lg',
  'icon',
  'icon-sm',
  'icon-lg',
] as const satisfies readonly NonNullable<ButtonVariants['size']>[]
