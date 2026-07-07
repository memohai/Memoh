<script setup lang="ts">
import type { PopoverContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { PopoverContent, PopoverPortal, useForwardProps } from 'reka-ui'
import { injectHoverCardContext } from './context'
import { menuSlideClass } from '#/lib/menu'
import { cn } from '#/lib/utils'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<PopoverContentProps & { class?: HTMLAttributes['class'] }>(),
  {
    // align START + alignOffset −16: open down-and-to-the-right, but aligned TEXT-to-TEXT
    // rather than edge-to-edge. The hit-zone is now ONLY this element + the trigger (see
    // context.ts) — no convex hull — so alignment is a pure layout choice. Design intent:
    // the trigger word and the card's body copy should share one left rail (minimal eye
    // travel). The card has p-4 (16px) padding, so its TEXT sits 16px in from its left
    // edge; pulling the whole card left by that padding (alignOffset −16) drops the card
    // text onto the trigger text's column. Keep this in sync with the content padding.
    align: 'start',
    alignOffset: -16,
    sideOffset: 4,
  },
)

const delegatedProps = reactiveOmit(props, 'class')
const forwarded = useForwardProps(delegatedProps)
const ctx = injectHoverCardContext()
</script>

<template>
  <PopoverPortal>
    <!-- Floating SURFACE shared with the menu family: same tokenised hairline
         (--border-menu), shell radius (rounded-menu-shell) and dropdown shadow as
         Select / DropdownMenu / Combobox, with a fast fade + 1-unit directional
         slide at duration-75 (no zoom). The pointerenter/leave pair is the other
         half of the hover bridge: entering the card cancels the pending
         close, leaving it arms the close timer. open-auto-focus is prevented so a
         hover preview never steals focus from the page. -->
    <PopoverContent
      data-slot="hover-card-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="cn(
        'bg-popover text-popover-foreground z-(--z-overlay) w-64 rounded-menu-shell border border-[color:var(--border-menu)] p-4 shadow-[var(--shadow-dropdown)] outline-hidden',
        'origin-(--reka-popover-content-transform-origin)',
        'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 duration-75',
        menuSlideClass,
        props.class,
      )"
      @open-auto-focus.prevent
      @pointerenter="ctx.cancelClose"
      @pointerleave="ctx.scheduleClose"
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>
