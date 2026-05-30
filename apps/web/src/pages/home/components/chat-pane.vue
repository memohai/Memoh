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
                v-for="(msg, msgIndex) in messages"
                :key="msg.id"
                :data-message-id="msg.id"
                :data-scroll-segment-id="messageSegmentDomId(msg, msgIndex)"
                :data-external-message-id="(msg.role === 'user' || msg.role === 'assistant') ? msg.externalMessageId : undefined"
                class="rounded-2xl transition-[background-color,box-shadow] duration-500"
                :class="highlightedMessageId === msg.id ? 'bg-muted/45 ring-1 ring-border shadow-sm' : ''"
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

          <div
            v-if="showScrollRail"
            class="group hidden md:flex absolute right-2 top-1/2 z-10 w-64 -translate-y-1/2 flex-col items-end pointer-events-none"
            aria-label="Conversation navigation"
          >
            <div
              v-if="hoveredScrollSegment"
              class="absolute right-12 top-1/2 w-fit max-w-72 -translate-y-1/2 rounded-lg border bg-background/95 px-2.5 py-1.5 text-left font-mono shadow-md backdrop-blur transition-opacity duration-150"
            >
              <div
                v-if="hoveredScrollSegment.role === 'assistant'"
                class="mb-1 text-[10px] font-medium leading-none text-muted-foreground"
              >
                memoh
              </div>
              <div class="line-clamp-4 overflow-hidden text-ellipsis break-words text-[11px] leading-snug text-foreground">
                {{ hoveredScrollSegment.preview }}
              </div>
            </div>

            <button
              type="button"
              class="absolute bottom-full mb-1 flex size-8 translate-y-1 items-center justify-center rounded-full border border-transparent text-muted-foreground opacity-0 transition-all duration-200 hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-0 disabled:hover:bg-transparent disabled:hover:text-muted-foreground group-hover:translate-y-0 group-hover:opacity-100 disabled:group-hover:opacity-40 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring pointer-events-auto"
              aria-label="Navigate to previous message"
              :disabled="!previousScrollSegment"
              @click="scrollToAdjacentSegment(-1)"
            >
              <ChevronUp class="size-4" />
            </button>

            <div class="flex flex-col items-end gap-0 pointer-events-auto">
              <button
                v-for="segment in scrollSegments"
                :key="segment.id"
                type="button"
                class="group/timeline-tick relative flex h-3 w-8 cursor-pointer items-center justify-center rounded-full border border-transparent text-xs font-medium text-muted-foreground transition-colors duration-100 hover:bg-transparent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                :aria-label="segment.label"
                @mouseenter="hoveredSegmentId = segment.id"
                @mouseleave="hoveredSegmentId = ''"
                @focus="hoveredSegmentId = segment.id"
                @blur="hoveredSegmentId = ''"
                @click="scrollToSegment(segment)"
              >
                <span
                  class="h-px rounded-full bg-muted-foreground/70 opacity-50 transition-all duration-150 group-hover:opacity-100 group-hover/timeline-tick:w-4 group-hover/timeline-tick:bg-foreground/70"
                  :class="[
                    activeSegmentId === segment.id ? 'w-4 bg-foreground/80 opacity-100' : segment.index % 2 === 0 ? 'w-1.5' : 'w-3',
                    hoveredSegmentId === segment.id ? '!w-4 !bg-foreground/80 opacity-100' : '',
                  ]"
                />
              </button>
            </div>

            <button
              type="button"
              class="absolute top-full mt-1 flex size-8 -translate-y-1 items-center justify-center rounded-full border border-transparent text-muted-foreground opacity-0 transition-all duration-200 hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-0 disabled:hover:bg-transparent disabled:hover:text-muted-foreground group-hover:translate-y-0 group-hover:opacity-100 disabled:group-hover:opacity-40 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring pointer-events-auto"
              aria-label="Navigate to next message"
              :disabled="!nextScrollSegment"
              @click="scrollToAdjacentSegment(1)"
            >
              <ChevronDown class="size-4" />
            </button>
          </div>
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
        <div class="relative w-full max-w-4xl mx-auto">
          <Transition
            enter-active-class="transition-opacity duration-150 ease-out"
            enter-from-class="opacity-0"
            enter-to-class="opacity-100"
            leave-active-class="transition-opacity duration-150 ease-in"
            leave-from-class="opacity-100"
            leave-to-class="opacity-0"
          >
            <Button
              v-if="showJumpToBottom"
              type="button"
              size="icon"
              variant="secondary"
              class="absolute left-1/2 bottom-full z-20 mb-2 size-8 -translate-x-1/2 rounded-full border bg-background/95 shadow-sm backdrop-blur hover:bg-accent"
              aria-label="Scroll to latest message"
              @click="scrollToBottom"
            >
              <ArrowDown class="size-4" />
            </Button>
          </Transition>

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
                <Popover v-model:open="agentPopoverOpen">
                  <PopoverTrigger as-child>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      :disabled="!currentBotId || activeChatReadOnly || agentChanging || !canChangeAgent"
                      class="gap-1.5 text-muted-foreground max-w-40"
                    >
                      <LoaderCircle
                        v-if="agentChanging"
                        class="size-3 animate-spin"
                      />
                      <component
                        :is="selectedAgentIcon"
                        v-else
                        class="size-3.5 shrink-0"
                      />
                      <span class="truncate text-[11px]">{{ selectedAgentLabel }}</span>
                      <ChevronDown class="size-3 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    class="w-56 p-1"
                    align="start"
                  >
                    <button
                      type="button"
                      class="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs hover:bg-muted"
                      @click="selectMemohAgent"
                    >
                      <MessageSquare class="size-3.5 text-muted-foreground" />
                      <span class="min-w-0 flex-1 truncate">{{ $t('chat.agentMemoh') }}</span>
                      <Check
                        v-if="!activeIsACP"
                        class="size-3 text-muted-foreground"
                      />
                    </button>
                    <button
                      v-for="profile in enabledACPProfiles"
                      :key="profile.id"
                      type="button"
                      class="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs hover:bg-muted"
                      @click="selectACPAgent(profile)"
                    >
                      <component
                        :is="acpAgentIcon(profile.id, true)"
                        class="size-3.5 shrink-0"
                      />
                      <span class="min-w-0 flex-1 truncate">{{ profile.display_name || profile.id }}</span>
                      <Check
                        v-if="activeACPAgentId === normalizedProfileID(profile.id)"
                        class="size-3 text-muted-foreground"
                      />
                    </button>
                  </PopoverContent>
                </Popover>

                <Popover v-model:open="modelPopoverOpen">
                  <PopoverTrigger as-child>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      :disabled="!currentBotId || activeChatReadOnly || acpModelChanging || acpModelsLoading"
                      class="gap-0.5 text-muted-foreground max-w-40"
                    >
                      <LoaderCircle
                        v-if="acpModelChanging || acpModelsLoading"
                        class="size-3 animate-spin"
                      />
                      <span class="truncate text-[11px]">{{ selectedModelLabel }}</span>
                      <ChevronDown class="size-3 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    class="w-96 p-0"
                    align="start"
                  >
                    <div
                      v-if="activeIsACP"
                      class="max-h-80 overflow-y-auto p-1"
                    >
                      <div
                        v-if="acpModelsLoading"
                        class="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
                      >
                        <LoaderCircle class="size-3 animate-spin" />
                        {{ $t('common.loading') }}
                      </div>
                      <div
                        v-else-if="!acpModels.length"
                        class="px-2 py-3 text-xs text-muted-foreground"
                      >
                        {{ $t('chat.noModels') }}
                      </div>
                      <button
                        v-for="model in acpModels"
                        v-else
                        :key="model.id || model.name"
                        type="button"
                        class="flex min-h-8 w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted"
                        @click="onACPModelSelected(model)"
                      >
                        <span class="min-w-0 flex-1">
                          <span class="block truncate">
                            {{ model.name || model.id }}
                          </span>
                          <span
                            v-if="model.description"
                            class="mt-0.5 block line-clamp-2 text-[11px] leading-snug text-muted-foreground"
                          >
                            {{ model.description }}
                          </span>
                        </span>
                        <Check
                          v-if="model.id === currentACPModelId"
                          class="mt-0.5 size-3 shrink-0 text-muted-foreground"
                        />
                      </button>
                    </div>
                    <ModelOptions
                      v-else
                      v-model="overrideModelId"
                      :models="models"
                      :providers="providers"
                      model-type="chat"
                      :open="modelPopoverOpen"
                      @update:model-value="onModelSelected"
                    />
                  </PopoverContent>
                </Popover>

                <Button
                  v-if="activeIsACP"
                  type="button"
                  size="sm"
                  variant="ghost"
                  class="gap-1 text-muted-foreground max-w-40"
                  disabled
                >
                  <FolderOpen class="size-3.5 shrink-0" />
                  <span class="truncate text-[11px]">{{ activeACPProjectLabel }}</span>
                </Button>

                <Popover
                  v-if="!activeIsACP"
                  v-model:open="reasoningPopoverOpen"
                >
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
                  v-if="!activeIsACP"
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
                  v-if="!activeIsACP"
                  class="ml-auto"
                  :override-model-id="overrideModelId"
                />
                <div
                  v-else
                  class="ml-auto"
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
import { LoaderCircle, Image as ImageIcon, File as FileIcon, X, Paperclip, Send, ChevronDown, ChevronUp, Lightbulb, CircleAlert, ArrowDown, MessageSquare, Check, FolderOpen } from 'lucide-vue-next'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Popover, PopoverContent, PopoverTrigger } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useScroll, useElementBounding, useIntersectionObserver, useStorage } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getAcpProfiles, getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { AcpclientModelInfo, AcpprofilePublicProfile, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import MessageItem from './message-item.vue'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import SessionInfoRing from './session-info-ring.vue'
import ModelOptions from '@/pages/bots/components/model-options.vue'
import ReasoningEffortSelect from '@/pages/bots/components/reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY, REASONING_EFFORT_ADAPTIVE, REASONING_EFFORT_DISABLE } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import type { ChatAttachment } from '@/composables/api/useChat'
import { onAuthSessionCleared } from '@/lib/auth-session'
import type { ChatMessage } from '@/store/chat-list'
import { useACPRuntime } from '@/composables/useACPRuntime'
import { acpAgentDisplayName, acpAgentIcon, ACP_NO_PROJECT_MODE, createACPNoProjectPath, isACPAgentEnabled, isACPNoProject, normalizeACPAgentID } from '@/utils/acp'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface ScrollSegment {
  id: string
  targetSegmentId: string
  targetMessageId: string
  role: 'user' | 'assistant'
  label: string
  preview: string
  index: number
  top: number
  topPercent: number
}

interface ScrollSegmentSource {
  message: ChatMessage
  messageIndex: number
}

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
const agentPopoverOpen = ref(false)
const agentChanging = ref(false)
const acpModelChanging = ref(false)
const inputDrafts = useStorage<Record<string, string>>('chat-input-drafts', {})

const {
  messages,
  streaming,
  currentBotId,
  bots,
  activeSession,
  activeChatReadOnly,
  loadingOlder,
  loadingChats,
  hasMoreOlder,
  overrideModelId,
  overrideReasoningEffort,
  startupSendFailure
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

const { data: acpProfileData } = useQuery({
  key: () => ['acp-profiles'],
  query: async () => {
    const { data } = await getAcpProfiles({ throwOnError: true })
    return data
  },
})

const currentBot = computed(() => bots.value.find(bot => bot.id === currentBotId.value) ?? null)
const acpProfiles = computed<AcpprofilePublicProfile[]>(() => acpProfileData.value?.items ?? [])
const enabledACPProfiles = computed(() =>
  acpProfiles.value.filter(profile => isACPAgentEnabled(currentBot.value?.metadata as Record<string, unknown> | undefined, profile.id)),
)

const activeSessionMetadata = computed<Record<string, unknown>>(() =>
  activeSession.value?.metadata && typeof activeSession.value.metadata === 'object'
    ? activeSession.value.metadata
    : {},
)
const activeIsACP = computed(() => activeSession.value?.type === 'acp_agent')
const activeACPAgentId = computed(() => normalizeACPAgentID(activeSessionMetadata.value.acp_agent_id))
const selectedAgentIcon = computed(() => activeIsACP.value ? acpAgentIcon(activeACPAgentId.value, true) : MessageSquare)
const selectedAgentLabel = computed(() =>
  activeIsACP.value
    ? acpAgentDisplayName(activeACPAgentId.value, t('chat.agentCodex'))
    : t('chat.agentMemoh'),
)
const activeACPProjectLabel = computed(() => {
  if (isACPNoProject(activeSessionMetadata.value)) return t('chat.noProject')
  const path = String(activeSessionMetadata.value.project_path ?? '').trim()
  const parts = path.split('/').filter(Boolean)
  return path ? parts[parts.length - 1] ?? path : t('chat.noProject')
})
const canChangeAgent = computed(() => !streaming.value && messages.value.length === 0)
const activeSessionId = computed(() => activeSession.value?.id ?? '')
const {
  runtime: acpRuntime,
  models: acpModels,
  currentModelId: currentACPModelId,
  isEnsuring: acpRuntimeEnsuring,
  setModel: setActiveACPModel,
} = useACPRuntime({
  botId: currentBotId,
  sessionId: activeSessionId,
  enabled: computed(() => activeIsACP.value && !!currentBotId.value && !!activeSessionId.value),
  onError: (error) => {
    if (activeIsACP.value) {
      composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
    }
  },
})

const models = computed<ModelsGetResponse[]>(() => modelData.value ?? [])
const providers = computed<ProvidersGetResponse[]>(() => providerData.value ?? [])
const acpModelsLoading = computed(() =>
  activeIsACP.value && !acpRuntime.value?.models && (agentChanging.value || acpRuntimeEnsuring.value),
)

const activeModel = computed(() => {
  const id = overrideModelId.value || botSettings.value?.chat_model_id || ''
  return models.value.find((m) => m.id === id)
})

const activeModelSupportsReasoning = computed(() =>
  !!activeModel.value?.config?.compatibilities?.includes('reasoning'),
)

const availableReasoningEfforts = computed(() => {
  const efforts = ((activeModel.value?.config as { reasoning_efforts?: string[] } | undefined)?.reasoning_efforts ?? [])
    .filter((e) => [REASONING_EFFORT_ADAPTIVE, 'none', 'low', 'medium', 'high', 'xhigh'].includes(e))
  return [...new Set([REASONING_EFFORT_ADAPTIVE, ...(efforts.length > 0 ? efforts : ['low', 'medium', 'high'])])]
})

const selectedModelLabel = computed(() => {
  if (activeIsACP.value) {
    const current = acpModels.value.find(model => model.id === currentACPModelId.value)
    return current?.name || current?.id || currentACPModelId.value || t('chat.modelDefault')
  }
  const m = models.value.find((m) => m.id === overrideModelId.value)
  return m?.name || m?.model_id || t('chat.modelDefault')
})

const selectedReasoningLabel = computed(() => {
  const v = overrideReasoningEffort.value
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
      overrideReasoningEffort.value = REASONING_EFFORT_DISABLE
    }
  }
}

watch(botSettings, () => initFromBotSettings(), { immediate: true })

watch(currentBotId, () => {
  overrideModelId.value = ''
  overrideReasoningEffort.value = ''
})

watch(activeIsACP, (isACP) => {
  if (isACP) {
    pendingFiles.value = []
  }
})

function normalizedProfileID(value: unknown): string {
  return normalizeACPAgentID(value)
}

async function selectACPAgent(profile: AcpprofilePublicProfile) {
  const agentId = normalizeACPAgentID(profile.id)
  if (!agentId || agentChanging.value || !canChangeAgent.value) return
  agentPopoverOpen.value = false
  agentChanging.value = true
  composerError.value = ''
  try {
    const projectPath = createACPNoProjectPath()
    if (chatStore.sessionId) {
      await chatStore.updateCurrentSessionAgent({
        agentId,
        projectPath,
        projectMode: ACP_NO_PROJECT_MODE,
      })
    } else {
      await chatStore.createACPSession({
        agentId,
        projectPath,
        projectMode: ACP_NO_PROJECT_MODE,
      })
    }
    pendingFiles.value = []
  } catch (error) {
    composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
  } finally {
    agentChanging.value = false
  }
}

async function selectMemohAgent() {
  if (agentChanging.value || !canChangeAgent.value) return
  agentPopoverOpen.value = false
  if (!activeIsACP.value || !chatStore.sessionId) return
  agentChanging.value = true
  composerError.value = ''
  try {
    await chatStore.updateCurrentSessionToMemoh()
  } catch (error) {
    composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
  } finally {
    agentChanging.value = false
  }
}

function onModelSelected() {
  modelPopoverOpen.value = false
  if (!activeModelSupportsReasoning.value) {
    overrideReasoningEffort.value = REASONING_EFFORT_DISABLE
  }
}

async function onACPModelSelected(model: AcpclientModelInfo) {
  const modelId = (model.id ?? '').trim()
  if (!modelId || acpModelChanging.value) return
  modelPopoverOpen.value = false
  acpModelChanging.value = true
  composerError.value = ''
  try {
    await setActiveACPModel(modelId)
  } catch (error) {
    composerError.value = resolveApiErrorMessage(error, t('chat.modelSwitchFailed'))
  } finally {
    acpModelChanging.value = false
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
const stopAuthSessionCleanup = onAuthSessionCleared(() => {
  inputDrafts.value = {}
  inputText.value = ''
  pendingFiles.value = []
  composerError.value = ''
})
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

watch([
  startupSendFailure,
  currentBotId,
  () => chatStore.sessionId,
  () => props.tabId,
  isActive,
], ([failure]) => {
  if (!failure || !isActive.value) return
  if (failure.botId && failure.botId !== currentBotId.value) return
  if (failure.sessionId && failure.sessionId !== chatStore.sessionId) return
  if (failure.sessionId && props.tabId !== `chat:${failure.sessionId}`) return

  inputText.value = failure.restoreInput
  saveInputDraft(inputDraftKey.value, failure.restoreInput)
  composerError.value = failure.error || t('chat.sendFailed')
  chatStore.clearStartupSendFailure(failure.id)
}, { immediate: true })

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
const scrollSegments = ref<ScrollSegment[]>([])
const activeSegmentId = ref('')
const hoveredSegmentId = ref('')
const scrollAnchorOffset = 8
const scrollAnimationMaxDuration = 760
const scrollAnimationMinDuration = 260
const scrollNavigationLockMs = scrollAnimationMaxDuration + 80
let highlightTimer: ReturnType<typeof setTimeout> | null = null
let scrollNavigationLockTimer: ReturnType<typeof setTimeout> | null = null
let scrollNavigationRaf = 0
let scrollAnimationRaf = 0

onBeforeUnmount(() => {
  stopAuthSessionCleanup()
  if (highlightTimer) clearTimeout(highlightTimer)
  if (scrollNavigationLockTimer) clearTimeout(scrollNavigationLockTimer)
  if (scrollNavigationRaf) cancelAnimationFrame(scrollNavigationRaf)
  if (scrollAnimationRaf) cancelAnimationFrame(scrollAnimationRaf)
})

const showJumpToBottom = computed(() =>
  isActive.value
  && !loadingChats.value
  && messages.value.length > 0
  && !arrivedState.bottom,
)

const showScrollRail = computed(() =>
  isActive.value
  && !loadingChats.value
  && scrollSegments.value.length > 1,
)

const hoveredScrollSegment = computed(() => {
  const id = hoveredSegmentId.value
  return scrollSegments.value.find(segment => segment.id === id)
})

const activeSegmentIndex = computed(() => {
  if (!scrollSegments.value.length) return -1
  const index = scrollSegments.value.findIndex(segment => segment.id === activeSegmentId.value)
  return index >= 0 ? index : 0
})

const previousScrollSegment = computed(() => {
  const index = activeSegmentIndex.value
  return index > 0 ? scrollSegments.value[index - 1] : null
})

const nextScrollSegment = computed(() => {
  const index = activeSegmentIndex.value
  return index >= 0 && index < scrollSegments.value.length - 1 ? scrollSegments.value[index + 1] : null
})

function messageSegmentDomId(message: ChatMessage, index: number) {
  return `${index}:${message.role}:${message.id}`
}

function findSegmentElement(segmentId: string): HTMLElement | null {
  const root = scrollEl.value
  if (!root) return null
  for (const item of Array.from(root.querySelectorAll<HTMLElement>('[data-scroll-segment-id]'))) {
    if (item.dataset.scrollSegmentId === segmentId) return item
  }
  return null
}

function getElementAbsoluteTop(target: HTMLElement, root: HTMLElement, rootRect = root.getBoundingClientRect()) {
  const rect = target.getBoundingClientRect()
  return root.scrollTop + rect.top - rootRect.top
}

function getSegmentAbsoluteTop(segment: ScrollSegment) {
  const target = findSegmentElement(segment.targetSegmentId)
  if (!target) return segment.top
  const root = scrollEl.value
  if (!root) return segment.top
  return getElementAbsoluteTop(target, root)
}

function getSegmentText(message: ChatMessage) {
  if (message.role === 'user') {
    return message.text?.trim().replace(/\s+/g, ' ') || ''
  }
  if (message.role === 'assistant') {
    const textBlock = message.messages.find(block => block.type === 'text')
    return textBlock?.content?.trim().replace(/\s+/g, ' ') || ''
  }
  return ''
}

function getSegmentLabel(index: number, message: ChatMessage) {
  const preview = getSegmentText(message)
  const roleLabel = message.role === 'assistant'
    ? t('chat.timelineAnswer')
    : t('chat.timelineMessage')
  return preview ? `${roleLabel} ${index + 1}. ${preview.slice(0, 48)}` : `${roleLabel} ${index + 1}`
}

function buildSegmentSources(): ScrollSegmentSource[] {
  const sources: ScrollSegmentSource[] = []
  messages.value.forEach((message, messageIndex) => {
    if ((message.role === 'user' || message.role === 'assistant') && getSegmentText(message).length > 0) {
      sources.push({ message, messageIndex })
    }
  })
  return sources
}

const scrollSegmentStructureKey = computed(() =>
  messages.value
    .filter(message => message.role === 'user' || message.role === 'assistant')
    .map(message => `${message.id}:${message.role}:${message.streaming ? '1' : '0'}:${getSegmentText(message).length > 0 ? 'text' : 'empty'}`)
    .join('|'),
)

function measureScrollNavigation() {
  scrollNavigationRaf = 0
  const root = scrollEl.value
  if (!root) return

  const scrollHeight = Math.max(root.scrollHeight, 1)
  const rootRect = root.getBoundingClientRect()
  const segmentSources = buildSegmentSources()
  const nextSegments: ScrollSegment[] = []

  segmentSources.forEach(({ message, messageIndex }, index) => {
    const targetSegmentId = messageSegmentDomId(message, messageIndex)
    const target = findSegmentElement(targetSegmentId)
    if (!target) return

    const absoluteTop = getElementAbsoluteTop(target, root, rootRect)

    nextSegments.push({
      id: targetSegmentId,
      targetSegmentId,
      targetMessageId: message.id,
      role: message.role,
      label: getSegmentLabel(index, message),
      preview: getSegmentText(message) || getSegmentLabel(index, message),
      index,
      top: absoluteTop,
      topPercent: Math.min(100, Math.max(0, (absoluteTop / scrollHeight) * 100)),
    })
  })

  scrollSegments.value = nextSegments

  if (scrollNavigationLockTimer) return

  const viewportAnchor = root.scrollTop + scrollAnchorOffset
  let active = nextSegments[0]?.id ?? ''
  let activeDistance = Number.POSITIVE_INFINITY
  for (const segment of nextSegments) {
    const distance = Math.abs(segment.top - viewportAnchor)
    if (distance < activeDistance) {
      active = segment.id
      activeDistance = distance
    }
  }
  activeSegmentId.value = active
}

function scheduleScrollNavigationMeasure() {
  if (scrollNavigationRaf) return
  scrollNavigationRaf = requestAnimationFrame(measureScrollNavigation)
}

function easeInOutCubic(progress: number) {
  return progress < 0.5
    ? 4 * progress * progress * progress
    : 1 - ((-2 * progress + 2) ** 3) / 2
}

function scrollViewportTo(top: number, animated = true) {
  const root = scrollEl.value
  if (!root) return
  const nextTop = Math.min(Math.max(top, 0), Math.max(root.scrollHeight - root.clientHeight, 0))
  if (scrollAnimationRaf) cancelAnimationFrame(scrollAnimationRaf)

  if (!animated) {
    root.scrollTop = nextTop
    y.value = nextTop
    scheduleScrollNavigationMeasure()
    return
  }

  const startTop = root.scrollTop
  const distance = nextTop - startTop
  if (Math.abs(distance) < 1) {
    root.scrollTop = nextTop
    y.value = nextTop
    scheduleScrollNavigationMeasure()
    return
  }

  const duration = Math.min(scrollAnimationMaxDuration, Math.max(scrollAnimationMinDuration, Math.abs(distance) * 0.45))
  const startedAt = performance.now()

  const step = (now: number) => {
    const progress = Math.min(1, (now - startedAt) / duration)
    root.scrollTop = startTop + distance * easeInOutCubic(progress)
    if (progress < 1) {
      scrollAnimationRaf = requestAnimationFrame(step)
      return
    }
    root.scrollTop = nextTop
    y.value = nextTop
    scrollAnimationRaf = 0
    scheduleScrollNavigationMeasure()
  }

  scrollAnimationRaf = requestAnimationFrame(step)
}

async function scrollToSegment(segment: ScrollSegment) {
  activeSegmentId.value = segment.id
  if (scrollNavigationLockTimer) clearTimeout(scrollNavigationLockTimer)
  scrollNavigationLockTimer = setTimeout(() => {
    scrollNavigationLockTimer = null
    scheduleScrollNavigationMeasure()
  }, scrollNavigationLockMs)
  await scrollToMessage(segment)
}

async function scrollToAdjacentSegment(direction: -1 | 1) {
  const target = resolveAdjacentSegment(direction)
  if (!target) return
  await scrollToSegment(target)
}

function resolveAdjacentSegment(direction: -1 | 1) {
  const segments = scrollSegments.value
  if (!segments.length) return null

  const targetIndex = activeSegmentIndex.value + direction
  return targetIndex >= 0 && targetIndex < segments.length ? segments[targetIndex] : null
}

function scrollToBottom() {
  const root = scrollEl.value
  if (!root) return
  isAutoScroll.value = true
  isInstant.value = true
  scrollViewportTo(root.scrollHeight)
}


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

watch([scrollSegmentStructureKey, y, height, isActive], async () => {
  if (!isActive.value) return
  await nextTick()
  scheduleScrollNavigationMeasure()
}, {
  flush: 'post',
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

async function scrollToMessage(messageOrSegment: string | ScrollSegment): Promise<boolean> {
  await nextTick()
  const root = scrollEl.value
  const target = typeof messageOrSegment === 'string'
    ? findMessageElement(messageOrSegment)
    : findSegmentElement(messageOrSegment.targetSegmentId)
  if (!root || !target) return false
  isAutoScroll.value = false
  isInstant.value = false
  const messageId = typeof messageOrSegment === 'string'
    ? messageOrSegment
    : messageOrSegment.targetMessageId
  const absoluteTop = typeof messageOrSegment === 'string'
    ? getElementAbsoluteTop(target, root)
    : getSegmentAbsoluteTop(messageOrSegment)
  const nextTop = absoluteTop - scrollAnchorOffset

  scrollViewportTo(nextTop)
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
  if (activeIsACP.value && files.length) {
    composerError.value = t('chat.acpAttachmentsUnsupported')
    return
  }

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
    const restoreInput = result.restoreInput ?? text
    inputText.value = restoreInput
    saveInputDraft(inputDraftKey.value || sentDraftKey, restoreInput)
    pendingFiles.value = files
    composerError.value = result.error || t('chat.sendFailed')
  }
}
</script>
