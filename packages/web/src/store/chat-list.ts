import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import type { user, robot } from '@memoh/shared'
import {
  fetchBots,
  createSession,
  sendChatMessage,
  createStreamConnection,
} from '@/composables/api/useChat'

export const useChatList = defineStore('chatList', () => {
  const chatList = reactive<(user | robot)[]>([])
  const loading = ref(false)
  const botId = ref<string | null>(null)
  const sessionId = ref<string | null>(null)
  const abortStream = ref<(() => void) | null>(null)

  const nextId = () => `${Date.now()}-${Math.floor(Math.random() * 1000)}`

  const addUserMessage = (text: string) => {
    chatList.push({
      description: text,
      time: new Date(),
      action: 'user',
      id: nextId(),
    })
  }

  const addRobotMessage = (text: string) => {
    chatList.push({
      description: text,
      time: new Date(),
      action: 'robot',
      id: nextId(),
      type: 'Memoh Agent',
      state: 'complete',
    })
  }

  const ensureSession = async () => {
    if (botId.value && sessionId.value) return

    const bots = await fetchBots()
    if (!bots.length) throw new Error('No bots found')

    botId.value = botId.value ?? bots[0]!.id
    sessionId.value = await createSession(botId.value)

    if (botId.value && sessionId.value) {
      // 关闭旧流
      abortStream.value?.()
      abortStream.value = createStreamConnection(
        botId.value,
        sessionId.value,
        addRobotMessage,
      )
    }
  }

  const sendMessage = async (text: string) => {
    const trimmed = text.trim()
    if (!trimmed) return

    loading.value = true
    try {
      addUserMessage(trimmed)
      await ensureSession()
      if (!botId.value || !sessionId.value) {
        throw new Error('Session not ready')
      }
      await sendChatMessage(botId.value, sessionId.value, trimmed)
    } finally {
      loading.value = false
    }
  }

  return {
    chatList,
    loading,
    sendMessage,
  }
})
