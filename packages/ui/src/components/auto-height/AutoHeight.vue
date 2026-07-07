<script setup lang="ts">
// AutoHeight — animates its OWN height to fit whatever it currently wraps, so a
// content swap (a dialog's list <-> form view, a conditional section revealing)
// grows/shrinks smoothly instead of hard-cutting. It measures the natural height
// of an inner content div via ResizeObserver and drives an explicit px height on
// the clipped outer; the inner is never height-constrained, so there is no
// measure/apply feedback loop.
//
// WHY a JS primitive and not pure CSS `height: auto` + `interpolate-size`: that
// keyword ships in Chromium (Electron) but not yet Safari, and apps/web targets
// general browsers — so the portable path is measure-and-set. WHY overflow is
// hidden: the two settled states both fit their content exactly (nothing clips at
// rest); clipping only happens mid-animation, which is the accordion-style reveal
// we want. Popovers/selects inside the slot portal to <body>, so they are never
// clipped by this.
//
// First paint does NOT animate (transitions are enabled one frame after the
// initial height is committed): a freshly-mounted surface reveals as a plain cut,
// matching SwapTransition's "no appear". Duration/easing are fixed here (not a
// prop) so the value stays documented in packages/ui/AGENTS.md and every caller
// animates identically; reduced-motion drops the transition entirely.
import { onBeforeUnmount, onMounted, ref } from 'vue'

const inner = ref<HTMLElement | null>(null)
const height = ref<string>('auto')
const ready = ref(false)
let observer: ResizeObserver | null = null

function measure() {
  const content = inner.value
  if (!content) return
  height.value = `${content.offsetHeight}px`
}

onMounted(() => {
  measure()
  // Commit the initial height first, THEN turn transitions on next frame — so the
  // mount itself is not animated from 0.
  requestAnimationFrame(() => {
    ready.value = true
  })
  observer = new ResizeObserver(measure)
  if (inner.value) observer.observe(inner.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<template>
  <div
    data-slot="auto-height"
    class="overflow-hidden"
    :class="{ 'auto-height-animated': ready }"
    :style="{ height }"
  >
    <div ref="inner">
      <slot />
    </div>
  </div>
</template>

<style scoped>
/* 220ms sits between the accordion reveal (180ms) and the segmented thumb (250ms)
   — a larger surface reads better a touch slower, still inside the motion palette
   documented in packages/ui/AGENTS.md. The curve is the house spring
   cubic-bezier(0.32, 0.72, 0, 1) (segmented thumb / width motions), NOT a flat
   ease-out: a big surface resizing on a straight-ish curve reads mechanical;
   the fast-out-soft-landing spring makes the size change feel settled. */
.auto-height-animated {
  transition: height 220ms cubic-bezier(0.32, 0.72, 0, 1);
}
@media (prefers-reduced-motion: reduce) {
  .auto-height-animated {
    transition: none;
  }
}
</style>
