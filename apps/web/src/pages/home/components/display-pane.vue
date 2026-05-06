<template>
  <div class="absolute inset-0 flex flex-col bg-black">
    <video
      ref="videoRef"
      class="size-full min-h-0 flex-1 bg-black object-contain"
      autoplay
      playsinline
      muted
      tabindex="0"
      @contextmenu.prevent
      @mousedown.prevent="onPointerDown"
      @mousemove="onPointerMove"
      @mouseup.prevent="onPointerUp"
      @mouseleave="onPointerLeave"
      @wheel.prevent="onWheel"
      @keydown.prevent="onKeyDown"
      @keyup.prevent="onKeyUp"
    />
    <div
      v-if="prepareProgress"
      class="absolute inset-0 flex items-center justify-center bg-background/95 px-6"
    >
      <div class="w-full max-w-[520px] rounded-lg border border-border bg-background p-5">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <p class="text-sm font-medium text-foreground">
              {{ t('chat.display.prepare.title') }}
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              {{ prepareProgress.message }}
            </p>
          </div>
          <span class="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
            {{ preparePercent }}%
          </span>
        </div>
        <div class="mt-4 h-2 w-full overflow-hidden rounded-full bg-muted">
          <div
            class="h-full rounded-full bg-foreground transition-all duration-300 ease-out"
            :style="{ width: `${preparePercent}%` }"
          />
        </div>
        <div class="mt-5 grid grid-cols-4 gap-2">
          <div
            v-for="stage in prepareStages"
            :key="stage.key"
            class="flex min-w-0 flex-col items-center gap-2 rounded-md border border-border bg-background px-2 py-3 text-center"
            :class="stage.active ? 'text-foreground' : 'text-muted-foreground'"
          >
            <component
              :is="stage.icon"
              class="size-4"
              :class="{ 'animate-pulse': stage.active }"
            />
            <span class="w-full truncate text-[11px] font-medium">
              {{ stage.label }}
            </span>
          </div>
        </div>
      </div>
    </div>
    <div
      v-if="status === 'connected' || displaySessionId"
      class="absolute right-2 top-2 flex items-center gap-1 rounded-md border border-border bg-background/95 p-1 text-xs text-muted-foreground"
    >
      <span class="max-w-[180px] truncate px-2">
        {{ title || t('chat.display.title') }}
      </span>
      <button
        v-if="closable !== false"
        type="button"
        class="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
        :title="t('chat.display.closeSession')"
        :aria-label="t('chat.display.closeSession')"
        @click="closeDisplayWindow"
      >
        <X class="size-3.5" />
      </button>
    </div>
    <div
      v-if="prepareProgress"
      class="shrink-0 border-t border-border bg-background px-3 py-2 text-xs text-muted-foreground"
    >
      <div class="mb-1.5 flex items-center justify-between gap-3">
        <span class="inline-flex min-w-0 items-center gap-2">
          <Spinner class="size-3.5 shrink-0" />
          <span class="truncate">{{ prepareProgress.message }}</span>
        </span>
        <span class="shrink-0 tabular-nums">{{ preparePercent }}%</span>
      </div>
      <div class="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div
          class="h-full rounded-full bg-foreground transition-all duration-300 ease-out"
          :style="{ width: `${preparePercent}%` }"
        />
      </div>
    </div>
    <div
      v-else-if="status !== 'connected'"
      class="shrink-0 flex items-center justify-end gap-2 px-3 py-1.5 text-xs text-muted-foreground border-t border-border bg-background"
    >
      <span>{{ statusLabel }}</span>
      <Button
        v-if="status === 'disconnected' || status === 'unavailable'"
        size="sm"
        variant="outline"
        @click="connect"
      >
        {{ t('chat.display.reconnect') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  deleteBotsByBotIdContainerDisplaySessionsBySessionId,
  getBotsByBotIdContainerDisplay,
  postBotsByBotIdContainerDisplayWebrtcOffer,
} from '@memohai/sdk'
import { Button, Spinner } from '@memohai/ui'
import { Globe, Monitor, Package, Wrench, X } from 'lucide-vue-next'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  postBotsByBotIdContainerDisplayPrepareStream,
  type DisplayPrepareStreamEvent,
} from '@/composables/api/useDisplayPrepareStream'

const props = defineProps<{
  botId: string
  tabId: string
  title?: string
  active?: boolean
  closable?: boolean
}>()

const emit = defineEmits<{
  close: []
  snapshot: [payload: { tabId: string; sessionId?: string; dataUrl: string }]
}>()

type DisplayStatus = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'unavailable'

interface DisplayOfferResponse {
  type: 'answer'
  sdp: string
  session_id?: string
}

interface DisplayInfoPayload {
  enabled?: boolean
  available?: boolean
  running?: boolean
  encoder_available?: boolean
  desktop_available?: boolean
  browser_available?: boolean
  toolkit_available?: boolean
  prepare_supported?: boolean
  prepare_system?: string
  unavailable_reason?: string
}

interface PrepareProgress {
  percent: number
  message: string
  step?: string
}

interface PrepareStage {
  key: string
  label: string
  icon: Component
  active: boolean
}

const { t } = useI18n()
const videoRef = ref<HTMLVideoElement | null>(null)
const status = ref<DisplayStatus>('idle')
const unavailableReason = ref('')
const prepareProgress = ref<PrepareProgress | null>(null)
const displaySessionId = ref('')
let peer: RTCPeerConnection | null = null
let inputChannel: RTCDataChannel | null = null
let pointerMask = 0
let lastPointerPoint: { x: number; y: number } | null = null
let snapshotTimer: ReturnType<typeof window.setInterval> | null = null

const statusLabel = computed(() => {
  if (status.value === 'unavailable') {
    return unavailableReason.value || t('chat.display.status.unavailable')
  }
  switch (status.value) {
    case 'connecting': return t('chat.display.status.connecting')
    case 'connected': return t('chat.display.status.connected')
    case 'disconnected': return t('chat.display.status.disconnected')
    default: return t('chat.display.status.idle')
  }
})

const preparePercent = computed(() => Math.max(0, Math.min(100, Math.round(prepareProgress.value?.percent ?? 0))))

const prepareStageOrder = ['checking', 'system', 'installing', 'browser', 'starting', 'desktop', 'complete']

const prepareStages = computed<PrepareStage[]>(() => {
  const current = prepareProgress.value?.step ?? 'checking'
  const currentIndex = Math.max(0, prepareStageOrder.indexOf(current))
  return [
    { key: 'checking', label: t('chat.display.prepare.stageCheck'), icon: Wrench },
    { key: 'installing', label: t('chat.display.prepare.stageInstall'), icon: Package },
    { key: 'browser', label: t('chat.display.prepare.stageBrowser'), icon: Globe },
    { key: 'desktop', label: t('chat.display.prepare.stageDesktop'), icon: Monitor },
  ].map((stage) => ({
    ...stage,
    active: stage.key === current || prepareStageOrder.indexOf(stage.key) <= currentIndex,
  }))
})

function formatUnavailableReason(reason: string): string {
  switch (reason) {
    case 'container not reachable':
      return t('chat.display.unavailable.container')
    case 'display bundle unavailable':
      return t('chat.display.unavailable.bundle')
    case 'display server not reachable':
      return t('chat.display.unavailable.server')
    case 'gstreamer unavailable':
      return t('chat.display.unavailable.encoder')
    case 'manager not configured':
      return t('chat.display.unavailable.manager')
    case 'browser unavailable':
      return t('chat.display.unavailable.browser')
    case 'desktop unavailable':
      return t('chat.display.unavailable.desktop')
    case 'toolkit unavailable':
      return t('chat.display.unavailable.toolkit')
    default:
      return reason || t('chat.display.status.unavailable')
  }
}

function cleanupLocal() {
  stopSnapshotCapture()
  pointerMask = 0
  lastPointerPoint = null
  if (inputChannel) {
    inputChannel.close()
    inputChannel = null
  }
  if (peer) {
    peer.close()
    peer = null
  }
  if (videoRef.value?.srcObject) {
    const stream = videoRef.value.srcObject as MediaStream
    for (const track of stream.getTracks()) {
      track.stop()
    }
    videoRef.value.srcObject = null
  }
}

function closeRemoteSession() {
  const sessionID = displaySessionId.value
  displaySessionId.value = ''
  if (!sessionID) return
  void deleteBotsByBotIdContainerDisplaySessionsBySessionId({
    path: {
      bot_id: props.botId,
      session_id: sessionID,
    },
  }).catch(() => {})
}

function cleanup() {
  closeRemoteSession()
  cleanupLocal()
}

function closeDisplayWindow() {
  cleanup()
  status.value = 'disconnected'
  emit('close')
}

function setPeerStatus(next: RTCPeerConnectionState) {
  switch (next) {
    case 'connected':
      status.value = 'connected'
      startSnapshotCapture()
      break
    case 'failed':
    case 'closed':
    case 'disconnected':
      status.value = 'disconnected'
      stopSnapshotCapture()
      break
    default:
      status.value = 'connecting'
  }
}

function startSnapshotCapture() {
  if (snapshotTimer) return
  snapshotTimer = window.setInterval(captureSnapshot, 1800)
  window.setTimeout(captureSnapshot, 250)
}

function stopSnapshotCapture() {
  if (!snapshotTimer) return
  window.clearInterval(snapshotTimer)
  snapshotTimer = null
}

function captureSnapshot() {
  const video = videoRef.value
  if (!video || video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA || !video.videoWidth || !video.videoHeight) {
    return
  }
  try {
    const width = 320
    const height = Math.round(width * video.videoHeight / video.videoWidth)
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.drawImage(video, 0, 0, width, height)
    emit('snapshot', {
      tabId: props.tabId,
      sessionId: displaySessionId.value || undefined,
      dataUrl: canvas.toDataURL('image/jpeg', 0.72),
    })
  } catch {
    // Some browsers can briefly refuse drawing a still-starting WebRTC frame.
  }
}

function waitForIceGatheringComplete(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === 'complete') {
    return Promise.resolve()
  }
  return new Promise((resolve) => {
    const timeout = window.setTimeout(done, 3000)
    function done() {
      window.clearTimeout(timeout)
      pc.removeEventListener('icegatheringstatechange', onChange)
      resolve()
    }
    function onChange() {
      if (pc.iceGatheringState === 'complete') {
        done()
      }
    }
    pc.addEventListener('icegatheringstatechange', onChange)
  })
}

async function createDisplayAnswer(pc: RTCPeerConnection): Promise<DisplayOfferResponse> {
  const local = pc.localDescription
  if (!local?.sdp) {
    throw new Error('local WebRTC offer is unavailable')
  }
  const { data } = await postBotsByBotIdContainerDisplayWebrtcOffer({
    path: { bot_id: props.botId },
    body: {
      type: local.type,
      sdp: local.sdp,
      session_id: displaySessionId.value || undefined,
      candidate_host: window.location.hostname,
    },
    throwOnError: true,
  })
  if (!data?.sdp) {
    throw new Error('display WebRTC answer is empty')
  }
  return { type: 'answer', sdp: data.sdp, session_id: data.session_id }
}

function sendInput(payload: Record<string, unknown>) {
  if (inputChannel?.readyState !== 'open') return
  inputChannel.send(JSON.stringify(payload))
}

function buttonBit(button: number): number {
  switch (button) {
    case 0: return 1
    case 1: return 2
    case 2: return 4
    default: return 0
  }
}

function resolveVideoPoint(event: MouseEvent | WheelEvent): { x: number; y: number } | null {
  const video = videoRef.value
  if (!video) return null
  const rect = video.getBoundingClientRect()
  const sourceWidth = video.videoWidth || 1280
  const sourceHeight = video.videoHeight || 800
  const scale = Math.min(rect.width / sourceWidth, rect.height / sourceHeight)
  const width = sourceWidth * scale
  const height = sourceHeight * scale
  const offsetX = (rect.width - width) / 2
  const offsetY = (rect.height - height) / 2
  const x = (event.clientX - rect.left - offsetX) / scale
  const y = (event.clientY - rect.top - offsetY) / scale
  if (x < 0 || y < 0 || x > sourceWidth || y > sourceHeight) {
    return null
  }
  return {
    x: Math.max(0, Math.min(sourceWidth - 1, Math.round(x))),
    y: Math.max(0, Math.min(sourceHeight - 1, Math.round(y))),
  }
}

function sendPointer(event: MouseEvent | WheelEvent, mask = pointerMask) {
  const point = resolveVideoPoint(event)
  if (!point) return
  lastPointerPoint = point
  sendInput({
    type: 'pointer',
    x: point.x,
    y: point.y,
    button_mask: mask,
  })
}

function onPointerDown(event: MouseEvent) {
  videoRef.value?.focus()
  pointerMask |= buttonBit(event.button)
  sendPointer(event)
}

function onPointerMove(event: MouseEvent) {
  sendPointer(event)
}

function onPointerUp(event: MouseEvent) {
  pointerMask &= ~buttonBit(event.button)
  sendPointer(event)
}

function onPointerLeave(event: MouseEvent) {
  pointerMask = 0
  const point = resolveVideoPoint(event) ?? lastPointerPoint
  if (!point) return
  sendInput({
    type: 'pointer',
    x: point.x,
    y: point.y,
    button_mask: 0,
  })
}

function onWheel(event: WheelEvent) {
  const bit = event.deltaY < 0 ? 8 : 16
  sendPointer(event, pointerMask | bit)
  sendPointer(event, pointerMask)
}

function keysymForEvent(event: KeyboardEvent): number | null {
  if (event.key.length === 1) {
    return event.key.codePointAt(0) ?? null
  }
  const keysyms: Record<string, number> = {
    Backspace: 0xff08,
    Tab: 0xff09,
    Enter: 0xff0d,
    Escape: 0xff1b,
    Delete: 0xffff,
    Home: 0xff50,
    ArrowLeft: 0xff51,
    ArrowUp: 0xff52,
    ArrowRight: 0xff53,
    ArrowDown: 0xff54,
    PageUp: 0xff55,
    PageDown: 0xff56,
    End: 0xff57,
    Insert: 0xff63,
    Shift: 0xffe1,
    Control: 0xffe3,
    Alt: 0xffe9,
    Meta: 0xffeb,
  }
  if (/^F([1-9]|1[0-2])$/.test(event.key)) {
    return 0xffbe + Number(event.key.slice(1)) - 1
  }
  return keysyms[event.key] ?? null
}

function sendKey(event: KeyboardEvent, down: boolean) {
  const keysym = keysymForEvent(event)
  if (!keysym) return
  sendInput({
    type: 'key',
    keysym,
    down,
  })
}

function onKeyDown(event: KeyboardEvent) {
  if (event.repeat) return
  sendKey(event, true)
}

function onKeyUp(event: KeyboardEvent) {
  sendKey(event, false)
}

async function loadDisplayInfo(): Promise<DisplayInfoPayload> {
  const { data } = await getBotsByBotIdContainerDisplay({
    path: { bot_id: props.botId },
    throwOnError: true,
  })
  return data ?? {}
}

function isDisplayReady(info: DisplayInfoPayload): boolean {
  return info.enabled === true
    && info.available === true
    && info.running === true
    && info.desktop_available !== false
    && info.browser_available !== false
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

async function waitForDisplayReady(): Promise<DisplayInfoPayload> {
  let last = await loadDisplayInfo()
  for (let attempt = 0; attempt < 12 && !isDisplayReady(last); attempt += 1) {
    await delay(500)
    last = await loadDisplayInfo()
  }
  return last
}

function canPrepareDisplay(info: DisplayInfoPayload): boolean {
  const reason = info.unavailable_reason ?? ''
  if (!info.enabled) return false
  if (reason === 'container not reachable' || reason === 'manager not configured') return false
  if (info.encoder_available === false && reason === 'gstreamer unavailable') return false
  return !info.available
    || !info.running
    || info.desktop_available === false
    || info.browser_available === false
}

function prepareEventMessage(event: DisplayPrepareStreamEvent): string {
  switch (event.step) {
    case 'checking': return t('chat.display.prepare.checking')
    case 'toolkit': return t('chat.display.prepare.toolkit')
    case 'system': return t('chat.display.prepare.system')
    case 'installing': return t('chat.display.prepare.installing')
    case 'browser': return t('chat.display.prepare.browser')
    case 'starting': return t('chat.display.prepare.starting')
    case 'desktop': return t('chat.display.prepare.desktop')
    case 'complete': return t('chat.display.prepare.complete')
    default: return event.message || t('chat.display.prepare.default')
  }
}

async function prepareDisplay(): Promise<boolean> {
  prepareProgress.value = {
    percent: 5,
    message: t('chat.display.prepare.checking'),
    step: 'checking',
  }
  try {
    const { stream } = await postBotsByBotIdContainerDisplayPrepareStream({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    for await (const event of stream) {
      if (event.type === 'error') {
        throw new Error(event.message)
      }
      prepareProgress.value = {
        percent: event.percent ?? prepareProgress.value?.percent ?? 0,
        message: prepareEventMessage(event),
        step: event.step ?? prepareProgress.value?.step,
      }
      if (event.type === 'complete') {
        return true
      }
    }
    return true
  } catch (error) {
    status.value = 'unavailable'
    unavailableReason.value = resolveApiErrorMessage(error, t('chat.display.prepare.failed'))
    return false
  } finally {
    if (status.value === 'unavailable') {
      prepareProgress.value = null
    }
  }
}

async function connect() {
  cleanup()
  status.value = 'connecting'
  unavailableReason.value = ''
  prepareProgress.value = null

  try {
    let info = await loadDisplayInfo()
    if (!info.enabled) {
      status.value = 'unavailable'
      unavailableReason.value = t('chat.display.unavailable.disabled')
      return
    }
    if (canPrepareDisplay(info)) {
      const prepared = await prepareDisplay()
      if (!prepared) return
      info = await waitForDisplayReady()
    }
    if (!info.available || !info.running) {
      status.value = 'unavailable'
      unavailableReason.value = formatUnavailableReason(info.unavailable_reason ?? '')
      prepareProgress.value = null
      return
    }
    if (info.desktop_available === false) {
      status.value = 'unavailable'
      unavailableReason.value = formatUnavailableReason('desktop unavailable')
      prepareProgress.value = null
      return
    }
    if (info.browser_available === false) {
      status.value = 'unavailable'
      unavailableReason.value = formatUnavailableReason('browser unavailable')
      prepareProgress.value = null
      return
    }
  } catch (error) {
    status.value = 'unavailable'
    unavailableReason.value = resolveApiErrorMessage(error, t('chat.display.status.unavailable'))
    prepareProgress.value = null
    return
  }

  const next = new RTCPeerConnection()
  peer = next
  inputChannel = next.createDataChannel('display-input', { ordered: true })
  next.addTransceiver('video', { direction: 'recvonly' })
  next.addEventListener('connectionstatechange', () => setPeerStatus(next.connectionState))
  next.addEventListener('track', (event) => {
    const video = videoRef.value
    if (!video) return
    video.srcObject = event.streams[0] ?? new MediaStream([event.track])
    void video.play()
  })

  try {
    const offer = await next.createOffer()
    await next.setLocalDescription(offer)
    await waitForIceGatheringComplete(next)
    const answer = await createDisplayAnswer(next)
    displaySessionId.value = answer.session_id ?? ''
    await next.setRemoteDescription(new RTCSessionDescription(answer))
    prepareProgress.value = null
  } catch (error) {
    cleanupLocal()
    closeRemoteSession()
    status.value = 'unavailable'
    unavailableReason.value = resolveApiErrorMessage(error, t('chat.display.status.unavailable'))
    prepareProgress.value = null
  }
}

onMounted(() => {
  if (props.active) {
    void connect()
  }
})

watch(() => props.active, (active) => {
  if (!active) return
  if (peer || status.value === 'connecting' || status.value === 'connected') return
  void connect()
})

watch(() => props.botId, () => {
  if (!props.active) {
    cleanup()
    status.value = 'idle'
    return
  }
  void connect()
})
onBeforeUnmount(cleanup)
</script>
