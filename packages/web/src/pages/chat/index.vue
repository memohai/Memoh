<template>
  <section class="h-[calc(100vh-calc(var(--spacing)*20))]  max-w-187 gap-8 w-full *:w-full m-auto flex flex-col">
    <section class="flex-1 h-0">
      <ScrollArea
        ref="chat-container"
        class="max-h-full h-full w-full rounded-md border p-4 **:focus-visible:ring-0! "
      >
        <ChatList />
      </ScrollArea>
    </section>
    <section class="flex-none relative">
      <Textarea
        v-model="curInputSay"
        class="pb-16 pt-4"
        :placeholder="$t('prompt.enter', { msg: $t('desc.question') })"
      />
      <section class="absolute bottom-0 h-14 px-2 inset-x-0 flex items-center">
        <Button
          variant="default"
          class="ml-auto"
          @click="send"
        >
          <template v-if="!loading">
            {{ $t('chat.send') }}
            <svg-icon
              type="mdi"
              :path="mdiSendOutline"
            />
          </template>
          <img
            v-else
            src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIyNCIgaGVpZ2h0PSIyNCIgdmlld0JveD0iMCAwIDI0IDI0Ij48Y2lyY2xlIGN4PSI0IiBjeT0iMTIiIHI9IjEuNSIgZmlsbD0iI2ZmZiI+PGFuaW1hdGUgYXR0cmlidXRlTmFtZT0iciIgZHVyPSIwLjc1cyIgcmVwZWF0Q291bnQ9ImluZGVmaW5pdGUiIHZhbHVlcz0iMS41OzM7MS41Ii8+PC9jaXJjbGU+PGNpcmNsZSBjeD0iMTIiIGN5PSIxMiIgcj0iMyIgZmlsbD0iI2ZmZiI+PGFuaW1hdGUgYXR0cmlidXRlTmFtZT0iciIgZHVyPSIwLjc1cyIgcmVwZWF0Q291bnQ9ImluZGVmaW5pdGUiIHZhbHVlcz0iMzsxLjU7MyIvPjwvY2lyY2xlPjxjaXJjbGUgY3g9IjIwIiBjeT0iMTIiIHI9IjEuNSIgZmlsbD0iI2ZmZiI+PGFuaW1hdGUgYXR0cmlidXRlTmFtZT0iciIgZHVyPSIwLjc1cyIgcmVwZWF0Q291bnQ9ImluZGVmaW5pdGUiIHZhbHVlcz0iMS41OzM7MS41Ii8+PC9jaXJjbGU+PC9zdmc+"
            alt="loading"
          >
        </Button>
      </section>
    </section>
  </section>
</template>

<script setup lang="ts">
import {
  ScrollArea,
  Textarea,
  Button
} from '@memoh/ui'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiSendOutline } from '@mdi/js'
import ChatList from '@/components/ChatList/index.vue'
import { provide, ref } from 'vue'
import { useChatList } from '@/store/ChatList'
import {storeToRefs} from 'pinia'

const chatSay = ref('')
const curInputSay = ref('')

const {loading}=storeToRefs(useChatList())
provide('chatSay', chatSay)


const send = () => {
  if (loading.value === false) {
    chatSay.value = curInputSay.value
    curInputSay.value = ''
  }
}
</script>