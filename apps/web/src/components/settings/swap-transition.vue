<template>
  <!-- Directional push-pop for the list <-> detail swap. No `mode`, so the
       leaving and entering panes animate at the SAME time and cross-slide; the
       leaving pane is pulled out of flow (position:absolute, via the keyframes
       in style.css) so it doesn't shove the entering pane around. No `appear`:
       the first paint (landing from the sidebar) is a plain cut, like
       Appearance / Profile — only the swap moves. -->
  <!-- overflow-x-clip: the slide pushes panes ±24px horizontally; clip (not
       hidden) keeps that from flashing a horizontal scrollbar WITHOUT turning
       this into a vertical scroll container — vertical scrolling stays owned by
       the ancestor scroll area (see the scroll-restore block in the script for
       why that ancestor is exactly the thing this component has to reach into). -->
  <div
    ref="rootEl"
    class="relative overflow-x-clip"
  >
    <Transition
      :name="direction === 'back' ? 'swap-back' : 'swap-forward'"
      @before-enter="onBeforeEnter"
    >
      <slot />
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { SwapDirection } from '@/composables/useViewSwap'

const props = defineProps<{ direction: SwapDirection }>()

const rootEl = ref<HTMLElement | null>(null)

// The list and detail panes are both v-if branches of the SAME caller
// component, so they never leave the page's real vertical scroll container —
// that container is an ANCESTOR outside this component entirely (settings
// pages scroll their shared `<section overflow-y-auto>` in settings-section/
// index.vue; a bot's detail tabs scroll their own `overflow-y-auto` pane in
// bots/detail.vue). Swapping the slot content does nothing to that ancestor's
// scrollTop, so a detail view opened while the list was scrolled down used to
// inherit the list's scroll offset outright — the detail's own back button
// could land off-screen above the fold. This component owns the fix centrally
// (not each SwapTransition caller) so every current and future list<->detail
// pair — bot-access, memory's 3-level nav, appearance, whatever comes next —
// gets correct scroll behavior for free just by using SwapTransition.
//
// Behavior: forward always opens the next pane at the top (a fresh surface
// shouldn't inherit a scroll offset that has nothing to do with it); back
// restores the exact offset the list/parent view had before the user drilled
// in (iOS-style: pushing a screen remembers where you were, popping returns
// you there). A plain stack (not a single saved value) makes this correct for
// ANY nesting depth without special-casing — memory's main -> backends ->
// detail is two forward pushes and two back pops of the same mechanism.
let scrollEl: HTMLElement | null = null
const scrollStack: number[] = []

function findScrollAncestor(el: HTMLElement | null): HTMLElement | null {
  let node = el?.parentElement ?? null
  while (node && node !== document.body) {
    const overflowY = getComputedStyle(node).overflowY
    if (overflowY === 'auto' || overflowY === 'scroll') return node
    node = node.parentElement
  }
  return null
}

onMounted(() => {
  scrollEl = findScrollAncestor(rootEl.value)
})

// Captured on a flush:'pre' watcher (guaranteed to run before this component's
// own re-render) rather than the Transition's @before-leave hook — with no
// `mode`, the leaving pane's and entering pane's hooks fire around the same
// patch and their relative order isn't something to depend on. Reading here is
// unambiguously "before anything changes"; @before-enter below is unambiguously
// "right before the entering pane is inserted", so capture and apply can never
// race each other.
watch(() => props.direction, (dir) => {
  if (!scrollEl) return
  if (dir === 'forward') scrollStack.push(scrollEl.scrollTop)
  // 'back' has nothing to capture: a detail pane's own scroll offset is never
  // remembered, so reopening it later always starts at the top too.
}, { flush: 'pre' })

function onBeforeEnter() {
  if (!scrollEl) return
  scrollEl.scrollTop = props.direction === 'back' ? (scrollStack.pop() ?? 0) : 0
}
</script>
