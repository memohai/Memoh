<script setup lang="ts">
import type { SelectContentEmits, SelectContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  SelectContent,
  SelectPortal,
  SelectViewport,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '#/lib/utils'
import { SelectScrollDownButton, SelectScrollUpButton } from '.'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<SelectContentProps & { class?: HTMLAttributes['class'] }>(),
  {
    position: 'popper',
    // Keep the page SCROLLABLE while the menu is open. The scroll freeze is caused
    // solely by reka's bodyLock (it sets overflow:hidden on <body>), so we turn
    // ONLY that off. We deliberately LEAVE disableOutsidePointerEvents at reka's
    // default (true): it sets pointer-events:none on <body>, which does NOT block
    // wheel scrolling but DOES make the trigger inert while the menu is open. That
    // inertness matters — with it off, the still-live trigger steals focus from
    // the row reka focuses on open, which made the highlight flicker on/off (and
    // left the trigger looking "triggered"). Matching shadcn's default here fixes
    // both. The popper still follows the trigger; an outside click still dismisses.
    bodyLock: false,
    // Shift the WHOLE menu left so the first row's text lands under the trigger
    // text — without touching the menu's internal proportions. The menu's text
    // sits at border(1)+p-1.5(6)+px-2.5(10)=17px from its edge; the trigger text
    // at px-3=12px. Delta = 5px, so the menu overhangs the trigger by 5px on the
    // start side (and, via the +8px viewport min-width below → +10px on the
    // bordered box, 5px on the end side too — wider than the button on both
    // sides, per the Scale reference).
    alignOffset: -5,
  },
)
const emits = defineEmits<SelectContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <SelectPortal>
    <SelectContent
      data-slot="select-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="cn(
        'bg-popover text-popover-foreground border border-[var(--border-menu)]',
        'duration-100 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-[0.98] data-[state=open]:zoom-in-[0.98]',
        'data-[side=bottom]:slide-in-from-top-1 data-[side=left]:slide-in-from-right-1 data-[side=right]:slide-in-from-left-1 data-[side=top]:slide-in-from-bottom-1',
        'relative z-50 max-h-(--reka-select-content-available-height) min-w-[8rem] overflow-x-hidden overflow-y-auto rounded-menu-shell shadow-[var(--shadow-dropdown)]',
        position === 'popper'
          && 'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
        props.class,
      )
      "
    >
      <SelectScrollUpButton />
      <SelectViewport :class="cn('flex flex-col gap-0.5 p-1.5', position === 'popper' && 'w-full min-w-[calc(var(--reka-select-trigger-width)_+_8px)] scroll-my-1')">
        <slot />
      </SelectViewport>
      <SelectScrollDownButton />
    </SelectContent>
  </SelectPortal>
</template>
