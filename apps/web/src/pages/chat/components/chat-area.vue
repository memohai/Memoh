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
      <!-- Session header -->
      <div class="border-b px-4 py-2 flex items-center justify-between min-h-12">
        <div class="flex items-center gap-2 min-w-0">
          <h2 class="text-sm font-medium truncate">
            {{ activeSession?.title || $t('chat.untitledSession') }}
          </h2>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            :aria-label="$t('chat.newSession')"
            @click="chatStore.createNewSession()"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="size-3.5"
            />
          </Button>
        </div>
      </div>

      <!-- Messages -->
      <section class="flex-1 relative w-full">
        <section class="absolute inset-0">
          <ScrollArea
            ref="scrollContainer"
            class="h-full"
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
          </ScrollArea>
        </section>
      </section>


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
              <span class="truncate max-w-30">{{ file.name }}</span>
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

          <section>
            <InputGroup>
              <InputGroupTextarea
                v-model="inputText"
                class=" max-h-15 resize-none break-all!"
                :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
                :disabled="!currentBotId || activeChatReadOnly"
                style="scrollbar-width: none;"
                @keydown.enter.exact="handleKeydown"
                @paste="handlePaste"
              />
              <InputGroupAddon
                align="block-end"
                class="justify-end"
              >
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
                  type="button"
                  size="sm"
                  variant="ghost"
                  :disabled="!currentBotId"
                  :aria-label="$t('chat.files')"
                  @click="fileManagerOpen = true"
                >
                  <FontAwesomeIcon
                    :icon="['fas', 'folder-open']"
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
              </InputGroupAddon>
            </InputGroup>
          </section>
        </div>
      </div>
    </template>

    <!-- File manager sheet -->
    <Sheet v-model:open="fileManagerOpen">
      <SheetContent
        side="right"
        class="sm:max-w-2xl w-full p-0 flex flex-col"
      >
        <SheetHeader class="px-4 pt-4 pb-0">
          <SheetTitle>{{ $t('chat.files') }}</SheetTitle>
          <SheetDescription class="sr-only">
            {{ $t('chat.files') }}
          </SheetDescription>
        </SheetHeader>
        <div class="flex-1 min-h-0 relative">
          <FileManager
            v-if="currentBotId"
            ref="fileManagerRef"
            :bot-id="currentBotId"
            :sync-url="false"
          />
        </div>
      </SheetContent>
    </Sheet>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, provide, useTemplateRef, watchEffect} from 'vue'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@memoh/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import MessageItem from './message-item.vue'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import FileManager from '@/components/file-manager/index.vue'
import { useMediaGallery } from '../composables/useMediaGallery'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import type { ChatAttachment } from '@/composables/api/useChat'
import { useScroll, useElementBounding } from '@vueuse/core'

const chatStore = useChatStore()
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
const fileManagerOpen = ref(false)
const fileManagerRef = ref<InstanceType<typeof FileManager> | null>(null)

const FILE_MANAGER_ROOT = '/data'

function normalizeFileManagerPath(path: string): string {
  const trimmedPath = path.trim()
  if (!trimmedPath) return FILE_MANAGER_ROOT
  if (trimmedPath === FILE_MANAGER_ROOT || trimmedPath.startsWith(`${FILE_MANAGER_ROOT}/`)) {
    return trimmedPath
  }
  if (trimmedPath === '/') return FILE_MANAGER_ROOT
  if (trimmedPath.startsWith('/')) {
    return `${FILE_MANAGER_ROOT}${trimmedPath}`
  }
  return `${FILE_MANAGER_ROOT}/${trimmedPath}`
}

provide(openInFileManagerKey, (path: string, isDir = false) => {
  const normalizedPath = normalizeFileManagerPath(path)
  fileManagerOpen.value = true
  nextTick(() => {
    if (!fileManagerRef.value) return
    if (isDir) {
      fileManagerRef.value.navigateTo(normalizedPath)
    } else {
      fileManagerRef.value.openFileByPath(normalizedPath)
    }
  })
})
const {
  messages,
  streaming,
  currentBotId,
  activeSession,
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


onMounted(async () => {
  try {
    if (chatStore.currentBotId || chatStore.sessionId) {
      await chatStore.initialize()
    }
  } finally {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        isInstant.value = true
      })
    })
  }
})

const elNode = useTemplateRef('scrollContainer')
const descEl = computed(() => elNode.value?.$el?.children[0]?.children[0])
const scrollEl = computed(() => descEl.value?.parentNode)
const isAutoScroll = ref(true)
const isInstant=ref(false)
const { y, directions, arrivedState } = useScroll(scrollEl, { behavior: computed(() => isAutoScroll.value&&isInstant.value ? 'smooth' : 'instant') })
const { height,bottom } = useElementBounding(descEl)


watchEffect(() => {
  if (directions.top) {
    isAutoScroll.value = false
  }
  if (arrivedState.bottom) {
    isAutoScroll.value = true
  }
})

watchEffect(() => {  
  if (isAutoScroll.value) {
    y.value = height.value
  }
})

let Throttle = true

watchEffect(() => {
  if (directions.top && arrivedState.top && Throttle && hasMoreOlder.value && !loadingOlder.value) {
    const prev=bottom.value
    Throttle = false    
    chatStore.loadOlderMessages().then((count) => {
      setTimeout(() => {
        if (count > 0) {               
          y.value = height.value-prev
          Throttle = true        
        }    
      })
    })
  }
})

function handleKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  e.preventDefault()
  handleSend()
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
  isAutoScroll.value=true
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
