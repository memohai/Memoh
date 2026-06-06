<script setup lang="ts">
import { useVModel } from '@vueuse/core'
import { PopoverRoot } from 'reka-ui'
import { onBeforeUnmount, provide } from 'vue'
import { HOVER_CARD_INJECTION_KEY } from './context'

// We do NOT use reka's HoverCardRoot: it bakes in `useGraceArea`, an invisible
// convex-hull hit-zone that, for a card wider than its trigger, keeps the card
// open far on whichever side the card overhangs (lopsided "won't close" feel).
// Instead this is an Arkloop-style hover bridge built on the plain Popover
// primitive (Popover = anchor + portal + Popper positioning, no hover logic):
// the card opens on trigger enter and closes a short delay after the pointer
// leaves BOTH the trigger and the card, with re-entry cancelling the timer.
const props = withDefaults(
  defineProps<{
    /** Delay (ms) before opening once the pointer enters the trigger. */
    openDelay?: number
    /** Delay (ms) before closing after the pointer leaves trigger + content.
     *  This is purely a time bridge for the trigger→card hop, NOT a spatial
     *  hit-zone, so it stays small. */
    closeDelay?: number
    defaultOpen?: boolean
    open?: boolean
  }>(),
  {
    openDelay: 150,
    closeDelay: 120,
    defaultOpen: false,
    open: undefined,
  },
)
const emits = defineEmits<{ 'update:open': [value: boolean] }>()

const open = useVModel(props, 'open', emits, {
  defaultValue: props.defaultOpen,
  passive: (props.open === undefined) as false,
})

let openTimer: ReturnType<typeof setTimeout> | undefined
let closeTimer: ReturnType<typeof setTimeout> | undefined

function clearOpenTimer() {
  if (openTimer) {
    clearTimeout(openTimer)
    openTimer = undefined
  }
}
function clearCloseTimer() {
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = undefined
  }
}

function scheduleOpen() {
  clearCloseTimer()
  if (open.value)
    return
  clearOpenTimer()
  openTimer = setTimeout(() => {
    open.value = true
    openTimer = undefined
  }, props.openDelay)
}

function scheduleClose() {
  clearOpenTimer()
  clearCloseTimer()
  closeTimer = setTimeout(() => {
    open.value = false
    closeTimer = undefined
  }, props.closeDelay)
}

function cancelClose() {
  clearCloseTimer()
}

provide(HOVER_CARD_INJECTION_KEY, { scheduleOpen, scheduleClose, cancelClose })

onBeforeUnmount(() => {
  clearOpenTimer()
  clearCloseTimer()
})
</script>

<template>
  <PopoverRoot
    v-model:open="open"
    data-slot="hover-card"
  >
    <slot />
  </PopoverRoot>
</template>
