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
      v-if="status !== 'connected'"
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getBotsByBotIdContainerDisplay, postBotsByBotIdContainerDisplayWebrtcOffer } from '@memohai/sdk'
import { Button } from '@memohai/ui'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

type DisplayStatus = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'unavailable'

interface DisplayOfferResponse {
  type: 'answer'
  sdp: string
}

const { t } = useI18n()
const videoRef = ref<HTMLVideoElement | null>(null)
const status = ref<DisplayStatus>('idle')
const unavailableReason = ref('')
let peer: RTCPeerConnection | null = null
let inputChannel: RTCDataChannel | null = null
let pointerMask = 0
let lastPointerPoint: { x: number; y: number } | null = null

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
    default:
      return reason || t('chat.display.status.unavailable')
  }
}

function cleanup() {
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

function setPeerStatus(next: RTCPeerConnectionState) {
  switch (next) {
    case 'connected':
      status.value = 'connected'
      break
    case 'failed':
    case 'closed':
    case 'disconnected':
      status.value = 'disconnected'
      break
    default:
      status.value = 'connecting'
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
    },
    throwOnError: true,
  })
  if (!data?.sdp) {
    throw new Error('display WebRTC answer is empty')
  }
  return { type: 'answer', sdp: data.sdp }
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

async function connect() {
  cleanup()
  status.value = 'connecting'
  unavailableReason.value = ''

  try {
    const { data } = await getBotsByBotIdContainerDisplay({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    if (!data?.enabled) {
      status.value = 'unavailable'
      unavailableReason.value = t('chat.display.unavailable.disabled')
      return
    }
    if (!data.available || !data.running) {
      status.value = 'unavailable'
      unavailableReason.value = formatUnavailableReason(data.unavailable_reason ?? '')
      return
    }
  } catch (error) {
    status.value = 'unavailable'
    unavailableReason.value = resolveApiErrorMessage(error, t('chat.display.status.unavailable'))
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
    await next.setRemoteDescription(new RTCSessionDescription(answer))
  } catch (error) {
    cleanup()
    status.value = 'unavailable'
    unavailableReason.value = resolveApiErrorMessage(error, t('chat.display.status.unavailable'))
  }
}

onMounted(connect)
watch(() => props.botId, connect)
onBeforeUnmount(cleanup)
</script>
