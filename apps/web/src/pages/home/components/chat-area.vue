<template>
  <div class="flex-1 flex h-full min-w-0">
    <div class="flex-1 flex flex-col h-full min-w-0">
      <!-- No bot selected -->
      <div
        v-if="!currentBotId"
        class="flex-1 flex items-center justify-center"
      >
        <div class="text-center">
          <p class="text-xs font-medium text-foreground">
            {{ $t('chat.selectBot') }}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ $t('chat.selectBotHint') }}
          </p>
        </div>
      </div>

      <template v-else>
        <!-- Session header -->
        <!-- <div class="border-b px-4 py-2 flex items-center justify-between min-h-12">
        <div class="flex items-center gap-2 min-w-0">
          <h2 class="text-xs font-medium truncate">
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
      </div> -->

        <!-- Messages -->
        <section class="flex-1 relative w-full px-3 sm:px-5 lg:px-8">
          <section class="absolute inset-0">
            <ScrollArea
              ref="scrollContainer"
              class="h-full"
            >
              <div class="w-full max-w-4xl mx-auto px-10 py-6 space-y-6">
                <!-- Load older indicator -->
                <div
                  v-if="loadingOlder"
                  class="flex justify-center py-2"
                >
                  <LoaderCircle
                    class="size-3.5 animate-spin text-muted-foreground"
                  />
                </div>

                <!-- Empty state -->
                <div
                  v-if="messages.length === 0 && !loadingChats"
                  class="flex items-center justify-center min-h-[300px]"
                >
                  <p class="text-muted-foreground text-xs">
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
        <div class="px-3 sm:px-5 lg:px-8 py-2.5">
          <div class="w-full max-w-4xl mx-auto">
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
                <component
                  :is="file.type.startsWith('image/') ? ImageIcon : FileIcon"
                  class="size-3 text-muted-foreground"
                />
                <span class="truncate max-w-30">{{ file.name }}</span>
                <button
                  type="button"
                  class="ml-1 text-muted-foreground hover:text-foreground"
                  :aria-label="`${$t('common.delete')}: ${file.name}`"
                  @click="pendingFiles.splice(i, 1)"
                >
                  <X
                    class="size-3"
                  />
                </button>
              </div>
            </div>

            <input
              ref="fileInput"
              type="file"
              multiple
              class="hidden"
              @change="handleFileInputChange"
            >
            <section>
              <InputGroup class="bg-transparent overflow-hidden shadow-none! ring-0! border-border!">
                <InputGroupTextarea
                  v-model="inputText"
                  class="min-h-14 max-h-14 text-xs resize-none break-all!"                
                  :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
                  :disabled="!currentBotId || activeChatReadOnly"
                  style="scrollbar-width: none;"
                  @keydown.enter.exact="handleKeydown"
                  @paste="handlePaste"
                />
                <InputGroupAddon
                  align="block-end"
                  class="items-center py-1.5"
                >
                  <!-- Model override selector -->
                  <Popover v-model:open="modelPopoverOpen">
                    <PopoverTrigger as-child>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        :disabled="!currentBotId || activeChatReadOnly"
                        class="gap-0.5 text-muted-foreground max-w-40"
                      >
                        <span class="truncate text-[11px]">{{ selectedModelLabel }}</span>
                        <ChevronDown class="size-3 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent
                      class="w-96 p-0"
                      align="start"
                    >
                      <ModelOptions
                        v-model="overrideModelId"
                        :models="models"
                        :providers="providers"
                        model-type="chat"
                        :open="modelPopoverOpen"
                        @update:model-value="onModelSelected"
                      />
                    </PopoverContent>
                  </Popover>

                  <!-- Reasoning effort selector -->
                  <Popover v-model:open="reasoningPopoverOpen">
                    <PopoverTrigger as-child>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        :disabled="!currentBotId || activeChatReadOnly || !activeModelSupportsReasoning"
                        class="gap-0.5 text-muted-foreground"
                      >
                        <Lightbulb
                          class="size-3.5 shrink-0"
                          :style="{ opacity: reasoningTriggerOpacity }"
                        />
                        <span class="text-[11px]">{{ selectedReasoningLabel }}</span>
                        <ChevronDown class="size-3 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent
                      class="w-40 p-0"
                      align="start"
                    >
                      <ReasoningEffortSelect
                        v-model="overrideReasoningEffort"
                        :efforts="availableReasoningEfforts"
                        @update:model-value="onReasoningSelected"
                      />
                    </PopoverContent>
                  </Popover>

                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    :disabled="!currentBotId || activeChatReadOnly || streaming"
                    aria-label="Attach files"
                    @click="fileInput?.click()"
                  >
                    <Paperclip
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
                    <FolderOpen
                      class="size-3.5"
                    />
                  </Button>
                  <Button
                    v-if="!streaming"
                    type="button"
                    size="icon"
                    :disabled="(!inputText.trim() && !pendingFiles.length) || !currentBotId || activeChatReadOnly"
                    aria-label="Send message"
                    class="ml-auto size-7 rounded-full bg-[#8B56E3] text-white"
                    @click="handleSend"
                  >
                    <Send
                      class="size-3"
                    />
                  </Button>
                  <Button
                    v-else
                    type="button"
                    size="icon"
                    variant="destructive"
                    class="ml-auto size-7 rounded-full"
                    aria-label="Stop generating response"
                    @click="chatStore.abort()"
                  >
                    <LoaderCircle
                      class="size-3.5 animate-spin"
                    />
                  </Button>
                </InputGroupAddon>
              </InputGroup>
            </section>
          </div>
        </div>
      </template>
    </div>

    <!-- File manager panel -->
    <div
      v-if="fileManagerOpen"
      class="flex shrink-0 h-full relative"
      :style="{ width: `${fileManagerWidth}px` }"
    >
      <div
        class="absolute top-0 left-0 w-1 h-full cursor-col-resize z-10 group"
        @mousedown="onFmResizeStart"
      >
        <div
          class="w-full h-full transition-colors group-hover:bg-primary/20"
          :class="{ 'bg-primary/30': isFmResizing }"
        />
      </div>

      <div class="flex flex-col h-full flex-1 min-w-0 border-l border-border bg-sidebar">
        <div class="flex items-center justify-between px-4 h-12 shrink-0">
          <span class="text-sm font-medium text-foreground">{{ $t('chat.files') }}</span>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            class="size-6"
            @click="fileManagerOpen = false"
          >
            <X class="size-3.5" />
          </Button>
        </div>
        <div class="flex-1 min-h-0 relative">
          <FileManager
            v-if="currentBotId"
            ref="fileManagerRef"
            :bot-id="currentBotId"
            :sync-url="false"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, provide, useTemplateRef, watchEffect, watch } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { LoaderCircle, Image as ImageIcon, File as FileIcon, X, Paperclip, FolderOpen, Send, ChevronDown, Lightbulb } from 'lucide-vue-next'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Popover, PopoverContent, PopoverTrigger } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import MessageItem from './message-item.vue'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import FileManager from '@/components/file-manager/index.vue'
import ModelOptions from '@/pages/bots/components/model-options.vue'
import ReasoningEffortSelect from '@/pages/bots/components/reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import type { ChatAttachment } from '@/composables/api/useChat'
import { useScroll, useElementBounding } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const chatStore = useChatStore()
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
const fileManagerOpen = ref(false)
const fileManagerRef = ref<InstanceType<typeof FileManager> | null>(null)
const modelPopoverOpen = ref(false)
const reasoningPopoverOpen = ref(false)

const FM_MIN_WIDTH = 320
const FM_MAX_WIDTH = 800
const FM_DEFAULT_WIDTH = 520

const fileManagerWidth = useLocalStorage('file-manager-panel-width', FM_DEFAULT_WIDTH)
const isFmResizing = ref(false)

function onFmResizeStart(e: MouseEvent) {
  e.preventDefault()
  isFmResizing.value = true
  const startX = e.clientX
  const startWidth = fileManagerWidth.value

  function onMouseMove(ev: MouseEvent) {
    const delta = startX - ev.clientX
    fileManagerWidth.value = Math.min(FM_MAX_WIDTH, Math.max(FM_MIN_WIDTH, startWidth + delta))
  }

  function onMouseUp() {
    isFmResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }

  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

onBeforeUnmount(() => {
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})

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
  activeChatReadOnly,
  loadingOlder,
  loadingChats,
  hasMoreOlder,
  overrideModelId,
  overrideReasoningEffort,
} = storeToRefs(chatStore)

const { data: modelData } = useQuery({
  key: ['all-models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['all-providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: botSettings } = useQuery({
  key: () => ['bot-settings', currentBotId.value],
  query: async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const { data } = await (getBotsByBotIdSettings as any)({
      path: { bot_id: currentBotId.value! },
      throwOnError: true,
    })
    return data as import('@memohai/sdk').SettingsSettings | undefined
  },
  enabled: () => !!currentBotId.value,
})

const models = computed<ModelsGetResponse[]>(() => modelData.value ?? [])
const providers = computed<ProvidersGetResponse[]>(() => providerData.value ?? [])

const activeModel = computed(() => {
  const id = overrideModelId.value || botSettings.value?.chat_model_id || ''
  return models.value.find((m) => m.id === id)
})

const activeModelSupportsReasoning = computed(() =>
  !!activeModel.value?.config?.compatibilities?.includes('reasoning'),
)

const availableReasoningEfforts = computed(() => {
  const efforts = ((activeModel.value?.config as { reasoning_efforts?: string[] } | undefined)?.reasoning_efforts ?? [])
    .filter((e) => ['none', 'low', 'medium', 'high', 'xhigh'].includes(e))
  return efforts.length > 0 ? efforts : ['low', 'medium', 'high']
})

const selectedModelLabel = computed(() => {
  const m = models.value.find((m) => m.id === overrideModelId.value)
  return m?.name || m?.model_id || t('chat.modelDefault')
})

const selectedReasoningLabel = computed(() => {
  const v = overrideReasoningEffort.value
  if (v === 'off') return t('chat.reasoningOff')
  return t(EFFORT_LABELS[v] ?? 'chat.modelDefault')
})

const reasoningTriggerOpacity = computed(() =>
  EFFORT_OPACITY[overrideReasoningEffort.value] ?? 0.5,
)

function initFromBotSettings() {
  if (!botSettings.value) return
  if (!overrideModelId.value) {
    overrideModelId.value = botSettings.value.chat_model_id ?? ''
  }
  if (!overrideReasoningEffort.value) {
    if (botSettings.value.reasoning_enabled && botSettings.value.reasoning_effort) {
      overrideReasoningEffort.value = botSettings.value.reasoning_effort
    } else {
      overrideReasoningEffort.value = 'off'
    }
  }
}

watch(botSettings, () => initFromBotSettings(), { immediate: true })

watch(currentBotId, () => {
  overrideModelId.value = ''
  overrideReasoningEffort.value = ''
})

function onModelSelected() {
  modelPopoverOpen.value = false
  if (!activeModelSupportsReasoning.value) {
    overrideReasoningEffort.value = 'off'
  }
}

function onReasoningSelected() {
  reasoningPopoverOpen.value = false
}

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
  if (e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  handleSend()
}


function handleFileInputChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) {
    for (const file of Array.from(input.files)) {
      pendingFiles.value.push(file)
    }
  }
  input.value = ''
}

function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of Array.from(items)) {
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
