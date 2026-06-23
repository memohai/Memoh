<script setup lang="ts">
import type { SelectContentEmits, SelectContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  injectSelectRootContext,
  SelectContent,
  SelectPortal,
  SelectViewport,
  useForwardPropsEmits,
} from 'reka-ui'
import { onBeforeUnmount, ref, watch } from 'vue'
import { menuContentClass, menuSlideClass, menuViewportClass } from '#/lib/menu'
import { cn } from '#/lib/utils'
import { SelectScrollDownButton, SelectScrollUpButton } from '.'

defineOptions({
  inheritAttrs: false,
})

// "Highlight the selected row on OPEN — once." reka focuses the selected item on
// open, but the highlight is cleared a frame later (the classic flash), so we hold it
// ourselves via [data-open-hint] (style.css) until the user actually interacts. The
// flag is reset on the REAL open state — NOT onMounted: this wrapper mounts once and
// reka toggles the panel internally, which is exactly why a second open used to flash.
// The handoff happens on a GENUINE pointer move (changed coords) or a key press; the
// open itself fires a spurious pointermove with UNCHANGED coords (popper re-positions /
// the selected row scrolls into view) which we ignore. After handoff the pointer owns
// the highlight and it never snaps back to the selected row when the cursor leaves.
const rootContext = injectSelectRootContext()
const openHint = ref(false)
let originX = Number.NaN
let originY = Number.NaN
function clearHint(): void {
  openHint.value = false
}
function onMenuPointerMove(e: PointerEvent): void {
  if (!openHint.value) return
  if (Number.isNaN(originX)) {
    originX = e.clientX
    originY = e.clientY
    return
  }
  if (e.clientX !== originX || e.clientY !== originY) clearHint()
}
watch(() => rootContext.open.value, (open) => {
  if (open) {
    openHint.value = true
    originX = Number.NaN
    originY = Number.NaN
    document.addEventListener('keydown', clearHint, true)
  }
  else {
    openHint.value = false
    document.removeEventListener('keydown', clearHint, true)
  }
}, { immediate: true })
onBeforeUnmount(() => document.removeEventListener('keydown', clearHint, true))

const props = withDefaults(
  defineProps<SelectContentProps & { class?: HTMLAttributes['class'], size?: 'sm' | 'default' }>(),
  {
    size: 'default',
    position: 'popper',
    // Keep the page SCROLLABLE while the menu is open. The scroll freeze is caused
    // solely by reka's bodyLock (it sets overflow:hidden on <body>), so we turn
    // ONLY that off. We deliberately LEAVE disableOutsidePointerEvents at reka's
    // default (true): it sets pointer-events:none on <body>, which does NOT block
    // wheel scrolling but keeps the rest of the page inert so an outside click
    // still dismisses. Turning it fully off (page-wide live) made reka's focus
    // dance with the trigger and flickered the row highlight. We instead re-enable
    // pointer-events on the TRIGGER ALONE in style.css (so its hover tracks the
    // pointer), leaving everything else inert. The popper still follows the trigger.
    bodyLock: false,
    // Shift the WHOLE menu left so the first row's text lands under the trigger
    // text — without touching the menu's internal proportions. The menu's text
    // sits at border(1)+p-1.5(6)+px-2.5(10)=17px from its edge; the trigger text
    // at px-3=12px. Delta = 5px, so the menu overhangs the trigger by 5px on the
    // start side (and, via the +8px viewport min-width below → +10px on the
    // bordered box, 5px on the end side too — wider than the button on both
    // sides, per the shared menu geometry).
    alignOffset: -5,
  },
)
const emits = defineEmits<SelectContentEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'size')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <SelectPortal>
    <SelectContent
      data-slot="select-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="cn(
        menuContentClass,
        'relative max-h-(--reka-select-content-available-height) min-w-[8rem]',
        position === 'popper'
          && cn(menuSlideClass, 'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1'),
        // A Select is a value CHOOSER, not an action menu: its rows track the
        // TRIGGER's type size (default 13px / sm 12px), never the 14px action-menu
        // size — so the open list reads as the same control, not a heavier menu.
        // Only the type scales; row padding/height stay comfortable (no cramping).
        size === 'sm'
          ? '[&_[data-slot=select-item]]:text-body'
          : '[&_[data-slot=select-item]]:text-label',
        props.class,
      )
      "
    >
      <SelectScrollUpButton />
      <SelectViewport
        data-slot="select-viewport"
        :data-open-hint="openHint ? '' : undefined"
        :class="cn(menuViewportClass, position === 'popper' && 'w-full min-w-[calc(var(--reka-select-trigger-width)_+_8px)] scroll-my-1')"
        @pointermove="onMenuPointerMove"
      >
        <slot />
      </SelectViewport>
      <SelectScrollDownButton />
    </SelectContent>
  </SelectPortal>
</template>
