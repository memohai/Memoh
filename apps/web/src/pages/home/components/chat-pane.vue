<template>
  <div class="flex-1 flex flex-col h-full min-w-0 relative">
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
      <section class="flex-1 relative w-full px-3 sm:px-5 lg:px-8">
        <section class="absolute inset-0">
          <ScrollArea
          
            ref="scrollContainer"
            :class="`${transitionScroll?'opacity-100':'opacity-0'} h-full`"
          >
            <div            
              class="w-full max-w-4xl mx-auto px-10 pt-6 pb-6 space-y-6"
            >
              <div
                ref="loadMoreSentinel"
                aria-hidden="true"
                class="h-px w-full"
              />
              <div
                v-if="loadingOlder"
                class="flex justify-center py-2"
              >
                <LoaderCircle class="size-3.5 animate-spin text-muted-foreground" />
              </div>

              <div
                v-if="messages.length === 0 && !loadingChats"
                class="flex items-center justify-center min-h-75"
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

              <div
                v-for="msg in messages"
                :key="msg.id"
                :data-message-id="msg.id"
                :data-external-message-id="(msg.role === 'user' || msg.role === 'assistant') ? msg.externalMessageId : undefined"
                class="rounded-2xl transition-[background-color,box-shadow] duration-500"
                :class="highlightedMessageId === msg.id ? 'bg-primary/10 ring-2 ring-primary/25' : ''"
                :data-anchor="msg.id"
              >
                <MessageItem
                  :message="msg"
                  :session-type="activeSession?.type"
                  :bot-id="currentBotId"
                  :on-open-media="galleryOpenBySrc"
                  :on-reply-click="handleReplyJump"
                  :root-el="scrollEl"
                  :is-scrolling="isScrolling"
                  @active="isActiveEl"
                />
              </div>
            </div>
          </ScrollArea>
        </section>
      </section>

      <MediaGalleryLightbox
        :items="galleryItems"
        :open-index="galleryOpenIndex"
        @update:open-index="gallerySetOpenIndex"
      />

      <div
        v-if="!activeChatReadOnly"
        class="px-3 sm:px-5 lg:px-8 py-2.5"
      >
        <div class="w-full max-w-4xl mx-auto">
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
                <X class="size-3" />
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
            <div
              v-if="composerError"
              class="mb-2 flex items-start gap-2 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-xs text-destructive"
            >
              <CircleAlert class="mt-0.5 size-3.5 shrink-0" />
              <span class="min-w-0 break-words">{{ composerError }}</span>
            </div>
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
                <Popover v-model:open="modelPopoverOpen">
                  <PopoverTrigger as-child>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      :disabled="!currentBotId || activeChatReadOnly"
                      class="gap-0.5 text-muted-foreground max-w-40"
                      :title="selectedModelLabel"
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
                  <Paperclip class="size-3.5" />
                </Button>

                <SessionInfoRing
                  class="ml-auto"
                  :override-model-id="overrideModelId"
                />

                <Button
                  v-if="!streaming"
                  type="button"
                  size="icon"
                  :disabled="(!inputText.trim() && !pendingFiles.length) || !currentBotId || activeChatReadOnly"
                  aria-label="Send message"
                  class="size-7 rounded-full bg-primary text-primary-foreground"
                  @click="handleSend"
                >
                  <Send class="size-3" />
                </Button>
                <Button
                  v-else
                  type="button"
                  size="icon"
                  variant="destructive"
                  class="size-7 rounded-full"
                  aria-label="Stop generating response"
                  @click="chatStore.abort()"
                >
                  <LoaderCircle class="size-3.5 animate-spin" />
                </Button>
              </InputGroupAddon>
            </InputGroup>
          </section>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, useTemplateRef, watchEffect, watch, nextTick, onActivated, onDeactivated } from 'vue'
import { LoaderCircle, Image as ImageIcon, File as FileIcon, X, Paperclip, Send, ChevronDown, Lightbulb, CircleAlert } from 'lucide-vue-next'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Popover, PopoverContent, PopoverTrigger } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useScroll, useElementBounding, useIntersectionObserver, useStorage } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import MessageItem from './message-item.vue'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import SessionInfoRing from './session-info-ring.vue'
import ModelOptions from '@/pages/bots/components/model-options.vue'
import ReasoningEffortSelect from '@/pages/bots/components/reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import type { ChatAttachment } from '@/composables/api/useChat'

const props = withDefaults(defineProps<{
  tabId?: string
  active?: boolean
}>(), {
  tabId: 'chat',
  active: true,
})

const { t } = useI18n()
const chatStore = useChatStore()
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
const composerError = ref('')
const modelPopoverOpen = ref(false)
const reasoningPopoverOpen = ref(false)
const inputDrafts = useStorage<Record<string, string>>('chat-input-drafts', {})

const {
  messages,
  streaming,
  currentBotId,
  activeSession,
  activeChatReadOnly,
  loadingOlder,
  loadingChats,
  hasMoreOlder,
  overrideModelId,
  overrideReasoningEffort
} = storeToRefs(chatStore)

const isActive = computed(() => props.active !== false)


const { data: modelData } = useQuery({
  key: ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['providers'],
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
const inputDraftKey = computed(() => {
  const botId = (currentBotId.value ?? '').trim()
  const tabId = props.tabId.trim()
  if (!botId || !tabId) return ''
  return `${botId}:${tabId}`
})

function saveInputDraft(key: string, text: string) {
  if (!key) return
  const next = { ...inputDrafts.value }
  if (text) {
    next[key] = text
  } else {
    delete next[key]
  }
  inputDrafts.value = next
}

watch(inputDraftKey, (nextKey, previousKey) => {
  if (previousKey) {
    saveInputDraft(previousKey, inputText.value)
  }
  inputText.value = nextKey ? inputDrafts.value[nextKey] ?? '' : ''
}, { immediate: true })

watch(inputText, (text) => {
  saveInputDraft(inputDraftKey.value, text)
})

const elNode = useTemplateRef('scrollContainer')
// Resolve the real scrollable viewport via data-slot to avoid coupling to the
// child-index DOM shape of @memohai/ui's ScrollArea (which wraps reka-ui).
const scrollEl = computed<HTMLElement | null>(() => {
  const root = elNode.value?.$el as HTMLElement | undefined
  if (!root) return null
  return root.querySelector('[data-slot="scroll-area-viewport"]') as HTMLElement | null
})
const descEl = computed<HTMLElement | null>(() => {
  return (scrollEl.value?.firstElementChild as HTMLElement | null) ?? null
})
const loadMoreSentinel = useTemplateRef<HTMLElement>('loadMoreSentinel')
const isAutoScroll = ref(true)
const isInstant = ref(false)
const highlightedMessageId = ref('')
const { y, directions, arrivedState, isScrolling } = useScroll(scrollEl, { behavior: computed(() => isAutoScroll.value && isInstant.value ? 'smooth' : 'instant') })
const { height } = useElementBounding(descEl)
let highlightTimer: ReturnType<typeof setTimeout> | null = null

onBeforeUnmount(() => {
  if (highlightTimer) clearTimeout(highlightTimer)
})


const elId: { id: string, top: number }[] = []
function isActiveEl(isActive: boolean, item: { id: string, top: number }) {
  if (lockScroll.value) return
  let index = elId.findIndex(v => v.id === item.id)
  if (isActive) {
    if ((index < 0)) {
      elId.push(item)
    } else {
      elId[index]!.top = item.top
    }
  } else {
    if (index >= 0) {
      elId.splice(index, 1)
    }
  }
}


const lockScroll = ref(true)
let isInit = false
const transitionScroll=ref(false)
onActivated(() => {
  if (!isActive.value) return
  transitionScroll.value=false
  const unwatch = watch(loadingChats, async (newValue) => {
    
    if (elId[0]?.id && !newValue) {
      elId.sort((v1, v2) => Math.abs(v1.top) - Math.abs(v2.top))
      const el: HTMLElement | null = document.querySelector(`[data-message-id="${elId[0]?.id}"]`)
      if (el) {
        let cachePos = elId[0]?.top
        el.scrollIntoView()
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            scrollEl.value?.scrollBy({
              top: cachePos * -1
            })
            transitionScroll.value=true
          })
        })

      }
      setTimeout(() => {
        lockScroll.value = false
        isInit = true
        unwatch()
      })
    } else {
     
      isInit = true
      if (!newValue) {
        setTimeout(async () => {
          lockScroll.value = false
          transitionScroll.value=true
          unwatch()
        })
      }
    }
  }, {
    immediate: true,
    flush: 'post'
  })

})

onDeactivated(() => {
  lockScroll.value = true
  isInstant.value = false
  isAutoScroll.value = true
  isInit = false
  if (arrivedState.bottom) {
    elId.length=0
  }
})

watchEffect(() => {
  if (!isActive.value) return
  if (directions.top && !lockScroll.value) {
    isAutoScroll.value = false
    isInstant.value = false
    return
  }

  if (arrivedState.bottom && !lockScroll.value) {
    isAutoScroll.value = true
    isInstant.value = true
    return
  }
})

watch([isAutoScroll, height, isActive], async () => {
  if (!isActive.value) return
  if (isAutoScroll.value && height.value && isInit) {
    y.value = height.value
  }
}, {
  flush: 'post',
  deep: true
})

// Sentinel-based infinite scroll for older history. The IntersectionObserver
// fires reliably even when the user is pinned at scrollTop=0 (where scroll
// events stop), and we restore the visual position via scrollHeight diff —
// the only anchoring scheme that survives nested scroll containers and
// arbitrary page offsets. After each load we re-check whether the sentinel
// is still inside the rootMargin band and chain another load if so; this
// avoids the "must scroll down then up to load again" symptom that arises
// when IntersectionObserver's isIntersecting state stays sticky-true.
let isLoadingOlderInFlight = false

function isSentinelStillInRange(scrollElement: HTMLElement): boolean {
  const sentinel = loadMoreSentinel.value
  if (!sentinel) return false
  const rootRect = scrollElement.getBoundingClientRect()
  const sentinelRect = sentinel.getBoundingClientRect()
  return sentinelRect.bottom >= rootRect.top - 200
    && sentinelRect.top <= rootRect.bottom
}

async function ensureOlderLoaded() {

  if (isLoadingOlderInFlight) return
  if (loadingOlder.value || !hasMoreOlder.value) return
  if (!messages.value.length) return
  const scrollElement = scrollEl.value
  if (!scrollElement) return


  isLoadingOlderInFlight = true
  // The `if (isAutoScroll) y = height` watchEffect above will otherwise stomp
  // our restored scrollTop the moment new content lands (height grows, effect
  // fires, viewport jumps to bottom, sentinel flies off-screen — and IO never
  // fires again because the user can't scroll back up far enough). The user
  // is at the top by definition (sentinel just intersected), so disabling
  // stick-to-bottom here is correct; arrivedState.bottom will re-enable it
  // when the user scrolls back down to the latest messages.
  // isAutoScroll.value = false
  try {
    while (hasMoreOlder.value) {
      const prevScrollHeight = scrollElement.scrollHeight
      // const prevScrollTop = scrollElement.scrollTop

      let count = 0
      try {
        count = await chatStore.loadOlderMessages()
      } catch (error) {
        console.error('Failed to load older messages:', error)
        return
      }
      if (count <= 0) return

      await nextTick()

      const newScrollHeight = scrollElement.scrollHeight
      const delta = newScrollHeight - prevScrollHeight
      if (delta > 0) {
        // scrollElement.scrollTop = prevScrollTop + delta
      }

      // Yield one frame so the browser can re-evaluate layout and IO entries,
      // then bail out unless the sentinel is still inside the trigger band —
      // meaning the newly prepended page wasn't tall enough to push us out of
      // range and we should keep paginating.
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
      if (!isSentinelStillInRange(scrollElement)) return
    }
  } finally {
    isLoadingOlderInFlight = false
  }
}

useIntersectionObserver(
  loadMoreSentinel,
  ([entry]) => {
    if (!isActive.value) return
    if (!entry?.isIntersecting) return
    void ensureOlderLoaded()
  },
  {
    root: scrollEl,
    rootMargin: '200px 0px 0px 0px',
    threshold: 0,
  },
)

function findMessageElement(messageId: string): HTMLElement | null {
  const root = scrollEl.value
  if (!root) return null
  for (const item of Array.from(root.querySelectorAll<HTMLElement>('[data-message-id]'))) {
    if (item.dataset.messageId === messageId) return item
  }
  return null
}

async function scrollToMessage(messageId: string): Promise<boolean> {
  await nextTick()
  const root = scrollEl.value
  const target = findMessageElement(messageId)
  if (!root || !target) return false
  // isAutoScroll.value = false
  // isInstant.value = true
  const rootRect = root.getBoundingClientRect()
  const targetRect = target.getBoundingClientRect()
  const offset = targetRect.top - rootRect.top - Math.max(24, root.clientHeight * 0.22)

  root.scrollTo({ top: root.scrollTop + offset, behavior: 'smooth' })
  highlightedMessageId.value = messageId
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    if (highlightedMessageId.value === messageId) {
      highlightedMessageId.value = ''
    }
  }, 1800)
  return true
}

async function handleReplyJump(messageId: string) {
  const target = messageId.trim()
  if (!target) return
  const localId = chatStore.findMessageIdByExternalId(target)
  if (localId && await scrollToMessage(localId)) return
  const locatedId = await chatStore.locateMessageByExternalId(target)
  if (locatedId) {
    await scrollToMessage(locatedId)
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  isAutoScroll.value = true
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
  if (!isActive.value) return
  // isAutoScroll.value = true
  const text = inputText.value.trim()
  const files = [...pendingFiles.value]
  if ((!text && !files.length) || streaming.value || activeChatReadOnly.value) return

  const sentDraftKey = inputDraftKey.value
  composerError.value = ''
  inputText.value = ''
  saveInputDraft(sentDraftKey, '')
  pendingFiles.value = []

  let attachments: ChatAttachment[] | undefined
  try {
    if (files.length) {
      attachments = await Promise.all(files.map(fileToAttachment))
    }
  } catch (error) {
    inputText.value = text
    pendingFiles.value = files
    composerError.value = error instanceof Error ? error.message : t('chat.sendFailed')
    return
  }

  const result = await chatStore.sendMessage(text, attachments)
  if (!result.ok && result.stage === 'startup') {
    inputText.value = result.restoreInput ?? text
    pendingFiles.value = files
    composerError.value = result.error || t('chat.sendFailed')
  }
}
</script>
