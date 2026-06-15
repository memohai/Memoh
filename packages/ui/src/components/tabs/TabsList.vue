<script setup lang="ts">
import type { TabsListProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TabsList } from 'reka-ui'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<TabsListProps & {
  class?: HTMLAttributes['class']
  // underline = section nav rail (default); pill = the enclosed SegmentedControl
  // chrome ported verbatim. Both keep full Tabs semantics + panels — only the
  // skin + indicator differ.
  variant?: 'underline' | 'pill'
}>()

const variant = computed(() => props.variant ?? 'underline')
const delegatedProps = reactiveOmit(props, 'class', 'variant')

// The indicator is MEASURED off the active trigger (same technique as
// SegmentedControl) so the slide can never mismatch a trigger's footprint.
// Geometry is taken via getBoundingClientRect deltas to stay robust against the
// reka list nesting / offsetParent quirks.
const root = ref<HTMLElement>()
const box = ref({ left: 0, top: 0, width: 0, height: 0, ready: false })
// The indicator is placed instantly on first paint (no slide-in from the left
// edge); transitions turn on only after the initial measure, so subsequent tab
// changes still animate.
const animate = ref(false)
// press-shrink fires ONLY when the already-active pill is pressed (inactive
// items shrink via their own ::before:active, matching SegmentedControl).
const pressed = ref(false)

function sync() {
  const rootEl = root.value
  if (!rootEl)
    return
  const active = rootEl.querySelector<HTMLElement>('[data-slot=tabs-trigger][data-state=active]')
  if (!active) {
    box.value = { ...box.value, ready: false }
    return
  }
  const r = rootEl.getBoundingClientRect()
  const a = active.getBoundingClientRect()
  box.value = {
    left: a.left - r.left,
    top: a.top - r.top,
    width: a.width,
    height: a.height,
    ready: a.width > 0,
  }
}

let ro: ResizeObserver | undefined
let mo: MutationObserver | undefined
onMounted(() => {
  nextTick(() => {
    sync()
    // Enable transitions one frame after the first placement so the bar lands
    // in position instead of sliding/growing from the left on mount.
    requestAnimationFrame(() => { animate.value = true })
  })
  ro = new ResizeObserver(() => sync())
  if (root.value)
    ro.observe(root.value)
  // reka toggles data-state on triggers when the value changes — re-measure then.
  mo = new MutationObserver(() => nextTick(sync))
  if (root.value)
    mo.observe(root.value, { attributes: true, subtree: true, attributeFilter: ['data-state'] })
})
onBeforeUnmount(() => {
  ro?.disconnect()
  mo?.disconnect()
})

function onDown(e: PointerEvent) {
  const trigger = (e.target as HTMLElement | null)?.closest('[data-slot=tabs-trigger]')
  pressed.value = trigger?.getAttribute('data-state') === 'active'
}
function clearPress() {
  pressed.value = false
}

const rootClass = computed(() => variant.value === 'pill'
  ? 'relative inline-flex w-fit items-center rounded-lg bg-[color:var(--segment-track)] p-0.5'
  : 'relative inline-flex w-full items-center border-b border-border')

const listClass = computed(() => variant.value === 'pill'
  ? 'inline-flex items-center gap-0.5'
  : 'inline-flex w-full items-center justify-start gap-4')

const indicatorStyle = computed(() => {
  const b = box.value
  if (variant.value === 'pill') {
    return {
      translate: `${b.left}px ${b.top}px`,
      width: `${b.width}px`,
      height: `${b.height}px`,
      scale: pressed.value ? '0.97' : '1',
      opacity: b.ready ? '1' : '0',
    }
  }
  return {
    translate: `${b.left}px`,
    width: `${b.width}px`,
    opacity: b.ready ? '1' : '0',
  }
})
</script>

<template>
  <div
    ref="root"
    :data-variant="variant"
    :class="cn(rootClass, props.class)"
    @pointerdown="onDown"
    @pointerup="clearPress"
    @pointerleave="clearPress"
    @pointercancel="clearPress"
  >
    <span
      aria-hidden="true"
      :class="[variant === 'pill' ? 'tabs-thumb' : 'tabs-underline', { 'tabs-indicator-static': !animate }]"
      :style="indicatorStyle"
    />
    <TabsList
      data-slot="tabs-list"
      v-bind="delegatedProps"
      :class="listClass"
    >
      <slot />
    </TabsList>
  </div>
</template>

<style scoped>
/* First-paint guard: until the active item is measured, the indicator must land
 * in place rather than animate from the {left:0,width:0} default. */
.tabs-indicator-static {
  transition: none !important;
}

/* ── underline: a sliding bar pinned to the bottom rail ───────────────────── */
.tabs-underline {
  position: absolute;
  bottom: -1px;
  left: 0;
  height: 2px;
  border-radius: 9999px;
  background-color: var(--foreground);
  pointer-events: none;
  transition:
    translate 0.2s cubic-bezier(0.32, 0.72, 0, 1),
    width 0.2s cubic-bezier(0.32, 0.72, 0, 1),
    opacity 0.12s ease;
}

/* ── pill: the SegmentedControl sliding thumb, ported verbatim ─────────────── */
.tabs-thumb {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 0;
  border-radius: calc(var(--radius) - 0.125rem);
  background-color: var(--segment-thumb);
  box-shadow: var(--shadow-thumb);
  pointer-events: none;
  transition:
    translate 0.25s cubic-bezier(0.32, 0.72, 0, 1),
    width 0.25s cubic-bezier(0.32, 0.72, 0, 1),
    scale 0.16s ease,
    opacity 0.12s ease;
}

/* ── pill triggers: full parity with [data-segment-item] in style.css ─────── */
[data-variant="pill"] :deep([data-slot="tabs-trigger"]) {
  position: relative;
  z-index: 1;
  height: 1.75rem;
  padding-inline: 0.75rem;
  border-radius: calc(var(--radius) - 0.125rem);
  font-size: 0.875rem;
  font-weight: 475;
  letter-spacing: -0.16px;
  color: var(--control-label);
  transition: color 150ms ease;
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"]:hover) {
  color: var(--control-label-hover);
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"][data-state="active"]),
[data-variant="pill"] :deep([data-slot="tabs-trigger"][data-state="active"]:hover) {
  color: var(--foreground);
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"])::before {
  content: "";
  position: absolute;
  inset: 0;
  z-index: -1;
  border-radius: inherit;
  background-color: transparent;
  transition:
    background-color 150ms ease,
    scale 0.2s cubic-bezier(0.32, 0.72, 0, 1);
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"]:not([data-state="active"]):hover)::before {
  background-color: var(--segment-overlay-hover);
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"]:not([data-state="active"]):active)::before {
  scale: 0.965;
  background-color: var(--segment-overlay-active);
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"]:disabled) {
  pointer-events: none;
  opacity: 0.5;
}
[data-variant="pill"] :deep([data-slot="tabs-trigger"]:focus-visible) {
  outline: none;
  box-shadow:
    0 0 0 2px var(--segment-track),
    0 0 0 4px var(--ring);
}
</style>
