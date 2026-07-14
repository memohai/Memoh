import { computed, hasInjectionContext, inject, provide, type InjectionKey, type Ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore, type ChatViewTarget } from '@/store/chat-list'

const chatViewTargetKey: InjectionKey<Ref<ChatViewTarget>> = Symbol('chat-view-target')

export function provideChatViewTarget(target: Ref<ChatViewTarget>) {
  provide(chatViewTargetKey, target)
}

export function useChatViewTarget(): Ref<ChatViewTarget> {
  const provided = hasInjectionContext() ? inject(chatViewTargetKey, null) : null
  if (provided) return provided

  const chatStore = useChatStore()
  const { currentBotId, sessionId } = storeToRefs(chatStore)
  return computed(() => ({
    botId: currentBotId.value?.trim() ?? '',
    sessionId: sessionId.value?.trim() || null,
    viewId: 'chat',
  }))
}
