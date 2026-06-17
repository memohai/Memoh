import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Item } from './Item.vue'
export { default as ItemActions } from './ItemActions.vue'
export { default as ItemContent } from './ItemContent.vue'
export { default as ItemDescription } from './ItemDescription.vue'
export { default as ItemFooter } from './ItemFooter.vue'
export { default as ItemGroup } from './ItemGroup.vue'
export { default as ItemHeader } from './ItemHeader.vue'
export { default as ItemMedia } from './ItemMedia.vue'
export { default as ItemSeparator } from './ItemSeparator.vue'
export { default as ItemTitle } from './ItemTitle.vue'

export const itemVariants = cva(
  // COLOR LANGUAGE (matches Card + lib/menu.ts, NOT a bespoke gray scheme):
  //  - At rest a row is TRANSPARENT and inherits whatever surface it sits on
  //    (card / popover / page) — never a baked bg-background, which would render
  //    grayer than a bg-card parent and read as "inside ≠ outside".
  //  - Semantics come from the STROKE (variant="outline" adds the hairline), not
  //    from fill color. A bare row carries no color of its own.
  //  - Interaction is OPT-IN and TRANSIENT: hover/press fills only arm when the
  //    row is a real control (as="a" / as="button"), pulling the same --ui-hover
  //    / --ui-pressed ladder as ghost buttons + menu rows. SELECTION is NOT a
  //    persistent row background — per the menu contract it is shown by an
  //    indicator (check / dot) supplied by the call site.
  'group/item flex items-center border border-transparent text-body rounded-lg transition-colors duration-100 [a]:cursor-pointer [button]:cursor-pointer [a]:hover:bg-[color:var(--ui-hover)] [button]:hover:bg-[color:var(--ui-hover)] [a]:active:bg-[color:var(--ui-pressed)] [button]:active:bg-[color:var(--ui-pressed)] flex-wrap outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] shadow-none',
  {
    variants: {
      variant: {
        // outline = a self-contained bordered card row (transparent fill + hairline
        // stroke that inherits the parent surface). This is the ONLY fill/stroke
        // variant: in our system the row's meaning comes from the STROKE, not a
        // bespoke color. A bare <Item> (no variant) is a transparent row meant to
        // live inside an <ItemGroup>, which supplies the surrounding chrome — that
        // is the primary usage. There is intentionally no `default` (an invisible
        // borderless+colorless standalone row is unusable) and no `muted` (color is
        // not how we differentiate rows).
        outline: 'border-border',
      },
      size: {
        default: 'p-4 gap-4 ',
        sm: 'py-3 px-4 gap-2.5',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
)

export const itemMediaVariants = cva(
  'flex shrink-0 items-center justify-center gap-2 group-has-[[data-slot=item-description]]/item:self-start [&_svg]:pointer-events-none group-has-[[data-slot=item-description]]/item:translate-y-0.5',
  {
    variants: {
      variant: {
        default: 'bg-transparent',
        icon: 'size-8 border rounded-sm bg-transparent text-muted-foreground [&_svg:not([class*=size-])]:size-4',
        image:
          'size-10 rounded-sm overflow-hidden [&_img]:size-full [&_img]:object-cover',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

export type ItemVariants = VariantProps<typeof itemVariants>
export type ItemMediaVariants = VariantProps<typeof itemMediaVariants>
