<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { Primitive } from 'reka-ui'
import { ChevronRight } from 'lucide-vue-next'
import { cn } from '#/lib/utils'

// ActionCard — a clickable card whose meaning is ACTION / going somewhere next,
// not displaying information. It wears the same white-card language (bg-card,
// one border-border hairline, card radius, flat) as every other card, but three
// things mark it as an entry point rather than a display box:
//   1. a required leading ICON (the #icon slot — the caller passes a lucide
//      component; the card never bakes in a specific glyph),
//   2. a required trailing forward chevron (overridable via #trailing, e.g. an
//      ArrowUpRight for an external link),
//   3. the WHOLE card reacts to the pointer — hover/press change the fill in
//      place across the entire surface.
//
// It is PRESENTATION-AGNOSTIC: it only says "this is an action entry" and emits
// a click / is a link. What the next surface is — an in-place slide swap
// (useViewSwap + SwapTransition), a focused Dialog, or an external URL — is the
// caller's choice. This is the component the contract's "named entry point →
// focused surface" pattern (SKILL §9/§12) was missing; reach for it instead of
// stashing whole features behind an in-card "Advanced" disclosure.
//
// Shape: a slim SINGLE-LINE entry row by default — title + icon + chevron, no
// description. `py-3.5` (14px) + the title's own line-height (`text-control`,
// 20px) gives exactly 48px with no forced floor: 14 + 20 + 14 = 48. This
// matches a reference single-line settings/link row measured at 608×48 with
// 14px/16px padding. The leading icon is bare at `size-4` (16px), the SAME rung
// as the trailing chevron — a bigger boxed icon (e.g. a size-8 tile sized to
// match an adjacent `Avatar`) would push the row back to ~60px, defeating this
// slim ratio. TRADE-OFF, on purpose, open for review: this card is no longer
// pinned to match `SettingsRow`'s 60px floor (used by title+description rows
// like Access Mode / Members), and an icon this size reads lighter next to a
// filled Avatar circle in an adjacent list. If a future spot needs BOTH a
// description AND avatar-parity weight, that is a different shape — don't
// force-fit it here; ask before stretching this one two ways at once.
//
// A `description`, if the caller supplies one, still renders (truncated to one
// line) and grows the row past 48px — natural content height, no artificial cap.
//
// Interaction: the hover/press fill is a NEUTRAL OVERLAY on a ::before shell
// (style.css, data-slot="action-card"), NOT a bg-* swap on the card — because
// --card ≠ --background, a bg-[--ui-hover] on the card body would replace the
// white fill and bleed the page surface through. The ::before overlay composites
// OVER bg-card (drawn above the fill, below the content), so the card darkens in
// light / lightens in dark with no per-scheme override. There is deliberately NO
// press-scale: a large content card must never lift or shrink (SKILL "no
// hover-rise"); the press reads through a deeper fill, like a table row. The
// fill uses a BESPOKE --action-card-hover / --action-card-active pair (not the
// shared overlay ladder) — ActionCard is the one control whose hover surface is a
// whole card, and at that area even the ladder's lightest rung read too strong on
// dark, so it gets its own measured-delta wash (see style.css :root). Transitions
// at a snappy 15ms.
//
// Leading icon color is `text-foreground` — the SAME color as the title, not
// `text-muted-foreground`. This card's hover is a whole-surface fill that gets
// only marginally darker/lighter (the neutral overlay ladder is deliberately
// subtle); a muted icon on top of that subtle shift loses contrast right when
// hover is supposed to confirm "yes, this is clickable." The trailing chevron
// stays muted — that is the system-wide convention for a directional affordance
// glyph (BackendCard, DetailPane, etc. all keep it quiet) — but the LEADING icon
// is the action's identity mark, so it reads at full title weight.
const props = withDefaults(defineProps<PrimitiveProps & {
  /** The action / destination name. Single line, truncates. */
  title: string
  /** Optional one-line supporting text (truncates — never wraps). Grows the row
   *  past the 48px single-line height. The #description slot overrides. */
  description?: string
  class?: HTMLAttributes['class']
}>(), {
  as: 'button',
  description: '',
})
</script>

<template>
  <Primitive
    data-slot="action-card"
    :as="as"
    :as-child="asChild"
    :class="cn(
      'group/action relative isolate flex w-full min-h-[3rem] items-center gap-3 border bg-card px-4 py-3.5 text-left',
      'cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
      props.class,
    )"
  >
    <!-- Leading icon: bare, size-4 (16px) — the SAME rung as the trailing
         chevron, never boxed/bordered (a boxed tile inside an already-bordered
         card stacks two strokes on one visual unit — the "dirty" pattern this
         whole language forbids). text-foreground, NOT muted — see the
         Interaction comment above for why. -->
    <span class="flex shrink-0 items-center justify-center text-foreground [&_svg]:size-4">
      <slot name="icon" />
    </span>

    <span class="min-w-0 flex-1">
      <span class="block truncate text-control font-medium text-foreground">
        {{ title }}
      </span>
      <span
        v-if="description || $slots.description"
        class="mt-0.5 block truncate text-body font-normal text-muted-foreground"
      >
        <slot name="description">{{ description }}</slot>
      </span>
    </span>

    <!-- Trailing: forward chevron by default; override with ArrowUpRight for an
         external link, or any affordance the destination calls for. -->
    <slot name="trailing">
      <ChevronRight class="size-4 shrink-0 text-muted-foreground" />
    </slot>
  </Primitive>
</template>
