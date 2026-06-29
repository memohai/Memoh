import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore } from '@/store/chat-list'
import type { AcpclientModelInfo } from '@memohai/sdk'

interface UseACPRuntimeOptions {
  botId: MaybeRefOrGetter<string | null | undefined>
  sessionId: MaybeRefOrGetter<string | null | undefined>
  enabled: MaybeRefOrGetter<boolean>
  onError?: (error: unknown) => void
}

export function useACPRuntime(options: UseACPRuntimeOptions) {
  const chatStore = useChatStore()
  const { acpRuntimeStatuses, acpRuntimePending } = storeToRefs(chatStore)

  const key = computed(() => {
    const botId = toValue(options.botId)?.trim() ?? ''
    const sessionId = toValue(options.sessionId)?.trim() ?? ''
    if (!botId || !sessionId || !toValue(options.enabled)) return ''
    return chatStore.acpRuntimeKey(botId, sessionId)
  })

  const runtime = computed(() => key.value ? acpRuntimeStatuses.value[key.value] : undefined)
  const isEnsuring = computed(() => key.value ? !!acpRuntimePending.value[key.value] : false)
  const models = computed<AcpclientModelInfo[]>(() => runtime.value?.models?.available_models ?? [])
  const currentModelId = computed(() => runtime.value?.models?.current_model_id ?? '')

  async function ensure(sessionId?: string) {
    const sid = sessionId?.trim() || toValue(options.sessionId)?.trim() || ''
    if (!sid || !toValue(options.enabled)) return undefined
    return chatStore.ensureACPRuntime(sid)
  }

  async function setModel(modelId: string) {
    const sid = toValue(options.sessionId)?.trim() || ''
    return chatStore.setACPRuntimeModel(modelId, sid)
  }

  return {
    runtime,
    models,
    currentModelId,
    isEnsuring,
    ensure,
    setModel,
  }
}
