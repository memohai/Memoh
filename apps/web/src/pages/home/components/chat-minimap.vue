<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, shallowRef, useId, useTemplateRef, watch } from 'vue'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { ChatMessage } from '@/store/chat-list'
import { activeAnchorIndex, buildMinimapAnchors, panelScrollTop, tickWidth, viewportIndicator } from './chat-minimap'

const props = defineProps<{
  scrollEl: HTMLElement | null
  contentEl: HTMLElement | null
  messages: ChatMessage[]
}>()

const emit = defineEmits<{
  navigate: [messageId: string]
}>()

const { t } = useI18n()
const uid = useId()
const listId = `chat-minimap-list-${uid}`

const MIN_ANCHORS = 4
const ROW_HEIGHT = 28
const LIST_PADDING = 4

const anchors = computed(() => buildMinimapAnchors(props.messages))
const visible = computed(() => anchors.value.length >= MIN_ANCHORS)
const anchorsKey = computed(() => anchors.value.map(anchor => anchor.id).join('|'))

const geometry = shallowRef<{ tops: Map<string, number>, scrollHeight: number } | null>(null)
const activeIndex = shallowRef(0)
const highlightedIndex = shallowRef(-1)
const open = shallowRef(false)

const bandEl = useTemplateRef<HTMLElement>('band')
const listEl = useTemplateRef<HTMLElement>('list')

const anchorTops = computed(() => {
  const tops = geometry.value?.tops
  return anchors.value.map(anchor => tops?.get(anchor.id) ?? Number.MAX_SAFE_INTEGER)
})

const ticks = computed(() => {
  const total = geometry.value?.scrollHeight ?? 0
  if (!total) return []
  const tops = anchorTops.value
  return anchors.value.map((anchor, index) => ({
    id: anchor.id,
    topPercent: Math.min(100, Math.max(0, (tops[index]! / total) * 100)),
    width: tickWidth(anchor.preview.length),
  }))
})

const barStyle = computed(() => ({
  transform: `translateY(${LIST_PADDING + activeIndex.value * ROW_HEIGHT + (ROW_HEIGHT - 20) / 2}px)`,
}))

function rowId(index: number) {
  return `${listId}-row-${index}`
}

function rebuild() {
  const root = props.scrollEl
  if (!root || !visible.value) return
  const wanted = new Set(anchors.value.map(anchor => anchor.id))
  const rootRect = root.getBoundingClientRect()
  const tops = new Map<string, number>()
  for (const el of root.querySelectorAll<HTMLElement>('[data-message-id]')) {
    const id = el.dataset.messageId
    if (id && wanted.has(id)) {
      tops.set(id, root.scrollTop + el.getBoundingClientRect().top - rootRect.top)
    }
  }
  geometry.value = { tops, scrollHeight: root.scrollHeight }
  syncFromScroll()
}

let rebuildTimer: number | null = null
function scheduleRebuild() {
  if (rebuildTimer !== null) return
  rebuildTimer = window.setTimeout(() => {
    rebuildTimer = null
    rebuild()
  }, 200)
}

watch([() => props.scrollEl, anchorsKey, visible], () => rebuild(), { flush: 'post' })
useResizeObserver(computed(() => props.contentEl), scheduleRebuild)

let suppressTimer: number | null = null
function releaseSuppression() {
  if (suppressTimer === null) return
  clearTimeout(suppressTimer)
  suppressTimer = null
  syncFromScroll()
}

function suppressUntilScrollEnd() {
  if (suppressTimer !== null) clearTimeout(suppressTimer)
  suppressTimer = window.setTimeout(releaseSuppression, 900)
}

function syncFromScroll() {
  const root = props.scrollEl
  if (!root || !visible.value) return
  const view = { scrollTop: root.scrollTop, clientHeight: root.clientHeight, scrollHeight: root.scrollHeight }
  const indicator = viewportIndicator(view)
  const band = bandEl.value
  if (band) {
    band.style.top = `${indicator.topPercent}%`
    band.style.height = `${indicator.heightPercent}%`
  }
  if (suppressTimer !== null) return
  const index = activeAnchorIndex(anchorTops.value, view)
  if (index >= 0) activeIndex.value = index
}

let scrollRaf = 0
useEventListener(() => props.scrollEl, 'scroll', () => {
  if (scrollRaf) return
  scrollRaf = requestAnimationFrame(() => {
    scrollRaf = 0
    syncFromScroll()
  })
}, { passive: true })
useEventListener(() => props.scrollEl, 'scrollend', releaseSuppression, { passive: true })

let openTimer: number | null = null
let closeTimer: number | null = null

function clearOpenTimers() {
  if (openTimer !== null) {
    clearTimeout(openTimer)
    openTimer = null
  }
  if (closeTimer !== null) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
}

function openPanel() {
  clearOpenTimers()
  if (open.value) return
  open.value = true
  highlightedIndex.value = activeIndex.value
  void nextTick(() => positionList(true))
}

function closePanel() {
  clearOpenTimers()
  open.value = false
  highlightedIndex.value = -1
}

function scheduleOpen() {
  if (closeTimer !== null) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
  if (open.value || openTimer !== null) return
  openTimer = window.setTimeout(openPanel, 80)
}

function scheduleClose() {
  if (openTimer !== null) {
    clearTimeout(openTimer)
    openTimer = null
  }
  if (!open.value || closeTimer !== null) return
  closeTimer = window.setTimeout(closePanel, 150)
}

function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && (event.currentTarget as Node).contains(next)) return
  closePanel()
}

function positionList(instant = false) {
  const list = listEl.value
  if (!list) return
  const index = highlightedIndex.value >= 0 ? highlightedIndex.value : activeIndex.value
  const target = panelScrollTop({
    itemTop: LIST_PADDING + index * ROW_HEIGHT,
    itemHeight: ROW_HEIGHT,
    viewTop: list.scrollTop,
    viewHeight: list.clientHeight,
  })
  if (target !== null) list.scrollTo({ top: target, behavior: instant ? 'instant' : 'smooth' })
}

watch(activeIndex, () => {
  if (!open.value) return
  highlightedIndex.value = activeIndex.value
  positionList()
})

function navigate(index: number) {
  const anchor = anchors.value[index]
  if (!anchor) return
  activeIndex.value = index
  highlightedIndex.value = index
  suppressUntilScrollEnd()
  emit('navigate', anchor.id)
}

function moveHighlight(delta: number) {
  const count = anchors.value.length
  if (!count) return
  const current = highlightedIndex.value >= 0 ? highlightedIndex.value : activeIndex.value
  highlightedIndex.value = Math.min(count - 1, Math.max(0, current + delta))
  positionList()
}

function onKeydown(event: KeyboardEvent) {
  if (!open.value) {
    if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
      event.preventDefault()
      openPanel()
    }
    return
  }
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      moveHighlight(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      moveHighlight(-1)
      break
    case 'Home':
      event.preventDefault()
      highlightedIndex.value = 0
      positionList()
      break
    case 'End':
      event.preventDefault()
      highlightedIndex.value = anchors.value.length - 1
      positionList()
      break
    case 'Enter':
    case ' ':
      event.preventDefault()
      if (highlightedIndex.value >= 0) navigate(highlightedIndex.value)
      break
    case 'Escape':
      event.preventDefault()
      closePanel()
      break
  }
}

onBeforeUnmount(() => {
  clearOpenTimers()
  if (rebuildTimer !== null) clearTimeout(rebuildTimer)
  if (suppressTimer !== null) clearTimeout(suppressTimer)
  if (scrollRaf) cancelAnimationFrame(scrollRaf)
})
</script>

<template>
  <div
    v-if="visible"
    class="group/minimap hidden md:flex absolute inset-y-0 right-2 z-10 w-72 flex-col items-end justify-center pointer-events-none"
    @mouseenter="scheduleOpen"
    @mouseleave="scheduleClose"
    @focusin="openPanel"
    @focusout="onFocusOut"
  >
    <button
      type="button"
      class="relative w-5 h-[clamp(120px,42vh,320px)] pointer-events-auto cursor-pointer outline-none transition-opacity duration-150 focus-visible:ring-1 focus-visible:ring-ring rounded-sm"
      :class="open ? 'opacity-0' : 'opacity-100'"
      :aria-label="t('chat.minimapLabel')"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="listId"
      :aria-activedescendant="open && highlightedIndex >= 0 ? rowId(highlightedIndex) : undefined"
      @keydown="onKeydown"
      @click="openPanel"
    >
      <span
        ref="band"
        aria-hidden="true"
        class="absolute right-0 w-full rounded-full bg-foreground/[0.07]"
      />
      <span
        v-for="(tick, index) in ticks"
        :key="tick.id"
        aria-hidden="true"
        class="absolute right-0 h-0.5 rounded-full transition-colors duration-150"
        :class="index === activeIndex ? 'bg-primary' : 'bg-muted-foreground/40 group-hover/minimap:bg-muted-foreground/60'"
        :style="{ top: `${tick.topPercent}%`, width: `${tick.width}px` }"
      />
    </button>

    <Transition
      enter-active-class="motion-safe:transition-[opacity,transform] motion-safe:duration-150 motion-safe:ease-out"
      enter-from-class="motion-safe:opacity-0 motion-safe:translate-x-1"
      enter-to-class="opacity-100 translate-x-0"
      leave-active-class="motion-safe:transition-[opacity,transform] motion-safe:duration-100 motion-safe:ease-in"
      leave-from-class="opacity-100 translate-x-0"
      leave-to-class="motion-safe:opacity-0 motion-safe:translate-x-1"
    >
      <div
        v-if="open"
        class="absolute right-0 top-1/2 w-72 -translate-y-1/2 overflow-hidden rounded-lg border bg-background/95 shadow-md backdrop-blur pointer-events-auto"
      >
        <div
          :id="listId"
          ref="list"
          role="listbox"
          :aria-label="t('chat.minimapLabel')"
          class="relative max-h-[min(60vh,420px)] overflow-y-auto overscroll-contain scrollbar-none px-1 py-1 [mask-image:linear-gradient(to_bottom,transparent,black_12px,black_calc(100%-12px),transparent)]"
        >
          <span
            aria-hidden="true"
            class="absolute left-1.5 h-5 w-0.5 rounded-full bg-primary motion-safe:transition-transform motion-safe:duration-300"
            :style="barStyle"
          />
          <button
            v-for="(anchor, index) in anchors"
            :id="rowId(index)"
            :key="anchor.id"
            type="button"
            role="option"
            tabindex="-1"
            :aria-selected="index === activeIndex"
            class="flex h-7 w-full items-center rounded-sm pl-4 pr-3 text-left text-xs"
            :class="index === highlightedIndex || index === activeIndex ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'"
            @click="navigate(index)"
            @mousemove="highlightedIndex = index"
          >
            <span class="truncate">{{ anchor.preview }}</span>
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>
