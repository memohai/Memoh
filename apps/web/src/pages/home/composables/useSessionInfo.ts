import { computed, ref, type Ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { toast } from '@felinic/ui'
import { getBotsByBotIdSessionsBySessionIdStatus, postBotsByBotIdSessionsBySessionIdCompact } from '@memohai/sdk'
import type { HandlersSessionInfoResponse } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useChatStore } from '@/store/chat-list'

interface UseSessionInfoOptions {
  visible?: Ref<boolean>
  overrideModelId?: Ref<string>
  // The session status only reports a context window once the backend can
  // resolve one for the model; until then we fall back to the selected model's
  // configured window so the ring shows real headroom instead of an empty band.
  fallbackContextWindow?: Ref<number | null | undefined>
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
  const contextWindow = computed(() => {
    const fromStatus = info.value?.context_usage?.context_window
    if (fromStatus != null && fromStatus > 0) return fromStatus
    const fallback = options.fallbackContextWindow?.value
    return fallback != null && fallback > 0 ? fallback : null
  })
  const contextPercent = computed(() => {
    if (contextWindow.value == null || contextWindow.value <= 0) return 0
    return (usedTokens.value / contextWindow.value) * 100
  })

  // Compaction lives here (not in a component) so every surface that offers
  // it — the session info panel's button and the composer's /compact slash —
  // runs the identical action: same API call, same toasts, same cache
  // invalidation of this composable's own query.
  const { t } = useI18n()
  const queryCache = useQueryCache()
  const isCompacting = ref(false)

  async function triggerCompact() {
    const botId = currentBotId.value
    const sid = sessionId.value
    if (!botId || !sid || isCompacting.value) return

    isCompacting.value = true
    try {
      await postBotsByBotIdSessionsBySessionIdCompact({
        path: { bot_id: botId, session_id: sid },
        throwOnError: true,
      })
      toast.success(t('chat.compactSuccess'))
      queryCache.invalidateQueries({ key: ['session-status', botId, sid] })
    }
    catch (error) {
      toast.error(resolveApiErrorMessage(error, t('chat.compactFailed')))
    }
    finally {
      isCompacting.value = false
    }
  }

  return {
    info,
    usedTokens,
    contextWindow,
    contextPercent,
    currentBotId,
    sessionId,
    isCompacting,
    triggerCompact,
  }
}
