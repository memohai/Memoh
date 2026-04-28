import { computed, ref, type Ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useQuery } from '@pinia/colada'
import { getBotsByBotIdSessionsBySessionIdStatus } from '@memohai/sdk'
import type { HandlersSessionInfoResponse } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'

interface UseSessionInfoOptions {
  visible?: Ref<boolean>
  overrideModelId?: Ref<string>
}

export function useSessionInfo(options: UseSessionInfoOptions = {}) {
  const chatStore = useChatStore()
  const { currentBotId, sessionId } = storeToRefs(chatStore)
  const visible = options.visible ?? ref(true)

  const { data: info } = useQuery({
    key: () => [
      'session-status',
      currentBotId.value ?? '',
      sessionId.value ?? '',
      options.overrideModelId?.value ?? '',
    ],
    query: async () => {
      const { data } = await getBotsByBotIdSessionsBySessionIdStatus({
        path: {
          bot_id: currentBotId.value!,
          session_id: sessionId.value!,
        },
        query: {
          model_id: options.overrideModelId?.value || undefined,
        },
        throwOnError: true,
      })
      return data as HandlersSessionInfoResponse
    },
    enabled: () => !!currentBotId.value && !!sessionId.value && visible.value,
    refetchOnWindowFocus: false,
  })

  const usedTokens = computed(() => info.value?.context_usage?.used_tokens ?? 0)
  const contextWindow = computed(() => info.value?.context_usage?.context_window ?? null)
  const contextPercent = computed(() => {
    if (contextWindow.value == null || contextWindow.value <= 0) return 0
    return (usedTokens.value / contextWindow.value) * 100
  })

  return {
    info,
    usedTokens,
    contextWindow,
    contextPercent,
    currentBotId,
    sessionId,
  }
}
