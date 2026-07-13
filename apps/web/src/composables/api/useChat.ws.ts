import { sdkAuthQuery, sdkWebSocketUrl } from '@/lib/api-client'
import type { ChatAttachment, RequestedSkillRequest, UIStreamEvent, UIStreamEventHandler } from './useChat.types'

export interface WSUserInputAnswer {
  question_id: string
  option_ids?: string[]
  custom_text?: string
  text?: string
}

export interface WSClientMessage {
  type: 'message' | 'abort' | 'tool_approval_response' | 'user_input_response' | 'retry_message' | 'edit_message' | 'runtime_subscribe' | 'runtime_unsubscribe' | 'steer_current_run'
  stream_id?: string
  generation?: string
  invocation_id?: string
  composer_scope?: string
  text?: string
  session_id?: string
  message_id?: string
  attachments?: ChatAttachment[]
  requested_skills?: RequestedSkillRequest[]
  model_id?: string
  reasoning_effort?: string
  approval_id?: string
  user_input_id?: string
  short_id?: number
  tool_call_id?: string
  decision?: 'approve' | 'reject'
  reason?: string
  answers?: WSUserInputAnswer[]
  canceled?: boolean
}

export interface ChatWebSocket {
  send: (msg: WSClientMessage) => void
  abort: (streamId: string, sessionId: string, generation: string) => void
  close: () => void
  readonly connected: boolean
  onOpen: (() => void) | null
  onClose: (() => void) | null
}

function resolveWebSocketUrl(botId: string): string {
  return sdkWebSocketUrl({
    url: '/bots/{bot_id}/web/ws',
    path: { bot_id: botId },
    query: sdkAuthQuery(),
  })
}

export function connectWebSocket(
  botId: string,
  onStreamEvent: UIStreamEventHandler,
): ChatWebSocket {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')

  const wsUrl = resolveWebSocketUrl(id)
  const url = wsUrl

  let ws: WebSocket | null = null
  let isConnected = false
  let closed = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelay = 1000

  const handle: ChatWebSocket = {
    send(msg: WSClientMessage) {
      const payload = JSON.stringify(msg)
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(payload)
        return
      }
      throw new Error('WebSocket is not connected')
    },
    abort(streamId: string, sessionId: string, generation: string) {
      const id = streamId.trim()
      const sid = sessionId.trim()
      const runGeneration = generation.trim()
      if (!id || !sid || !runGeneration) return
      handle.send({ type: 'abort', stream_id: id, session_id: sid, generation: runGeneration })
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
      let parsed: unknown
      try {
        parsed = JSON.parse(event.data)
      } catch {
        // Ignore unparsable messages.
        return
      }
      if (!parsed || typeof parsed !== 'object') return
      const eventType = String((parsed as { type?: unknown }).type ?? '').trim()
      if (
        eventType !== 'start'
        && eventType !== 'message'
        && eventType !== 'end'
        && eventType !== 'error'
        && eventType !== 'session_created'
        && eventType !== 'user_message'
        && eventType !== 'command_result'
        && eventType !== 'command_error'
        && eventType !== 'runtime_snapshot'
        && eventType !== 'runtime_delta'
        && eventType !== 'runtime_dropped'
      ) {
        return
      }
      onStreamEvent(parsed as UIStreamEvent)
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
