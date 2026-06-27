<template>
  <div
    ref="rootEl"
    class="group/tab relative z-[1] flex h-full min-w-0 items-center overflow-visible pr-[1.6875rem] pb-[3.5px] pl-2"
    @auxclick.middle.prevent="close"
  >
    <svg
      v-if="isActive"
      class="active-tab-shape z-0"
      :viewBox="activeTabViewBox"
      :style="activeTabShapeStyle"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <path
        :d="activeTabFillPath"
        fill="var(--surface-editor)"
      />
      <path
        :d="activeTabStrokePath"
        fill="none"
        stroke="var(--dock-stroke)"
        stroke-width="1"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <!-- Active state is signalled by text colour, fill, and the connected chip
         shape, not by weight or size. Every tab is the same height. The tab's
         reserved top border plus this bottom padding parks label and close on
         the same optical center for active and inactive states. -->
    <span
      class="relative z-[1] min-w-0 flex-1 truncate text-label leading-[1.3] tracking-normal transition-colors"
      :class="[
        isActive ? 'text-foreground' : 'text-muted-foreground',
      ]"
    >{{ title }}</span>
    <!-- Unsaved-changes dot: sits in the close slot at rest so the affordance never
         shifts; hovering fades it out as the close button fades in.
         Painted over the same fade as the button so a long title dissolves behind
         it instead of colliding with the glyph. -->
    <div
      v-if="isDirty"
      class="close-fade pointer-events-none absolute right-[0.1875rem] z-[2] flex items-center pl-6 pr-[0.1875rem] opacity-100 group-hover/tab:opacity-0"
    >
      <span class="flex size-5 items-center justify-center">
        <span
          class="size-[7px] rounded-full"
          :class="isActive ? 'bg-foreground' : 'bg-muted-foreground'"
        />
      </span>
    </div>
    <!-- Close affordance: hover-only, absolutely positioned so it never reserves a
         slot or resizes the chip (geometry is identical hovered or not). It paints
         the chip's own OPAQUE hover colour (--tab-hover-bg) as a left→right fade, so
         the title dissolves into the chip and nothing stays legible under the
         button. The fade layer is click-through; only the button takes pointer
         events. Keyboard focus reveals it for a11y; middle-click closes without it. -->
    <div
      class="close-fade pointer-events-none absolute right-[0.1875rem] z-[2] flex items-center pl-6 pr-[0.1875rem] opacity-0 group-hover/tab:opacity-100 focus-within:opacity-100"
    >
      <!-- No own hover fill: the close affordance is read through the left→right
           fade (which already paints the chip's hover surface) plus the icon
           darkening on hover. A second darker square behind the glyph just
           double-stacks chrome, so the ghost hover background is suppressed. -->
      <Button
        variant="ghost"
        class="pointer-events-auto size-5 shrink-0 translate-y-[-0.5px] rounded-sm p-0 text-muted-foreground [--btn-ghost-hover:transparent] hover:text-foreground"
        :aria-label="t('chat.tabMenu.close')"
        @pointerdown.stop
        @mousedown.stop
        @click.stop.prevent="close"
      >
        <X class="size-3.5" />
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

// Custom default tab: replaces dockview's built-in tab (square icon-hover
// block) with a design-system label + a ghost close button on a fixed slot.
const props = defineProps<{
  params: {
    api: DockviewPanelApi
    containerApi: DockviewApi
    params: Record<string, unknown>
  }
}>()

const { t } = useI18n()
const workspaceTabs = useWorkspaceTabsStore()

const rootEl = ref<HTMLElement | null>(null)
const panelId = props.params.api.id
const title = ref(props.params.api.title ?? '')
const isActive = ref(props.params.api.isActive)
const initialTabShape = buildActiveTabShape({
  width: 200,
  height: 31,
  radius: 8,
  strokeWidth: 1,
})
const activeTabViewBox = ref(initialTabShape.viewBox)
const activeTabFillPath = ref(initialTabShape.fillPath)
const activeTabStrokePath = ref(initialTabShape.strokePath)
const activeTabShapeStyle = ref<Record<string, string>>({
  left: '-8px',
  top: '0px',
  width: '216px',
  height: '31px',
})
let resizeObserver: ResizeObserver | null = null
let pendingShapeFrame = 0
// Unsaved-changes flag for file panels — read from the store's reactive map, so
// the dot, the sidebar badge and the close dialog never drift apart.
const isDirty = computed(() => !!workspaceTabs.fileDirty[panelId])
// Ephemeral preview tabs still get replaced in place when another
// preview-eligible tab opens into the same group (see workspace-tabs store), but
// the state is no longer surfaced visually — there is no italic or other marker.

const disposables = [
  props.params.api.onDidTitleChange((event) => {
    title.value = event.title
  }),
  props.params.api.onDidActiveChange((event) => {
    isActive.value = event.isActive
    if (event.isActive) scheduleActiveTabShapeUpdate()
  }),
  props.params.containerApi.onDidLayoutChange(() => {
    scheduleActiveTabShapeUpdate()
  }),
]

// The tab part is initialized before the panel's title is applied (dockview
// sets it right after init), so re-read once the addPanel call stack settled.
onMounted(() => {
  title.value = props.params.api.title ?? title.value
  isActive.value = props.params.api.isActive
  nextTick(() => {
    installShapeObserver()
    window.addEventListener('resize', scheduleActiveTabShapeUpdate)
    scheduleActiveTabShapeUpdate()
  })
})

// Route through the store guard: a dirty file opens the save-confirm dialog
// instead of closing straight away; clean tabs close immediately.
function close() {
  workspaceTabs.requestCloseTab(panelId)
}

onBeforeUnmount(() => {
  if (pendingShapeFrame) cancelAnimationFrame(pendingShapeFrame)
  resizeObserver?.disconnect()
  window.removeEventListener('resize', scheduleActiveTabShapeUpdate)
  for (const d of disposables) d.dispose()
})

watch(isActive, (active) => {
  if (active) nextTick(scheduleActiveTabShapeUpdate)
})

function installShapeObserver() {
  const root = rootEl.value
  if (!root || resizeObserver) return

  resizeObserver = new ResizeObserver(() => scheduleActiveTabShapeUpdate())
  resizeObserver.observe(root)

  const tab = root.closest<HTMLElement>('.dv-tab')
  if (tab && tab !== root) resizeObserver.observe(tab)
}

function scheduleActiveTabShapeUpdate() {
  if (!isActive.value || pendingShapeFrame) return

  pendingShapeFrame = requestAnimationFrame(() => {
    pendingShapeFrame = 0
    updateActiveTabShape()
  })
}

function updateActiveTabShape() {
  const root = rootEl.value
  const tab = root?.closest<HTMLElement>('.dv-tab')
  if (!root || !tab) return

  const rootRect = root.getBoundingClientRect()
  const tabRect = tab.getBoundingClientRect()
  if (tabRect.width <= 0 || tabRect.height <= 0) return

  const scale = window.devicePixelRatio || 1
  const alignedLeft = snapToDevicePixel(tabRect.left, scale)
  const alignedTop = snapToDevicePixel(tabRect.top, scale)
  const alignedRight = snapToDevicePixel(tabRect.right, scale)
  const alignedBottom = snapToDevicePixel(tabRect.bottom, scale)
  const width = alignedRight - alignedLeft
  const height = alignedBottom - alignedTop
  const radius = Math.min(
    snapToDevicePixel(readTabRadius(tab), scale),
    Math.max(0, width / 2),
    Math.max(0, height),
  )

  const shape = buildActiveTabShape({
    width,
    height,
    radius,
    strokeWidth: 1,
  })

  activeTabViewBox.value = shape.viewBox
  activeTabFillPath.value = shape.fillPath
  activeTabStrokePath.value = shape.strokePath
  activeTabShapeStyle.value = {
    left: `${formatPx(alignedLeft - rootRect.left - radius)}`,
    top: `${formatPx(alignedTop - rootRect.top)}`,
    width: `${formatPx(width + radius * 2)}`,
    height: `${formatPx(height)}`,
  }
}

function buildActiveTabShape({
  width,
  height,
  radius,
  strokeWidth,
}: {
  width: number
  height: number
  radius: number
  strokeWidth: number
}) {
  const strokeAdjustment = strokeWidth * 0.5
  const extension = radius
  const left = -extension
  const right = width + extension
  const extendedBottom = height
  const tabLeft = strokeAdjustment
  const tabRight = width - strokeAdjustment
  const tabTop = strokeAdjustment
  const tabBottom = height - strokeAdjustment
  const topRadius = Math.max(0, radius - strokeAdjustment)
  const bottomRadius = Math.max(0, radius - strokeAdjustment)

  const outline = [
    `M ${fmt(left)} ${fmt(extendedBottom)}`,
    `L ${fmt(left)} ${fmt(tabBottom)}`,
    `L ${fmt(tabLeft - bottomRadius)} ${fmt(tabBottom)}`,
    `A ${fmt(bottomRadius)} ${fmt(bottomRadius)} 0 0 0 ${fmt(tabLeft)} ${fmt(tabBottom - bottomRadius)}`,
    `L ${fmt(tabLeft)} ${fmt(tabTop + topRadius)}`,
    `A ${fmt(topRadius)} ${fmt(topRadius)} 0 0 1 ${fmt(tabLeft + topRadius)} ${fmt(tabTop)}`,
    `L ${fmt(tabRight - topRadius)} ${fmt(tabTop)}`,
    `A ${fmt(topRadius)} ${fmt(topRadius)} 0 0 1 ${fmt(tabRight)} ${fmt(tabTop + topRadius)}`,
    `L ${fmt(tabRight)} ${fmt(tabBottom - bottomRadius)}`,
    `A ${fmt(bottomRadius)} ${fmt(bottomRadius)} 0 0 0 ${fmt(tabRight + bottomRadius)} ${fmt(tabBottom)}`,
    `L ${fmt(right)} ${fmt(tabBottom)}`,
    `L ${fmt(right)} ${fmt(extendedBottom)}`,
  ].join(' ')

  return {
    viewBox: `${fmt(left)} 0 ${fmt(width + extension * 2)} ${fmt(height)}`,
    fillPath: `${outline} L ${fmt(left)} ${fmt(extendedBottom)} Z`,
    strokePath: outline,
  }
}

function readTabRadius(tab: HTMLElement) {
  const radius = Number.parseFloat(getComputedStyle(tab).borderTopLeftRadius)
  return Number.isFinite(radius) && radius > 0 ? radius : 8
}

function snapToDevicePixel(value: number, scale: number) {
  return Math.round(value * scale) / scale
}

function formatPx(value: number) {
  return `${fmt(value)}px`
}

function fmt(value: number) {
  const normalized = Math.abs(value) < 0.0001 ? 0 : value
  return Number(normalized.toFixed(3)).toString()
}
</script>

<style scoped>
.active-tab-shape {
  position: absolute;
  overflow: visible;
  pointer-events: none;
}

/* The close affordance blots out the title with the chip's own opaque hover colour:
 * transparent on the left so the text dissolves into the chip, fully opaque by the
 * button so NOTHING is legible underneath. --tab-hover-bg is inherited from .dv-tab
 * (the editor surface for the active tab, the hover tint otherwise), so the fade is
 * seamless with whatever the chip is wearing. Absolutely positioned, so painting it
 * never reserves a slot or resizes the chip. */
/* Two stacked fades so the blot is ALWAYS opaque. --tab-hover-bg is the surface a
 * tab wears (opaque --surface-editor when active; the TRANSLUCENT hover overlay
 * otherwise), so painting it alone left the title legible under the close button on
 * a hovered tab. Layering the opaque --surface-chrome base UNDER that overlay
 * composites to exactly what the tab shows — chrome+overlay on hover, plain editor
 * when active (the opaque top layer hides the base) — but never see-through. */
.close-fade {
  /* Span the tab's content box (1px top via the border / 3.5px bottom) so the
   * close affordance centres on the label. top:0 — the root is already below
   * the 1px top border. */
  top: 0;
  bottom: 3.5px;
  /* Instant hide when hover ends — avoids white/grey close-fade lingering on a tab
   * you just switched away from. Fade-in only while the tab group is hovered. */
  transition: none;
  background:
    linear-gradient(to right, transparent, var(--tab-hover-bg, var(--surface-editor)) 1rem),
    linear-gradient(to right, transparent, var(--surface-chrome) 1rem);
  /* Both right corners follow the hover rectangle's radius so the fade does not
   * square off the fill. */
  border-radius:
    0
    var(--tab-hover-right-radius, var(--tab-hover-radius, 0.3125rem))
    var(--tab-hover-right-radius, var(--tab-hover-radius, 0.3125rem))
    0;
}

.group\/tab:hover .close-fade,
.group\/tab:focus-within .close-fade {
  transition: opacity var(--tab-hover-duration, 150ms) ease-out;
}
</style>
