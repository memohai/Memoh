<script setup lang="ts">
// DialogBody — the scrollable body row of a capped dialog
// (DialogContent class="max-h-[80dvh] grid-rows-[auto_minmax(0,1fr)]").
// Two jobs, both previously hand-written per page:
//
// 1. Scroll gutter: `-mr-3 pr-3` pushes the scrollbar out of the content
//    column toward the DialogContent edge so text keeps the full p-6 width.
// 2. Scroll-edge fade: content that continues past the visible box fades out
//    over the last ~16px instead of being hard-clipped by the row boundary.
//    The fade is STATEFUL, not decorative — each edge only fades while there
//    is actually more content in that direction (at the top the top fade is
//    off; at the bottom the bottom fade is off; content shorter than the box
//    shows no fade at all). Driven by two @property-registered <length> vars
//    so the fade itself eases in/out (a fade that pops on at scroll start
//    would be the same hard cut one level up).
//
// The mask necessarily covers the scrollbar's first/last few px too — at 16px
// on a hairline scrollbar this is imperceptible, accepted trade-off.
//
// This is a plain div, NOT the AutoHeight primitive: AutoHeight's root is
// overflow-hidden (it clips its height tween) so it can never be the
// scroller. Nest it: <DialogBody><AutoHeight>…</AutoHeight></DialogBody>.
import type { HTMLAttributes } from 'vue'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const el = ref<HTMLElement | null>(null)
const fadeTop = ref(false)
const fadeBottom = ref(false)
let observer: ResizeObserver | null = null

function update() {
  const node = el.value
  if (!node)
    return
  // 1px slack: fractional scroll positions (zoom, dvh rounding) never quite
  // reach scrollHeight - clientHeight exactly.
  const remaining = node.scrollHeight - node.clientHeight - node.scrollTop
  fadeTop.value = node.scrollTop > 1
  fadeBottom.value = remaining > 1
}

onMounted(() => {
  update()
  // Watch BOTH boxes: the scroller (dialog cap / viewport changes) and the
  // content (AutoHeight tween, rows added/removed). Observing only the
  // scroller misses scrollHeight changes — its own box doesn't resize when
  // content grows inside a capped row.
  observer = new ResizeObserver(update)
  if (el.value) {
    observer.observe(el.value)
    for (const child of el.value.children) observer.observe(child)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<template>
  <div
    ref="el"
    data-slot="dialog-body"
    class="dialog-body -mr-3 overflow-y-auto pr-3"
    :class="cn(props.class)"
    :style="{
      '--dialog-body-fade-top': fadeTop ? '16px' : '0px',
      '--dialog-body-fade-bottom': fadeBottom ? '16px' : '0px',
    }"
    @scroll.passive="update"
  >
    <slot />
  </div>
</template>

<style scoped>
/* Registered as <length> (not inherited) so the fade distance itself can
   transition — an unregistered custom prop would snap between values. */
@property --dialog-body-fade-top {
  syntax: '<length>';
  inherits: false;
  initial-value: 0px;
}

@property --dialog-body-fade-bottom {
  syntax: '<length>';
  inherits: false;
  initial-value: 0px;
}

.dialog-body {
  /* transparent/black here are ALPHA stops in a mask, not paint colors. */
  mask-image: linear-gradient(
    to bottom,
    transparent,
    black var(--dialog-body-fade-top),
    black calc(100% - var(--dialog-body-fade-bottom)),
    transparent
  );
  transition:
    --dialog-body-fade-top 200ms ease,
    --dialog-body-fade-bottom 200ms ease;
}

@media (prefers-reduced-motion: reduce) {
  .dialog-body {
    transition: none;
  }
}
</style>
