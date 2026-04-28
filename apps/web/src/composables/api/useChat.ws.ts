import { client } from '@memohai/sdk/client'
import type { ChatAttachment, UIStreamEvent, UIStreamEventHandler } from './useChat.types'

export interface WSClientMessage {
  type: 'message' | 'abort' | 'tool_approval_response'
  text?: string
  session_id?: string
  attachments?: ChatAttachment[]
  model_id?: string
  reasoning_effort?: string
  approval_id?: string
  short_id?: number
  tool_call_id?: string
  decision?: 'approve' | 'reject'
  reason?: string
}

export interface ChatWebSocket {
  send: (msg: WSClientMessage) => void
  abort: () => void
  close: () => void
  readonly connected: boolean
  onOpen: (() => void) | null
  onClose: (() => void) | null
}

function resolveWebSocketUrl(botId: string): string {
  const baseUrl = String(client.getConfig().baseUrl || '').trim()
  const path = `/bots/${encodeURIComponent(botId)}/web/ws`

  if (!baseUrl || baseUrl.startsWith('/')) {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    const base = baseUrl || '/api'
    return `${proto}//${loc.host}${base.replace(/\/+$/, '')}${path}`
  }

  try {
    const url = new URL(path, baseUrl)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    return url.toString()
  } catch {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${loc.host}/api${path}`
  }
}

export function connectWebSocket(
  botId: string,
  onStreamEvent: UIStreamEventHandler,
): ChatWebSocket {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')

  const wsUrl = resolveWebSocketUrl(id)
  const token = localStorage.getItem('token') ?? ''
  const url = token ? `${wsUrl}?token=${encodeURIComponent(token)}` : wsUrl

  let ws: WebSocket | null = null
  let isConnected = false
  let closed = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelay = 1000
  const sendQueue: string[] = []

  const handle: ChatWebSocket = {
    send(msg: WSClientMessage) {
      const payload = JSON.stringify(msg)
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(payload)
        return
      }
      sendQueue.push(payload)
    },
    abort() {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'abort' }))
      }
    },
    close() {
      closed = true
      if (reconnectTimer) {
        clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      if (ws) {
        ws.close()
        ws = null
      }
      isConnected = false
    },
    get connected() {
      return isConnected
    },
    onOpen: null,
    onClose: null,
  }

  function connect() {
    if (closed) return
    ws = new WebSocket(url)

    ws.onopen = () => {
      isConnected = true
      reconnectDelay = 1000
      while (sendQueue.length > 0 && ws?.readyState === WebSocket.OPEN) {
        ws.send(sendQueue.shift()!)
      }
      handle.onOpen?.()
    }

    ws.onclose = () => {
      isConnected = false
      handle.onClose?.()
      scheduleReconnect()
    }

    ws.onerror = () => {
      // onerror is always followed by onclose; reconnect handled there.
    }

    ws.onmessage = (event) => {
      if (typeof event.data !== 'string') return
      try {
        const parsed = JSON.parse(event.data)
        if (!parsed || typeof parsed !== 'object') return
        const eventType = String(parsed.type ?? '').trim()
        if (eventType !== 'start' && eventType !== 'message' && eventType !== 'end' && eventType !== 'error') {
          return
        }
        onStreamEvent(parsed as UIStreamEvent)
      } catch {
        // Ignore unparsable messages.
      }
    }
  }

  function scheduleReconnect() {
    if (closed) return
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, reconnectDelay)
    reconnectDelay = Math.min(reconnectDelay * 1.5, 10000)
  }

  connect()
  return handle
}
