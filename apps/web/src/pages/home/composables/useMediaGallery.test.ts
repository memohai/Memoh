import { ref } from 'vue'
import { client } from '@memohai/sdk/client'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveUrl, useMediaGallery } from './useMediaGallery'
import type { ChatMessage } from '@/store/chat-list'

function stubLocalStorage() {
  const store = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value)
    }),
    removeItem: vi.fn((key: string) => {
      store.delete(key)
    }),
    clear: vi.fn(() => {
      store.clear()
    }),
  })
}

describe('useMediaGallery', () => {
  beforeEach(() => {
    stubLocalStorage()
    client.setConfig({ baseUrl: '/api' })
    localStorage.clear()
  })

  it('skips background task system turns when collecting media', () => {
    const messages = ref<ChatMessage[]>([
      {
        id: 'system-task-1',
        role: 'system',
        kind: 'background_task',
        backgroundTask: {
          taskId: 'task-1',
          status: 'completed',
        },
        timestamp: '2026-05-11T10:00:00Z',
        streaming: false,
      },
      {
        id: 'assistant-1',
        role: 'assistant',
        messages: [
          {
            id: 0,
            type: 'attachments',
            attachments: [
              {
                type: 'image',
                url: 'https://example.com/image.png',
              },
            ],
          },
        ],
        timestamp: '2026-05-11T10:00:01Z',
        streaming: false,
      },
    ])

    const gallery = useMediaGallery(messages)

    expect(() => gallery.items.value).not.toThrow()
    expect(gallery.items.value).toEqual([
      {
        src: 'https://example.com/image.png',
        type: 'image',
        name: undefined,
      },
    ])
  })

  it('uses the SDK base URL for stored media assets', () => {
    client.setConfig({ baseUrl: 'http://127.0.0.1:18731' })
    localStorage.setItem('token', 'token with spaces')

    expect(resolveUrl({
      type: 'image',
      bot_id: 'bot 1',
      content_hash: 'sha256:asset/1',
    })).toBe('http://127.0.0.1:18731/bots/bot%201/media/sha256%3Aasset%2F1?token=token%20with%20spaces')
  })
})
