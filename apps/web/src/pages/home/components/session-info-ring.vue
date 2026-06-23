<template>
  <Popover v-model:open="open">
    <PopoverTrigger
      as="button"
      type="button"
      :class="[
        'inline-flex items-center justify-center size-9 rounded-full text-foreground transition-[opacity,scale] duration-200 ease-out disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none',
        ($attrs.class as string | undefined) ?? '',
      ]"
      :disabled="!sessionId"
      :aria-label="t('chat.sessionInfoRingAria')"
      @mouseenter="handleTriggerMouseEnter"
      @mouseleave="handleMouseLeave"
      @blur="handleMouseLeave"
      @click="handleTriggerClick"
    >
      <svg
        viewBox="0 0 24 24"
        class="size-6 -rotate-90"
        aria-hidden="true"
      >
        <circle
          cx="12"
          cy="12"
          :r="radius"
          fill="none"
          stroke="currentColor"
          :stroke-width="strokeWidth"
          class="opacity-20"
        />
        <circle
          cx="12"
          cy="12"
          :r="radius"
          fill="none"
          :class="ringColorClass"
          stroke="currentColor"
          stroke-linecap="round"
          :stroke-width="strokeWidth"
          :stroke-dasharray="circumference"
          :stroke-dashoffset="dashOffset"
          class="transition-all"
        />
      </svg>
    </PopoverTrigger>
    <PopoverContent
      class="w-80 p-0 max-h-[60vh] overflow-hidden"
      align="end"
      side="top"
      :side-offset="8"
      @mouseenter="handleContentMouseEnter"
      @mouseleave="handleMouseLeave"
      @focusin="handleContentFocusIn"
      @focusout="handleContentFocusOut"
      @open-auto-focus="handleOpenAutoFocus"
    >
      <SessionInfoPanel
        :visible="open"
        :override-model-id="overrideModelId"
        :fallback-context-window="fallbackContextWindow"
      />
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Popover, PopoverContent, PopoverTrigger } from '@memohai/ui'
import SessionInfoPanel from './session-info-panel.vue'
import { useSessionInfo } from '../composables/useSessionInfo'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  overrideModelId?: string
  fallbackContextWindow?: number | null
}>()

const { t } = useI18n()
const open = ref(false)
const focusContentOnOpen = ref(false)

const overrideModelIdRef = computed(() => props.overrideModelId ?? '')
const fallbackContextWindowRef = computed(() => props.fallbackContextWindow ?? null)
const { contextPercent, sessionId } = useSessionInfo({
  overrideModelId: overrideModelIdRef,
  fallbackContextWindow: fallbackContextWindowRef,
})

const radius = 10
const strokeWidth = 2.5
const circumference = computed(() => 2 * Math.PI * radius)
const dashOffset = computed(() => {
  const pct = Math.max(0, Math.min(100, contextPercent.value))
  return circumference.value * (1 - pct / 100)
})

const ringColorClass = computed(() => {
  if (contextPercent.value >= 90) return 'text-destructive'
  if (contextPercent.value >= 70) return 'text-warning'
  return 'text-foreground'
})

let openTimer: ReturnType<typeof setTimeout> | null = null
let closeTimer: ReturnType<typeof setTimeout> | null = null

function clearTimers() {
  if (openTimer) {
    clearTimeout(openTimer)
    openTimer = null
  }
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
}

function scheduleOpen() {
  if (!sessionId.value) return
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
  if (open.value) return
  openTimer = setTimeout(() => {
    open.value = true
    openTimer = null
  }, 150)
}

function handleTriggerMouseEnter() {
  focusContentOnOpen.value = false
  scheduleOpen()
}

function handleTriggerClick() {
  focusContentOnOpen.value = true
  clearTimers()
  open.value = true
}

function handleContentMouseEnter() {
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
}

function handleContentFocusIn() {
  handleContentMouseEnter()
}

function handleContentFocusOut(event: FocusEvent) {
  const current = event.currentTarget as HTMLElement | null
  const next = event.relatedTarget as Node | null
  if (current && next && current.contains(next)) return
  handleMouseLeave()
}

function handleOpenAutoFocus(event: Event) {
  if (focusContentOnOpen.value) {
    focusContentOnOpen.value = false
    return
  }
  event.preventDefault()
}

function handleMouseLeave() {
  if (openTimer) {
    clearTimeout(openTimer)
    openTimer = null
  }
  closeTimer = setTimeout(() => {
    open.value = false
    closeTimer = null
  }, 200)
}

defineExpose({ clearTimers })
</script>
