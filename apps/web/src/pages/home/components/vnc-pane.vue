<template>
  <div class="absolute inset-0 flex flex-col bg-background">
    <div
      ref="containerRef"
      class="relative flex-1 min-h-0 overflow-hidden bg-muted"
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import RFB from '@novnc/novnc'
import { getBotsByBotIdContainerDisplay } from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import { Button } from '@memohai/ui'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const containerRef = ref<HTMLDivElement | null>(null)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected' | 'unavailable'>('idle')
const unavailableReason = ref('')
let rfb: RFB | null = null

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
    case 'manager not configured':
      return t('chat.display.unavailable.manager')
    default:
      return reason || t('chat.display.status.unavailable')
  }
}

function resolveDisplayWsUrl(): string {
  const baseUrl = String(client.getConfig().baseUrl || '').trim()
  const token = localStorage.getItem('token') ?? ''
  const path = `/bots/${encodeURIComponent(props.botId)}/container/display/ws`
  const query = `?token=${encodeURIComponent(token)}`

  if (!baseUrl || baseUrl.startsWith('/')) {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    const base = baseUrl || '/api'
    return `${proto}//${loc.host}${base.replace(/\/+$/, '')}${path}${query}`
  }

  try {
    const url = new URL(path, baseUrl)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    return url.toString() + query
  } catch {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${loc.host}/api${path}${query}`
  }
}

function cleanup() {
  if (rfb) {
    rfb.disconnect()
    rfb = null
  }
}

async function connect() {
  const target = containerRef.value
  if (!target) return
  cleanup()
  status.value = 'connecting'
  unavailableReason.value = ''
  target.replaceChildren()

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

  const next = new RFB(target, resolveDisplayWsUrl())
  next.scaleViewport = true
  next.resizeSession = false
  next.viewOnly = false
  next.background = 'var(--background)'
  next.addEventListener('connect', () => {
    status.value = 'connected'
  })
  next.addEventListener('disconnect', () => {
    status.value = 'disconnected'
  })
  next.addEventListener('securityfailure', () => {
    status.value = 'disconnected'
  })
  rfb = next
}

onMounted(connect)
onBeforeUnmount(cleanup)
</script>
