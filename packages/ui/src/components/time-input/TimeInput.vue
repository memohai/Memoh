<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { cn } from '#/lib/utils'
import type { HTMLAttributes } from 'vue'

const props = withDefaults(defineProps<{
  hour?: number
  minute?: number
  disabled?: boolean
  // Matches Input size ladder: sm = h-8 / default = h-9
  size?: 'sm' | 'default'
  class?: HTMLAttributes['class']
}>(), {
  hour: 0,
  minute: 0,
  disabled: false,
  size: 'default',
})

const emit = defineEmits<{
  'update:hour': [number]
  'update:minute': [number]
}>()

const hourRef = ref<HTMLInputElement>()
const minuteRef = ref<HTMLInputElement>()

// Local display strings — allow in-flight partial input (e.g. "1" before "13")
const hourStr = ref(pad2(props.hour, 23))
const minuteStr = ref(pad2(props.minute, 59))

function pad2(n: number, max: number): string {
  const clamped = Math.max(0, Math.min(max, Math.round(n)))
  return String(clamped).padStart(2, '0')
}

// Keep local display in sync when parent prop changes (e.g. mode reset)
watch(() => props.hour, (h) => { hourStr.value = pad2(h, 23) })
watch(() => props.minute, (m) => { minuteStr.value = pad2(m, 59) })

// --- Hour ---
function onHourInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value.replace(/\D/g, '').slice(0, 2)
  hourStr.value = raw
  if (raw.length === 2) {
    const n = Math.min(23, parseInt(raw, 10))
    emit('update:hour', n)
    hourStr.value = pad2(n, 23)
    minuteRef.value?.focus()
    minuteRef.value?.select()
  }
}

function onHourBlur() {
  const n = Math.min(23, Math.max(0, parseInt(hourStr.value || '0', 10) || 0))
  emit('update:hour', n)
  hourStr.value = pad2(n, 23)
}

function onHourKeydown(e: KeyboardEvent) {
  if (e.key === ':' || e.key === 'ArrowRight') {
    e.preventDefault()
    minuteRef.value?.focus()
    minuteRef.value?.select()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    const n = (props.hour + 1) % 24
    emit('update:hour', n)
    hourStr.value = pad2(n, 23)
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    const n = (props.hour - 1 + 24) % 24
    emit('update:hour', n)
    hourStr.value = pad2(n, 23)
  }
}

// --- Minute ---
function onMinuteInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value.replace(/\D/g, '').slice(0, 2)
  minuteStr.value = raw
  if (raw.length === 2) {
    const n = Math.min(59, parseInt(raw, 10))
    emit('update:minute', n)
    minuteStr.value = pad2(n, 59)
  }
}

function onMinuteBlur() {
  const n = Math.min(59, Math.max(0, parseInt(minuteStr.value || '0', 10) || 0))
  emit('update:minute', n)
  minuteStr.value = pad2(n, 59)
}

function onMinuteKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    const n = (props.minute + 1) % 60
    emit('update:minute', n)
    minuteStr.value = pad2(n, 59)
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    const n = (props.minute - 1 + 60) % 60
    emit('update:minute', n)
    minuteStr.value = pad2(n, 59)
  } else if (e.key === 'ArrowLeft') {
    const input = e.target as HTMLInputElement
    if (input.selectionStart === 0 && input.selectionEnd === 0) {
      e.preventDefault()
      hourRef.value?.focus()
      hourRef.value?.select()
    }
  }
}

const containerClass = computed(() => props.size === 'sm'
  ? 'h-8 px-2.5 text-[12px]'
  : 'h-9 px-3 text-[13px]',
)

const segmentClass = computed(() => props.size === 'sm'
  ? 'w-[1.5rem] text-[12px]'
  : 'w-[1.6rem] text-[13px]',
)
</script>

<template>
  <div
    data-slot="input-group"
    :class="cn(
      'inline-flex items-center rounded-md tracking-[0.01em]',
      'disabled:pointer-events-none',
      containerClass,
      props.disabled ? 'opacity-40' : '',
      props.class,
    )"
  >
    <input
      ref="hourRef"
      :value="hourStr"
      type="text"
      inputmode="numeric"
      maxlength="2"
      :disabled="disabled"
      :class="cn(
        'bg-transparent text-center text-foreground outline-none tabular-nums',
        'disabled:pointer-events-none',
        segmentClass,
      )"
      @input="onHourInput"
      @blur="onHourBlur"
      @focus="($event.target as HTMLInputElement).select()"
      @keydown="onHourKeydown"
    >
    <span
      class="select-none text-muted-foreground"
      :class="props.size === 'sm' ? 'text-[12px]' : 'text-[13px]'"
    >:</span>
    <input
      ref="minuteRef"
      :value="minuteStr"
      type="text"
      inputmode="numeric"
      maxlength="2"
      :disabled="disabled"
      :class="cn(
        'bg-transparent text-center text-foreground outline-none tabular-nums',
        'disabled:pointer-events-none',
        segmentClass,
      )"
      @input="onMinuteInput"
      @blur="onMinuteBlur"
      @focus="($event.target as HTMLInputElement).select()"
      @keydown="onMinuteKeydown"
    >
  </div>
</template>
