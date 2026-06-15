// Shared "select-trigger field" look — the single source for the closed-state
// trigger button. Used both by reka's SelectTrigger (packages/ui) and by the
// app-level Combobox, which is built on a Popover (not a reka Select) and so
// cannot reuse the SelectTrigger component itself, only its appearance.
//
// Deliberately omitted here, added by each consumer:
//  - width: Select wants `w-fit`, Combobox wants `w-full`.
//  - height/text scale: driven by the `data-size` attribute the consumer sets
//    (`data-size=sm|default|lg`), matching how reka's SelectTrigger sizes.
//  - placeholder tint: driven by a `data-placeholder` attribute (reka sets it on
//    SelectTrigger; the Combobox sets it manually when no value is selected).
export const selectTriggerClass
  = 'flex items-center justify-between gap-2 rounded-md px-3 py-2 tracking-[0.01em] whitespace-nowrap outline-none select-none '
    + 'data-[placeholder]:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-40 '
    + 'data-[size=sm]:h-8 data-[size=sm]:text-body data-[size=default]:h-9 data-[size=default]:text-label data-[size=lg]:h-10 data-[size=lg]:text-control data-[size=lg]:px-3.5 '
    + '*:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 '
    + '[&_svg:not([class*=text-])]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4'
