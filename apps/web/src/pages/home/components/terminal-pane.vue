<template>
  <div class="absolute inset-0 flex flex-col bg-[var(--terminal-background)]">
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
import { ref, onMounted, onBeforeUnmount, onActivated, onDeactivated, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SerializeAddon } from '@xterm/addon-serialize'
import { client } from '@memohai/sdk/client'
import { Button } from '@memohai/ui'
import {
  readTerminalSnapshot,
  terminalCacheKey,
  writeTerminalSnapshot,
} from '@/composables/useTerminalCache'
import { useSettingsStore } from '@/store/settings'
import '@xterm/xterm/css/xterm.css'

const props = withDefaults(defineProps<{
  botId: string
  tabId: string
  active?: boolean
}>(), {
  active: false,
})

const { t } = useI18n()
const settingsStore = useSettingsStore()

function cssVar(name: string): string {
  if (typeof document === 'undefined') return ''
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function resolveTerminalTheme() {
  return {
    background: cssVar('--terminal-background') || 'black',
    foreground: cssVar('--terminal-foreground') || 'white',
    cursor: cssVar('--terminal-cursor') || 'white',
    selectionBackground: cssVar('--terminal-selection') || 'gray',
  }
}

const TERMINAL_OPTIONS = {
  cursorBlink: true,
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
} as const

const wrapperRef = ref<HTMLDivElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected'>('idle')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let serializeAddon: SerializeAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
let fitTimer: ReturnType<typeof setTimeout> | null = null
let disposables: Array<{ dispose(): void }> = []

function currentCacheKey(): string {
  return terminalCacheKey(props.botId, props.tabId)
}

function persistSnapshot() {
  if (!serializeAddon) return
  try {
    writeTerminalSnapshot(currentCacheKey(), serializeAddon.serialize())
  } catch (error) {
    console.warn('Failed to serialize terminal buffer:', error)
  }
}

function fitTerminal() {
  if (!props.active) return
  fitAddon?.fit()
}

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

  fitTerminal()

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
    if (!props.active) return
    if (fitTimer) clearTimeout(fitTimer)
    fitTimer = setTimeout(() => {
      fitTerminal()
    }, 50)
  })
  resizeObserver.observe(wrapperRef.value)
}

onMounted(() => {
  if (!containerRef.value) return
  const term = new Terminal({ ...TERMINAL_OPTIONS, theme: resolveTerminalTheme() })
  const fa = new FitAddon()
  const sa = new SerializeAddon()
  term.loadAddon(fa)
  term.loadAddon(sa)
  term.open(containerRef.value)

  terminal = term
  fitAddon = fa
  serializeAddon = sa

  const snapshot = readTerminalSnapshot(currentCacheKey())
  if (snapshot) {
    term.write(snapshot)
  }

  nextTick(() => {
    setupResizeObserver()
    if (props.active) {
      fa.fit()
      connectWs()
    }
  })
})

onActivated(() => {
  nextTick(() => {
    fitTerminal()
  })
})

onDeactivated(() => {
  persistSnapshot()
})

watch(
  () => props.active,
  async (active) => {
    if (!active) {
      persistSnapshot()
      return
    }
    await nextTick()
    fitTerminal()
    if (status.value === 'idle') {
      connectWs()
    }
  },
  { flush: 'post' },
)

watch(
  [() => settingsStore.theme, () => settingsStore.colorScheme],
  () => {
    if (terminal) {
      terminal.options.theme = resolveTerminalTheme()
    }
  },
)

onBeforeUnmount(() => {
  persistSnapshot()
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
  serializeAddon = null
})
</script>
