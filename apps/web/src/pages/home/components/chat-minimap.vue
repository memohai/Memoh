<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, shallowRef, useId, useTemplateRef, watch } from 'vue'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { ChatMessage } from '@/store/chat-list'
import { activeAnchorIndex, buildMinimapAnchors, panelScrollTop, railActivePosition, sampleRailIndexes, tickWidth } from './chat-minimap'

const props = defineProps<{
  scrollEl: HTMLElement | null
  contentEl: HTMLElement | null
  messages: ChatMessage[]
  hasMoreOlder?: boolean
}>()

const emit = defineEmits<{
  navigate: [messageId: string]
}>()

const { t } = useI18n()
const uid = useId()
const listId = `chat-minimap-list-${uid}`

const MIN_ANCHORS = 4
const MAX_RAIL_MARKS = 28
const ROW_HEIGHT = 32
const LIST_PADDING = 6
const HINT_HEIGHT = 24

const anchors = computed(() => buildMinimapAnchors(props.messages))
const visible = computed(() => anchors.value.length >= MIN_ANCHORS)
const anchorsKey = computed(() => anchors.value.map(anchor => anchor.id).join('|'))

const railIndexes = computed(() => sampleRailIndexes(anchors.value.length, MAX_RAIL_MARKS))
const railMarks = computed(() => railIndexes.value.map((anchorIndex) => {
  const anchor = anchors.value[anchorIndex]!
  return { id: anchor.id, width: tickWidth(anchor.preview.length) }
}))

const geometry = shallowRef<{ tops: Map<string, number> } | null>(null)
const activeIndex = shallowRef(0)
const highlightedIndex = shallowRef(-1)
const open = shallowRef(false)

const railActive = computed(() => railActivePosition(railIndexes.value, activeIndex.value))

const railEl = useTemplateRef<HTMLButtonElement>('rail')
const listEl = useTemplateRef<HTMLElement>('list')

const anchorTops = computed(() => {
  const tops = geometry.value?.tops
  return anchors.value.map(anchor => tops?.get(anchor.id) ?? Number.MAX_SAFE_INTEGER)
})

const hintOffset = computed(() => props.hasMoreOlder ? HINT_HEIGHT : 0)

const barStyle = computed(() => ({
  transform: `translateY(${LIST_PADDING + hintOffset.value + activeIndex.value * ROW_HEIGHT + (ROW_HEIGHT - 20) / 2}px)`,
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
  geometry.value = { tops }
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
  if (!root || !visible.value || suppressTimer !== null) return
  const view = { scrollTop: root.scrollTop, clientHeight: root.clientHeight, scrollHeight: root.scrollHeight }
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

function openPanel(focusList = false) {
  clearOpenTimers()
  if (!open.value) {
    open.value = true
    highlightedIndex.value = activeIndex.value
  }
  void nextTick(() => {
    positionList(true)
    if (focusList) listEl.value?.focus({ preventScroll: true })
  })
}

let skipFocusOpen = false

function closePanel(restoreFocus = false) {
  clearOpenTimers()
  open.value = false
  highlightedIndex.value = -1
  if (restoreFocus) {
    skipFocusOpen = true
    railEl.value?.focus({ preventScroll: true })
  }
}

function onFocusIn() {
  if (skipFocusOpen) {
    skipFocusOpen = false
    return
  }
  openPanel()
}

function scheduleOpen() {
  if (closeTimer !== null) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
  if (open.value || openTimer !== null) return
  openTimer = window.setTimeout(() => openPanel(), 80)
}

function scheduleClose() {
  if (openTimer !== null) {
    clearTimeout(openTimer)
    openTimer = null
  }
  if (!open.value || closeTimer !== null) return
  closeTimer = window.setTimeout(() => closePanel(), 150)
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
    itemTop: LIST_PADDING + hintOffset.value + index * ROW_HEIGHT,
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

function onRailKeydown(event: KeyboardEvent) {
  if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
    event.preventDefault()
    openPanel(true)
  }
}

function onListKeydown(event: KeyboardEvent) {
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
      closePanel(true)
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
    @focusin="onFocusIn"
    @focusout="onFocusOut"
  >
    <button
      ref="rail"
      type="button"
      class="flex max-h-[60vh] w-5 flex-col items-end justify-center gap-2 overflow-hidden px-0.5 py-2 pointer-events-auto cursor-pointer outline-none rounded-sm transition-opacity duration-150 focus-visible:ring-1 focus-visible:ring-ring"
      :class="open ? 'opacity-0' : 'opacity-100'"
      :aria-label="t('chat.minimapLabel')"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="listId"
      @keydown="onRailKeydown"
      @click="openPanel(true)"
    >
      <span
        v-for="(mark, position) in railMarks"
        :key="mark.id"
        aria-hidden="true"
        class="h-0.5 shrink-0 rounded-full transition-colors duration-150"
        :class="position === railActive ? 'bg-primary' : 'bg-muted-foreground/35 group-hover/minimap:bg-muted-foreground/60'"
        :style="{ width: `${mark.width}px` }"
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
        class="absolute right-0 top-1/2 w-72 -translate-y-1/2 overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg pointer-events-auto"
      >
        <div
          :id="listId"
          ref="list"
          role="listbox"
          tabindex="-1"
          :aria-label="t('chat.minimapLabel')"
          :aria-activedescendant="highlightedIndex >= 0 ? rowId(highlightedIndex) : undefined"
          class="relative max-h-[min(60vh,420px)] overflow-y-auto overscroll-contain scrollbar-none p-1.5 outline-none [mask-image:linear-gradient(to_bottom,transparent,black_12px,black_calc(100%-12px),transparent)]"
          @keydown="onListKeydown"
        >
          <div
            v-if="hasMoreOlder"
            class="flex h-6 items-center justify-center text-[11px] text-muted-foreground/70 select-none"
          >
            {{ t('chat.minimapEarlier') }}
          </div>
          <span
            aria-hidden="true"
            class="absolute left-2 h-5 w-0.5 rounded-full bg-primary motion-safe:transition-transform motion-safe:duration-200 motion-safe:ease-out"
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
            class="flex h-8 w-full items-center rounded-md pl-3.5 pr-2.5 text-left text-xs transition-colors duration-100"
            :class="index === highlightedIndex
              ? 'bg-accent text-accent-foreground'
              : index === activeIndex ? 'text-foreground' : 'text-muted-foreground hover:text-foreground'"
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
