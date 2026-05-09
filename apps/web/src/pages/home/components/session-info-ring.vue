<template>
  <Popover v-model:open="open">
    <PopoverTrigger
      as="button"
      type="button"
      :class="[
        'inline-flex items-center justify-center size-7 rounded-full text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        ($attrs.class as string | undefined) ?? '',
      ]"
      :disabled="!sessionId"
      :aria-label="t('chat.sessionInfoRingAria')"
      @mouseenter="handleMouseEnter"
      @mouseleave="handleMouseLeave"
      @focus="handleMouseEnter"
      @blur="handleMouseLeave"
    >
      <svg
        viewBox="0 0 24 24"
        class="size-5 -rotate-90"
        aria-hidden="true"
      >
        <circle
          cx="12"
          cy="12"
          :r="radius"
          fill="none"
          stroke="currentColor"
          :stroke-width="strokeWidth"
          class="opacity-25"
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
      @open-auto-focus="(e) => e.preventDefault()"
    >
      <SessionInfoPanel
        :visible="open"
        :override-model-id="overrideModelId"
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
}>()

const { t } = useI18n()
const open = ref(false)

const overrideModelIdRef = computed(() => props.overrideModelId ?? '')
const { contextPercent, sessionId } = useSessionInfo({
  overrideModelId: overrideModelIdRef,
})

const radius = 9
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

function handleMouseEnter() {
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

function handleContentMouseEnter() {
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
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
