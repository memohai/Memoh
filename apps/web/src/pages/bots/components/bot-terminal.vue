<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { client } from '@memoh/sdk/client'
import { Button, Spinner } from '@memoh/ui'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import '@xterm/xterm/css/xterm.css'

const route = useRoute()
const { t } = useI18n()

const botId = computed(() => route.params.botId as string)
const activeTab = useSyncedQueryParam('tab', 'overview')

const wrapperRef = ref<HTMLDivElement | null>(null)
const terminalRef = ref<HTMLDivElement | null>(null)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected'>('idle')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
let fitTimer: ReturnType<typeof setTimeout> | null = null

function resolveTerminalWsUrl(botIdValue: string, cols: number, rows: number): string {
  const baseUrl = String(client.getConfig().baseUrl || '').trim()
  const token = localStorage.getItem('token') ?? ''
  const path = `/bots/${encodeURIComponent(botIdValue)}/container/terminal/ws`
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

function connect() {
  if (!terminalRef.value) return
  cleanup()

  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1a1b26',
      foreground: '#a9b1d6',
      cursor: '#c0caf5',
      selectionBackground: '#33467c',
    },
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(terminalRef.value)

  nextTick(() => {
    fitAddon!.fit()
  })

  const cols = terminal.cols
  const rows = terminal.rows

  status.value = 'connecting'
  const url = resolveTerminalWsUrl(botId.value, cols, rows)
  ws = new WebSocket(url)
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    status.value = 'connected'
  }

  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      terminal?.write(new Uint8Array(event.data))
    } else if (typeof event.data === 'string') {
      terminal?.write(event.data)
    }
  }

  ws.onclose = () => {
    status.value = 'disconnected'
    terminal?.write('\r\n\x1b[31m[Connection closed]\x1b[0m\r\n')
  }

  ws.onerror = () => {
    status.value = 'disconnected'
  }

  terminal.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new TextEncoder().encode(data))
    }
  })

  terminal.onResize(({ cols: c, rows: r }) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'resize', cols: c, rows: r }))
    }
  })

  resizeObserver = new ResizeObserver(() => {
    if (fitTimer) clearTimeout(fitTimer)
    fitTimer = setTimeout(() => {
      fitAddon?.fit()
    }, 50)
  })
  if (wrapperRef.value) {
    resizeObserver.observe(wrapperRef.value)
  }
}

function cleanup() {
  if (fitTimer) {
    clearTimeout(fitTimer)
    fitTimer = null
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  if (ws) {
    ws.onclose = null
    ws.onerror = null
    ws.onmessage = null
    ws.close()
    ws = null
  }
  terminal?.dispose()
  terminal = null
  fitAddon = null
  status.value = 'idle'
}

function handleReconnect() {
  connect()
}

onMounted(() => {
  if (activeTab.value === 'terminal') {
    connect()
  }
})

watch(activeTab, (tab) => {
  if (tab === 'terminal' && status.value === 'idle') {
    nextTick(() => connect())
  }
})

onBeforeUnmount(() => {
  cleanup()
})
</script>

<template>
  <div class="flex flex-col absolute inset-0 p-4">
    <div class="flex items-center justify-between mb-3">
      <h3 class="text-lg font-semibold">
        {{ t('bots.terminal.title') }}
      </h3>
      <div class="flex items-center gap-2">
        <span
          class="inline-flex items-center gap-1.5 text-xs"
          :class="{
            'text-green-500': status === 'connected',
            'text-yellow-500': status === 'connecting',
            'text-muted-foreground': status === 'idle' || status === 'disconnected',
          }"
        >
          <span
            class="size-2 rounded-full"
            :class="{
              'bg-green-500': status === 'connected',
              'bg-yellow-500 animate-pulse': status === 'connecting',
              'bg-muted-foreground': status === 'idle' || status === 'disconnected',
            }"
          />
          {{ t(`bots.terminal.status.${status}`) }}
        </span>
        <Button
          v-if="status === 'disconnected'"
          size="sm"
          variant="outline"
          @click="handleReconnect"
        >
          {{ t('bots.terminal.reconnect') }}
        </Button>
      </div>
    </div>
    <div
      v-if="status === 'connecting'"
      class="flex items-center justify-center flex-1"
    >
      <Spinner class="size-6" />
    </div>
    <div
      ref="wrapperRef"
      class="flex-1 relative min-h-0 rounded-md overflow-hidden border border-border terminal-wrapper"
      :class="{ 'hidden': status === 'idle' && !terminalRef }"
    >
      <div
        ref="terminalRef"
        class="absolute inset-0 terminal-container"
      />
    </div>
  </div>
</template>

<style scoped>
.terminal-wrapper {
  background-color: #1a1b26;
}

.terminal-container {
  padding: 8px;
}
</style>
