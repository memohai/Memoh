<template>
  <section class="h-[calc(100vh-calc(var(--spacing)*20))] max-w-187 gap-8 w-full *:w-full m-auto flex flex-col">
    <!-- 聊天区域 -->
    <section class="flex-1 h-0 [&:has(p)]:block! [&:has(p)+section_.logo-title]:hidden [&:has(p)+section]:mt-0! hidden">
      <ScrollArea class="max-h-full h-full w-full rounded-md p-4 **:focus-visible:ring-0!">
        <ChatList />
      </ScrollArea>
    </section>

    <!-- 输入区域 -->
    <section class="flex-none relative m-auto">
      <section class="mb-20 logo-title">
        <h4
          class="scroll-m-20 text-3xl font-semibold tracking-tight text-center"
          style="font-family: 'Source Han Serif CN', 'Noto Serif SC', 'STSong', 'SimSun', serif;"
        >
          <TextGenerateEffect :words="$t('chat.greeting')" />
        </h4>
      </section>

      <Textarea
        v-model="curInputSay"
        class="pb-16 pt-4"
        :placeholder="$t('chat.inputPlaceholder')"
      />

      <section class="absolute bottom-0 h-14 px-2 inset-x-0 flex items-center">
        <Button
          variant="default"
          class="ml-auto"
          @click="send"
        >
          <template v-if="!loading">
            {{ $t('chat.send') }}

            <FontAwesomeIcon :icon="['fas', 'paper-plane']" />
          </template>
          <LoadingDots v-else />
        </Button>
      </section>
    </section>
  </section>
</template>

<script setup lang="ts">
import {
  ScrollArea,
  Textarea,
  Button,
} from '@memoh/ui'
import ChatList from '@/components/chat-list/index.vue'
import LoadingDots from '@/components/loading-dots/index.vue'
import { provide, ref } from 'vue'
import { useChatList } from '@/store/chat-list'
import { storeToRefs } from 'pinia'

const chatSay = ref('')
const curInputSay = ref('')
const { loading } = storeToRefs(useChatList())

provide('chatSay', chatSay)

function send() {
  if (!loading.value) {
    chatSay.value = curInputSay.value
    curInputSay.value = ''
  }
}
</script>
