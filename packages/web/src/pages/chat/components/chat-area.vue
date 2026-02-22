<template>
  <div class="flex-1 flex flex-col h-full min-w-0">
    <!-- No bot selected -->
    <div
      v-if="!currentBotId"
      class="flex-1 flex items-center justify-center"
    >
      <div class="text-center">
        <p class="text-xl font-semibold tracking-tight text-foreground">
          {{ $t('chat.selectBot') }}
        </p>
        <p class="mt-1 text-sm text-muted-foreground">
          {{ $t('chat.selectBotHint') }}
        </p>
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
        role="log"
        aria-live="polite"
        aria-relevant="additions text"
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
            :on-open-media="galleryOpenBySrc"
          />
        </div>
      </div>

      <!-- Media gallery lightbox -->
      <MediaGalleryLightbox
        :items="galleryItems"
        :open-index="galleryOpenIndex"
        @update:open-index="gallerySetOpenIndex"
      />

      <!-- Input -->
      <div class="border-t p-4">
        <div class="max-w-3xl mx-auto">
          <!-- Pending attachment previews -->
          <div
            v-if="pendingFiles.length"
            class="flex flex-wrap gap-2 mb-2"
          >
            <div
              v-for="(file, i) in pendingFiles"
              :key="i"
              class="relative group flex items-center gap-1.5 px-2 py-1 rounded-md border bg-muted/40 text-xs"
            >
              <FontAwesomeIcon
                :icon="['fas', file.type.startsWith('image/') ? 'image' : 'file']"
                class="size-3 text-muted-foreground"
              />
              <span class="truncate max-w-[120px]">{{ file.name }}</span>
              <button
                type="button"
                class="ml-1 text-muted-foreground hover:text-foreground"
                :aria-label="`${$t('common.delete')}: ${file.name}`"
                @click="pendingFiles.splice(i, 1)"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'xmark']"
                  class="size-3"
                />
              </button>
            </div>
          </div>
          <div class="relative">
            <Textarea
              v-model="inputText"
              class="pr-24 min-h-[60px] max-h-[200px] resize-none"
              :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
              :disabled="!currentBotId || activeChatReadOnly"
              @keydown.enter.exact="handleKeydown"
              @paste="handlePaste"
            />
            <input
              ref="fileInput"
              type="file"
              class="hidden"
              aria-label="Attach files"
              multiple
              accept="image/*,audio/*,video/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.zip"
              @change="handleFileSelect"
            >
            <div class="absolute right-2 bottom-2 flex items-center gap-1">
              <Button
                v-if="!streaming"
                type="button"
                size="sm"
                variant="ghost"
                :disabled="!currentBotId || activeChatReadOnly"
                aria-label="Attach files"
                @click="fileInput?.click()"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'paperclip']"
                  class="size-3.5"
                />
              </Button>
              <Button
                v-if="!streaming"
                type="button"
                size="sm"
                :disabled="(!inputText.trim() && !pendingFiles.length) || !currentBotId || activeChatReadOnly"
                aria-label="Send message"
                @click="handleSend"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'paper-plane']"
                  class="size-3.5"
                />
              </Button>
              <Button
                v-else
                type="button"
                size="sm"
                variant="destructive"
                aria-label="Stop generating response"
                @click="chatStore.abort()"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'spinner']"
                  class="size-3.5 animate-spin"
                />
              </Button>
            </div>
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
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import { useMediaGallery } from '../composables/useMediaGallery'
import type { ChatAttachment } from '@/composables/api/useChat'

const chatStore = useChatStore()
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
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

const {
  items: galleryItems,
  openIndex: galleryOpenIndex,
  setOpenIndex: gallerySetOpenIndex,
  openBySrc: galleryOpenBySrc,
} = useMediaGallery(messages)

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

function scrollToBottom(instant = false) {
  nextTick(() => {
    const el = scrollContainer.value
    if (!el) return
    el.scrollTo({ top: el.scrollHeight, behavior: instant ? 'instant' : 'smooth' })
  })
}

function handleScroll() {
  const el = scrollContainer.value
  if (!el) return
  const distanceFromBottom = el.scrollHeight - el.clientHeight - el.scrollTop
  userScrolledUp = distanceFromBottom > 50

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

// After full load (initial / chat switch), instantly jump to bottom
watch(loadingChats, (cur, prev) => {
  if (prev && !cur) {
    userScrolledUp = false
    scrollToBottom(true)
  }
})

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

// New realtime message auto-scroll
watch(
  () => messages.value.length,
  () => {
    if (loadingChats.value) return
    userScrolledUp = false
    scrollToBottom()
  },
)

function handleKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  e.preventDefault()
  handleSend()
}

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) {
    pendingFiles.value.push(...Array.from(input.files))
  }
  input.value = ''
}

function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of items) {
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file) pendingFiles.value.push(file)
    }
  }
}

async function fileToAttachment(file: File): Promise<ChatAttachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      resolve({
        type: file.type.startsWith('image/') ? 'image' : 'file',
        base64: reader.result as string,
        mime: file.type || 'application/octet-stream',
        name: file.name,
      })
    }
    reader.onerror = () => reject(new Error('Failed to read file'))
    reader.readAsDataURL(file)
  })
}

async function handleSend() {
  const text = inputText.value.trim()
  const files = [...pendingFiles.value]
  if ((!text && !files.length) || streaming.value || activeChatReadOnly.value) return

  inputText.value = ''
  pendingFiles.value = []

  let attachments: ChatAttachment[] | undefined
  if (files.length) {
    attachments = await Promise.all(files.map(fileToAttachment))
  }

  chatStore.sendMessage(text, attachments)
}
</script>
