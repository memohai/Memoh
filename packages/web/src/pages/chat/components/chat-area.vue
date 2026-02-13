<template>
  <div class="flex-1 flex flex-col h-full min-w-0">
    <!-- No bot selected -->
    <div
      v-if="!currentBotId"
      class="flex-1 flex items-center justify-center text-muted-foreground"
    >
      <div class="text-center">
        <p class="text-lg">{{ $t('chat.selectBot') }}</p>
        <p class="text-sm mt-1">{{ $t('chat.selectBotHint') }}</p>
      </div>
    </div>

    <template v-else>
      <!-- Bot info header -->
      <div
        v-if="currentBot"
        class="flex items-center gap-3 px-4 py-2.5 border-b"
      >
        <Avatar class="size-8 shrink-0">
          <AvatarImage
            v-if="currentBot.avatar_url"
            :src="currentBot.avatar_url"
            :alt="currentBot.display_name"
          />
          <AvatarFallback class="text-xs">
            {{ (currentBot.display_name || currentBot.id || '').slice(0, 2).toUpperCase() }}
          </AvatarFallback>
        </Avatar>
        <div class="min-w-0">
          <span class="font-medium text-sm truncate">
            {{ currentBot.display_name || currentBot.id }}
          </span>
        </div>
        <Badge
          v-if="activeChatReadOnly"
          variant="secondary"
          class="ml-auto text-xs"
        >
          {{ $t('chat.readonly') }}
        </Badge>
      </div>

      <!-- Messages -->
      <div
        ref="scrollContainer"
        class="flex-1 overflow-y-auto"
        @scroll="handleScroll"
      >
        <div class="max-w-3xl mx-auto px-4 py-6 space-y-6">
          <!-- Load older indicator -->
          <div
            v-if="loadingOlder"
            class="flex justify-center py-2"
          >
            <FontAwesomeIcon
              :icon="['fas', 'spinner']"
              class="size-3.5 animate-spin text-muted-foreground"
            />
          </div>

          <!-- Empty state -->
          <div
            v-if="messages.length === 0 && !loadingChats"
            class="flex items-center justify-center min-h-[300px]"
          >
            <p class="text-muted-foreground text-lg">
              {{ $t('chat.greeting') }}
            </p>
          </div>

          <!-- Message list -->
          <MessageItem
            v-for="msg in messages"
            :key="msg.id"
            :message="msg"
          />
        </div>
      </div>

      <!-- Input -->
      <div class="border-t p-4">
        <div class="max-w-3xl mx-auto relative">
          <Textarea
            v-model="inputText"
            class="pr-16 min-h-[60px] max-h-[200px] resize-none"
            :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
            :disabled="!currentBotId || activeChatReadOnly"
            @keydown.enter.exact="handleKeydown"
          />
          <div class="absolute right-2 bottom-2">
            <Button
              v-if="!streaming"
              size="sm"
              :disabled="!inputText.trim() || !currentBotId || activeChatReadOnly"
              @click="handleSend"
            >
              <FontAwesomeIcon :icon="['fas', 'paper-plane']" class="size-3.5" />
            </Button>
            <Button
              v-else
              size="sm"
              variant="destructive"
              @click="chatStore.abort()"
            >
              <FontAwesomeIcon :icon="['fas', 'spinner']" class="size-3.5 animate-spin" />
            </Button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { Textarea, Button, Avatar, AvatarImage, AvatarFallback, Badge } from '@memoh/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import MessageItem from './message-item.vue'

const chatStore = useChatStore()
const {
  messages,
  streaming,
  currentBotId,
  bots,
  activeChatReadOnly,
  loadingOlder,
  loadingChats,
  hasMoreOlder,
} = storeToRefs(chatStore)

const inputText = ref('')
const scrollContainer = ref<HTMLElement>()

const currentBot = computed(() =>
  bots.value.find((b) => b.id === currentBotId.value) ?? null,
)

onMounted(() => {
  void chatStore.initialize()
})

// ---- Auto-scroll ----

let userScrolledUp = false

function scrollToBottom(smooth = true) {
  nextTick(() => {
    const el = scrollContainer.value
    if (!el) return
    el.scrollTo({
      top: el.scrollHeight,
      behavior: smooth ? 'smooth' : 'instant',
    })
  })
}

function handleScroll() {
  const el = scrollContainer.value
  if (!el) return
  const distanceFromBottom = el.scrollHeight - el.clientHeight - el.scrollTop
  userScrolledUp = distanceFromBottom > 50

  // Load older messages when scrolled near top
  if (el.scrollTop < 200 && hasMoreOlder.value && !loadingOlder.value) {
    const prevHeight = el.scrollHeight
    chatStore.loadOlderMessages().then((count) => {
      if (count > 0) {
        nextTick(() => {
          el.scrollTop = el.scrollHeight - prevHeight
        })
      }
    })
  }
}

// Stream content auto-scroll
watch(
  () => {
    const last = messages.value[messages.value.length - 1]
    return last?.blocks.reduce((acc, b) => {
      if (b.type === 'text') return acc + b.content.length
      if (b.type === 'thinking') return acc + b.content.length
      return acc + 1
    }, 0) ?? 0
  },
  () => {
    if (!userScrolledUp) scrollToBottom()
  },
)

// New message auto-scroll
watch(
  () => messages.value.length,
  () => {
    userScrolledUp = false
    scrollToBottom()
  },
)

function handleKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  e.preventDefault()
  handleSend()
}

function handleSend() {
  const text = inputText.value.trim()
  if (!text || streaming.value || activeChatReadOnly.value) return
  inputText.value = ''
  chatStore.sendMessage(text)
}
</script>
