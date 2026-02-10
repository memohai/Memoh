<template>
  <div
    ref="displayContainer"
    class="flex flex-col gap-4"
  >
    <template
      v-for="chatItem in chatList"
      :key="chatItem.id"
    >
      <UserChat
        v-if="chatItem.action === 'user'"
        :user-say="chatItem"
      />
      <RobotChat
        v-if="chatItem.action === 'robot'"
        :robot-say="chatItem"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import UserChat from './user-chat/index.vue'
import RobotChat from './robot-chat/index.vue'
import { inject, ref, watch } from 'vue'
import { useChatList } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useAutoScroll } from '@/composables/useAutoScroll'

const { chatList, sendMessage } = useChatList()
const { loading } = storeToRefs(useChatList())

// ---- 消息发送 ----
const chatSay = inject('chatSay', ref(''))

watch(chatSay, async () => {
  if (chatSay.value) {
    const text = chatSay.value
    chatSay.value = ''
    try {
      await sendMessage(text)
    } catch {
      // ignore
    }
  }
}, { immediate: true })

// ---- 自动滚动 ----
const displayContainer = ref<HTMLElement>()
useAutoScroll(displayContainer, loading)
</script>
