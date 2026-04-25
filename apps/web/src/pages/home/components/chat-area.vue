<template>
  <div class="flex-1 flex h-full min-w-0">
    <div class="flex-1 flex flex-col h-full min-w-0 relative">
      <!-- Desktop floating title bar (overlays the chat area, fades into messages
           via a bottom shadow gradient — pointer-events:none lets scroll/clicks
           fall through). -->
      <header
        v-if="topInset && currentBotId"
        class="pointer-events-none absolute top-0 left-0 right-0 z-10 select-none"
      >
        <div class="h-10 flex items-center justify-center bg-background px-12">
          <span class="text-xs font-medium text-foreground truncate">
            {{ desktopTitle }}
          </span>
        </div>
        <div class="h-5 bg-linear-to-b from-background to-transparent" />
      </header>

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
        <!-- Messages -->
        <section class="flex-1 relative w-full px-3 sm:px-5 lg:px-8">
          <section class="absolute inset-0">
            <ScrollArea
              ref="scrollContainer"
              class="h-full"
            >
              <div
                class="w-full max-w-4xl mx-auto px-10 pb-6 space-y-6"
                :class="topInset ? 'pt-15' : 'pt-6'"
              >
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
                  <p
                    v-if="activeSession?.type === 'subagent'"
                    class="text-muted-foreground text-xs"
                  >
                    {{ $t('chat.emptySubagent') }}
                  </p>
                  <p
                    v-else-if="activeSession?.type === 'heartbeat' || activeSession?.type === 'schedule'"
                    class="text-muted-foreground text-xs"
                  >
                    {{ $t('chat.emptySystemSession') }}
                  </p>
                  <p
                    v-else
                    class="text-muted-foreground text-xs"
                  >
                    {{ $t('chat.greeting') }}
                  </p>
                </div>

                <!-- Message list -->
                <MessageItem
                  v-for="msg in messages"
                  :key="msg.id"
                  :message="msg"
                  :session-type="activeSession?.type"
                  :bot-id="currentBotId"
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

        <!-- Input (hidden for read-only sessions) -->
        <div
          v-if="!activeChatReadOnly"
          class="px-3 sm:px-5 lg:px-8 py-2.5"
        >
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

    <!-- Right sidebar panel -->
    <div
      v-if="activeRightTab"
      class="flex shrink-0 h-full relative"
      :style="{ width: `${rightPanelWidth}px` }"
    >
      <!-- Resize handle -->
      <div
        class="absolute top-0 left-0 w-1 h-full cursor-col-resize z-10 group"
        @mousedown="onPanelResizeStart"
      >
        <div
          class="w-full h-full transition-colors group-hover:bg-primary/20"
          :class="{ 'bg-primary/30': isPanelResizing }"
        />
      </div>

      <div class="flex flex-col h-full flex-1 min-w-0 overflow-hidden border-l border-border bg-sidebar">
        <!-- Panel tab bar -->
        <div class="flex items-center h-12 shrink-0 border-b border-border">
          <button
            v-for="tab in rightTabs"
            :key="tab.id"
            class="flex items-center gap-1.5 px-4 h-full text-xs transition-colors border-b-2"
            :class="activeRightTab === tab.id
              ? 'border-foreground text-foreground font-medium'
              : 'border-transparent text-muted-foreground hover:text-foreground'"
            @click="activeRightTab = tab.id"
          >
            <component
              :is="tab.icon"
              class="size-4"
            />
            {{ tab.label }}
          </button>
        </div>

        <!-- Panel content -->
        <div class="flex-1 min-h-0 relative">
          <div
            v-show="activeRightTab === 'terminal'"
            class="absolute inset-0"
          >
            <TerminalComponent
              v-if="currentBotId"
              :bot-id="currentBotId"
              :visible="activeRightTab === 'terminal'"
            />
          </div>
          <div
            v-show="activeRightTab === 'files'"
            class="absolute inset-0"
          >
            <FileManager
              v-if="currentBotId"
              ref="fileManagerRef"
              :bot-id="currentBotId"
              :sync-url="false"
              preview-layout="bottom"
            />
          </div>
          <div
            v-if="activeRightTab === 'status'"
            class="absolute inset-0"
          >
            <SessionInfoPanel
              :visible="activeRightTab === 'status'"
              :override-model-id="overrideModelId"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Activity Bar -->
    <div class="flex flex-col items-center w-10 shrink-0 h-full border-l border-border bg-sidebar">
      <div class="flex flex-col items-center gap-3 pt-4">
        <button
          v-for="tab in rightTabs"
          :key="tab.id"
          class="flex items-center justify-center size-7 rounded-md transition-colors"
          :class="activeRightTab === tab.id
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'"
          :title="tab.label"
          @click="toggleRightPanel(tab.id)"
        >
          <component
            :is="tab.icon"
            class="size-4"
          />
        </button>
      </div>
      <div class="mt-auto pb-4">
        <button
          class="flex items-center justify-center size-7 rounded-md text-destructive/60 hover:bg-destructive/10 hover:text-destructive transition-colors"
          :title="$t('chat.deleteSession')"
          :disabled="!sessionId"
          @click="confirmDeleteSession"
        >
          <Trash2 class="size-4" />
        </button>
      </div>
    </div>

    <!-- Delete session confirmation dialog -->
    <Dialog v-model:open="deleteSessionDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('chat.deleteSession') }}</DialogTitle>
          <DialogDescription>{{ $t('chat.deleteSessionConfirm') }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="deleteSessionLoading"
            @click="deleteSessionDialogOpen = false"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deleteSessionLoading"
            @click="handleDeleteSession"
          >
            <LoaderCircle
              v-if="deleteSessionLoading"
              class="mr-1 size-3 animate-spin"
            />
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, nextTick, onMounted, onBeforeUnmount, provide, useTemplateRef, watchEffect, watch, type Component } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { LoaderCircle, Image as ImageIcon, File as FileIcon, X, Paperclip, FolderOpen, Send, ChevronDown, Lightbulb, TerminalSquare, BarChart3, Trash2 } from 'lucide-vue-next'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Popover, PopoverContent, PopoverTrigger, Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import MessageItem from './message-item.vue'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import FileManager from '@/components/file-manager/index.vue'
import TerminalComponent from '@/components/terminal/index.vue'
import ModelOptions from '@/pages/bots/components/model-options.vue'
import ReasoningEffortSelect from '@/pages/bots/components/reasoning-effort-select.vue'
import SessionInfoPanel from './session-info-panel.vue'
import { EFFORT_LABELS, EFFORT_OPACITY } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import type { ChatAttachment } from '@/composables/api/useChat'
import { DesktopShellKey } from '@/lib/desktop-shell'
import { useScroll, useElementBounding } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const chatStore = useChatStore()
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
const fileManagerRef = ref<InstanceType<typeof FileManager> | null>(null)
const modelPopoverOpen = ref(false)
const reasoningPopoverOpen = ref(false)

// ---- Right sidebar panel ----

type RightTabId = 'terminal' | 'files' | 'status'

interface RightTab {
  id: RightTabId
  label: string
  icon: Component
}

const rightTabs = computed<RightTab[]>(() => [
  { id: 'terminal', label: 'Terminal', icon: TerminalSquare },
  { id: 'files', label: t('chat.files'), icon: FolderOpen },
  { id: 'status', label: 'Status', icon: BarChart3 },
])

const activeRightTab = ref<RightTabId | null>(null)

function toggleRightPanel(tabId: RightTabId) {
  activeRightTab.value = activeRightTab.value === tabId ? null : tabId
}

function openRightPanel(tabId: RightTabId) {
  activeRightTab.value = tabId
}

watch(activeRightTab, () => {
  nextTick(() => window.dispatchEvent(new Event('resize')))
})

const PANEL_MIN_WIDTH = 320
const PANEL_MAX_WIDTH = 800
const PANEL_DEFAULT_WIDTH = 504

const rightPanelWidth = useLocalStorage('chat-right-panel-width', PANEL_DEFAULT_WIDTH)
const isPanelResizing = ref(false)

function onPanelResizeStart(e: MouseEvent) {
  e.preventDefault()
  isPanelResizing.value = true
  const startX = e.clientX
  const startWidth = rightPanelWidth.value

  function onMouseMove(ev: MouseEvent) {
    const delta = startX - ev.clientX
    rightPanelWidth.value = Math.min(PANEL_MAX_WIDTH, Math.max(PANEL_MIN_WIDTH, startWidth + delta))
  }

  function onMouseUp() {
    isPanelResizing.value = false
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

// ---- Delete session ----

const deleteSessionDialogOpen = ref(false)
const deleteSessionLoading = ref(false)

function confirmDeleteSession() {
  if (!sessionId.value) return
  deleteSessionDialogOpen.value = true
}

async function handleDeleteSession() {
  const sid = sessionId.value
  if (!sid || deleteSessionLoading.value) return
  deleteSessionLoading.value = true
  try {
    await chatStore.removeSession(sid)
    deleteSessionDialogOpen.value = false
  } finally {
    deleteSessionLoading.value = false
  }
}

// ---- File manager provider (for tool call components) ----

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
  openRightPanel('files')
  nextTick(() => {
    if (!fileManagerRef.value) return
    if (isDir) {
      fileManagerRef.value.navigateTo(normalizedPath)
    } else {
      fileManagerRef.value.openFileByPath(normalizedPath)
    }
  })
})

// ---- Chat store refs ----

const {
  messages,
  streaming,
  currentBotId,
  sessionId,
  activeSession,
  activeChatReadOnly,
  loadingOlder,
  loadingChats,
  hasMoreOlder,
  overrideModelId,
  overrideReasoningEffort,
  bots,
} = storeToRefs(chatStore)

const topInset = inject(DesktopShellKey, false)

const desktopTitle = computed(() => {
  const sessionTitle = (activeSession.value?.title ?? '').trim()
  const bot = bots.value.find((b) => b.id === currentBotId.value)
  const botName = (bot?.display_name ?? bot?.id ?? '').trim()
  if (sessionTitle && botName) return `${sessionTitle} - ${botName}`
  return sessionTitle || botName
})

// ---- Model / provider queries ----

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

// ---- Media gallery ----

const {
  items: galleryItems,
  openIndex: galleryOpenIndex,
  setOpenIndex: gallerySetOpenIndex,
  openBySrc: galleryOpenBySrc,
} = useMediaGallery(messages)

// ---- Input & scroll ----

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
const isInstant = ref(false)
const { y, directions, arrivedState } = useScroll(scrollEl, { behavior: computed(() => isAutoScroll.value && isInstant.value ? 'smooth' : 'instant') })
const { height, bottom } = useElementBounding(descEl)

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
    const prev = bottom.value
    Throttle = false
    chatStore.loadOlderMessages().then((count) => {
      setTimeout(() => {
        if (count > 0) {
          y.value = height.value - prev
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
  isAutoScroll.value = true
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
