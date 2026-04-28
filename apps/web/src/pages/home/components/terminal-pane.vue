<template>
  <div class="absolute inset-0 flex flex-col bg-[#1a1b26]">
    <div
      ref="wrapperRef"
      class="flex-1 relative min-h-0 terminal-wrapper"
    >
      <div
        ref="containerRef"
        class="absolute inset-2 terminal-container"
      />
    </div>
    <div
      v-if="status === 'disconnected'"
      class="shrink-0 flex items-center justify-end gap-2 px-3 py-1.5 text-xs text-muted-foreground border-t border-border bg-background"
    >
      <span>{{ t('bots.terminal.status.disconnected') }}</span>
      <Button
        size="sm"
        variant="outline"
        @click="reconnect"
      >
        {{ t('bots.terminal.reconnect') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { client } from '@memohai/sdk/client'
import { Button } from '@memohai/ui'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const TERMINAL_OPTIONS = {
  cursorBlink: true,
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  theme: {
    background: '#1a1b26',
    foreground: '#a9b1d6',
    cursor: '#c0caf5',
    selectionBackground: '#33467c',
  },
} as const

const wrapperRef = ref<HTMLDivElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected'>('idle')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
let fitTimer: ReturnType<typeof setTimeout> | null = null
let disposables: Array<{ dispose(): void }> = []

function resolveTerminalWsUrl(cols: number, rows: number): string {
  const baseUrl = String(client.getConfig().baseUrl || '').trim()
  const token = localStorage.getItem('token') ?? ''
  const path = `/bots/${encodeURIComponent(props.botId)}/container/terminal/ws`
  const query = `?token=${encodeURIComponent(token)}&cols=${cols}&rows=${rows}`

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

function closeWs() {
  if (ws) {
    ws.onclose = null
    ws.onerror = null
    ws.onmessage = null
    ws.close()
    ws = null
  }
}

function connectWs() {
  if (!terminal) return
  closeWs()

  fitAddon?.fit()

  const cols = terminal.cols
  const rows = terminal.rows

  status.value = 'connecting'
  const url = resolveTerminalWsUrl(cols, rows)
  const socket = new WebSocket(url)
  socket.binaryType = 'arraybuffer'
  ws = socket

  socket.onopen = () => {
    status.value = 'connected'
  }

  socket.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      terminal?.write(new Uint8Array(event.data))
    } else if (typeof event.data === 'string') {
      terminal?.write(event.data)
    }
  }

  socket.onclose = () => {
    status.value = 'disconnected'
    terminal?.write('\r\n\x1b[31m[Connection closed]\x1b[0m\r\n')
  }

  socket.onerror = () => {
    status.value = 'disconnected'
  }

  for (const d of disposables) d.dispose()
  disposables = []

  disposables.push(
    terminal.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    }),
    terminal.onResize(({ cols: c, rows: r }) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: c, rows: r }))
      }
    }),
  )
}

function reconnect() {
  connectWs()
}

function setupResizeObserver() {
  if (resizeObserver || !wrapperRef.value) return
  resizeObserver = new ResizeObserver(() => {
    if (fitTimer) clearTimeout(fitTimer)
    fitTimer = setTimeout(() => {
      fitAddon?.fit()
    }, 50)
  })
  resizeObserver.observe(wrapperRef.value)
}

onMounted(() => {
  if (!containerRef.value) return
  const term = new Terminal({ ...TERMINAL_OPTIONS })
  const fa = new FitAddon()
  term.loadAddon(fa)
  term.open(containerRef.value)

  terminal = term
  fitAddon = fa

  nextTick(() => {
    fa.fit()
    connectWs()
    setupResizeObserver()
  })
})

onBeforeUnmount(() => {
  if (fitTimer) {
    clearTimeout(fitTimer)
    fitTimer = null
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  closeWs()
  for (const d of disposables) d.dispose()
  disposables = []
  terminal?.dispose()
  terminal = null
  fitAddon = null
})
</script>

<style scoped>
.terminal-wrapper {
  background-color: #1a1b26;
}
</style>
