import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as ButtonGroup } from './ButtonGroup.vue'
export { default as ButtonGroupSeparator } from './ButtonGroupSeparator.vue'
export { default as ButtonGroupText } from './ButtonGroupText.vue'

export const buttonGroupVariants = cva(
  // The group owns ONE unified edge: a flat (no-shadow) outer border + rounded
  // corners, with internal dividers between items (orientation-aware below — a
  // horizontal group divides with border-r, a vertical group with border-b).
  // Height comes from the children (buttons carry their own size) — the group has
  // no size of its own. Grouped buttons also drop their standalone ::before chrome
  // (hairline ring + press-scale) in style.css so the group border is the single
  // edge and a press never pulls the fill away from a fixed divider.
  // ui-allow-z: z-10 below is NOT on the z-index ladder (packages/ui/AGENTS.md
  // "z 梯"): it only lifts a focused item's ring above its own unfocused
  // siblings inside ONE button-group (so the shared divider border doesn't
  // clip the ring) — it never competes with another component's z-index, so
  // a global token would overstate its scope.
  'bg-inherit border border-border rounded-lg flex w-fit items-stretch [&>*]:focus-visible:z-10 [&>*]:focus-visible:relative [&>[data-slot=select-trigger]:not([class*=\'w-\'])]:w-fit [&>input]:flex-1 has-[select[aria-hidden=true]:last-child]:[&>[data-slot=select-trigger]:last-of-type]:rounded-r-md has-[>[data-slot=button-group]]:gap-2',
  {
    variants: {
      orientation: {
        horizontal:
          '[&>*:not(:last-child)]:border-r [&>*:not(:last-child)]:border-border [&>*:not(:first-child)]:rounded-l-none [&>*:not(:first-child)]:border-l-0 [&>*:not(:last-child)]:rounded-r-none',
        vertical:
          'flex-col [&>*:not(:last-child)]:border-b [&>*:not(:last-child)]:border-border [&>*:not(:first-child)]:rounded-t-none [&>*:not(:first-child)]:border-t-0 [&>*:not(:last-child)]:rounded-b-none',
      },
    },
    defaultVariants: {
      orientation: 'horizontal',
    },
  },
)

export type ButtonGroupVariants = VariantProps<typeof buttonGroupVariants>
