import { fetchApi } from '@/utils/request'

// ---- Types ----

export interface Bot {
  id: string
  name?: string
}

export interface BotsResponse {
  items: Bot[]
}

export interface CreateSessionResponse {
  session_id: string
}

// ---- Plain async functions (used by chat store) ----

export async function fetchBots(): Promise<Bot[]> {
  const res = await fetchApi<BotsResponse>('/bots')
  return res.items
}

export async function createSession(botId: string): Promise<string> {
  const res = await fetchApi<CreateSessionResponse>(
    `/bots/${botId}/web/sessions`,
    { method: 'POST' },
  )
  return res.session_id
}

export async function sendChatMessage(
  botId: string,
  sessionId: string,
  text: string,
): Promise<void> {
  await fetchApi(`/bots/${botId}/web/sessions/${sessionId}/messages`, {
    method: 'POST',
    body: { text },
  })
}

/**
 * 创建 SSE 流连接。返回 abort 函数。
 * 调用方负责处理 reader 中的数据。
 */
export function createStreamConnection(
  botId: string,
  sessionId: string,
  onMessage: (text: string) => void,
): () => void {
  const controller = new AbortController()
  const token = localStorage.getItem('token') ?? ''

  ;(async () => {
    const resp = await fetch(`/api/bots/${botId}/web/sessions/${sessionId}/stream`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${token}` },
      signal: controller.signal,
    }).catch(() => null)

    if (!resp?.ok || !resp.body) return

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      let idx: number
      while ((idx = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, idx).trim()
        buffer = buffer.slice(idx + 1)
        if (!line.startsWith('data:')) continue
        const payload = line.slice(5).trim()
        if (!payload || payload === '[DONE]') continue

        const text = extractTextFromEvent(payload)
        if (text) onMessage(text)
      }
    }
  })()

  return () => controller.abort()
}

function extractTextFromEvent(payload: string): string | null {
  try {
    const event = JSON.parse(payload)
    if (typeof event === 'string') return event
    if (typeof event?.text === 'string') return event.text
    if (typeof event?.content === 'string') return event.content
    if (typeof event?.data === 'string') return event.data
    if (typeof event?.data?.text === 'string') return event.data.text
    return null
  } catch {
    return payload
  }
}
