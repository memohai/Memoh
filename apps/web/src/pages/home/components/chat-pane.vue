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
                class="rounded-2xl transition-[background-color] duration-500 scroll-mt-2 [content-visibility:auto] [contain-intrinsic-size:auto_200px]"
                :class="highlightedMessageId === msg.id ? 'bg-muted/45' : ''"
                :data-anchor="msg.id"
              >
                <MessageItem
                  :message="msg"
                  :session-type="activeSession?.type"
                  :bot-id="currentBotId"
                  :on-open-media="galleryOpenBySrc"
                  :on-reply-click="handleReplyJump"
                  @active="isActiveEl"
                />
              </div>
            </div>
          </ScrollArea>

          <ChatMinimap
            v-if="isActive && !loadingChats"
            :scroll-el="scrollEl"
            :content-el="descEl"
            :messages="messages"
            :has-more-older="hasMoreOlder"
            @navigate="handleMinimapNavigate"
          />
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
            enter-active-class="motion-safe:transition-opacity motion-safe:duration-150 ease-out"
            enter-from-class="motion-safe:opacity-0"
            enter-to-class="opacity-100"
            leave-active-class="motion-safe:transition-opacity motion-safe:duration-150 ease-in"
            leave-from-class="opacity-100"
            leave-to-class="motion-safe:opacity-0"
          >
            <BgTaskPill
              v-if="bgTaskPill"
              :pill="bgTaskPill"
              class="absolute left-0 bottom-full z-20 mb-2 max-w-[calc(50%-2rem)]"
              @jump="scrollToOffscreen"
            />
          </Transition>

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
            <Transition
              enter-active-class="transition-all duration-150 ease-out"
              enter-from-class="opacity-0 translate-y-1"
              enter-to-class="opacity-100 translate-y-0"
              leave-active-class="transition-all duration-100 ease-in"
              leave-from-class="opacity-100 translate-y-0"
              leave-to-class="opacity-0 translate-y-1"
            >
              <div
                v-if="pendingUserInput"
                class="mb-2 overflow-hidden rounded-lg border border-border bg-card shadow-sm"
              >
                <div
                  class="max-h-[45vh] overflow-y-auto overscroll-contain px-3 py-2 pr-2"
                  style="scrollbar-gutter: stable;"
                >
                  <div
                    v-for="(question, questionIndex) in pendingUserInputQuestions"
                    :key="question.id"
                    :class="questionIndex > 0 ? 'mt-3 border-t border-border/60 pt-3' : ''"
                  >
                    <p class="whitespace-pre-wrap break-words text-xs font-medium leading-relaxed text-foreground">
                      {{ question.text }}
                    </p>
                    <div>
                      <div
                        v-if="question.kind !== 'text' && question.options?.length"
                        class="mt-2 flex flex-col gap-1"
                      >
                        <Button
                          v-for="option in question.options"
                          :key="option.id"
                          type="button"
                          size="sm"
                          variant="ghost"
                          class="h-auto min-h-8 w-full justify-start whitespace-normal rounded-md px-2.5 py-1.5 text-left text-xs"
                          :class="isPendingUserInputOptionSelected(question.id, option.id) ? 'bg-muted text-foreground' : 'text-foreground hover:bg-accent'"
                          :title="option.description || option.label"
                          :role="question.kind === 'multi_select' ? 'checkbox' : 'radio'"
                          :aria-checked="isPendingUserInputOptionSelected(question.id, option.id)"
                          @click="togglePendingUserInputOption(question, option.id)"
                        >
                          <span
                            class="mr-2 flex size-4 shrink-0 items-center justify-center"
                            :class="isPendingUserInputOptionSelected(question.id, option.id) ? 'text-foreground' : 'text-muted-foreground'"
                          >
                            <component
                              :is="pendingUserInputOptionIcon(question, isPendingUserInputOptionSelected(question.id, option.id))"
                              class="size-4"
                            />
                          </span>
                          <span class="min-w-0 flex-1 break-words">{{ option.label }}</span>
                        </Button>
                        <Button
                          v-if="question.allow_custom"
                          type="button"
                          size="sm"
                          variant="ghost"
                          class="h-auto min-h-8 w-full justify-start whitespace-normal rounded-md px-2.5 py-1.5 text-left text-xs"
                          :class="isPendingUserInputCustomSelected(question.id) ? 'bg-muted text-foreground' : 'text-foreground hover:bg-accent'"
                          :role="question.kind === 'multi_select' ? 'checkbox' : 'radio'"
                          :aria-checked="isPendingUserInputCustomSelected(question.id)"
                          @click="togglePendingUserInputCustom(question)"
                        >
                          <span
                            class="mr-2 flex size-4 shrink-0 items-center justify-center"
                            :class="isPendingUserInputCustomSelected(question.id) ? 'text-foreground' : 'text-muted-foreground'"
                          >
                            <component
                              :is="pendingUserInputOptionIcon(question, isPendingUserInputCustomSelected(question.id))"
                              class="size-4"
                            />
                          </span>
                          <span class="min-w-0 flex-1 break-words">{{ $t('chat.tools.userInputCustomOption') }}</span>
                        </Button>
                      </div>
                      <div
                        v-if="question.kind === 'text' || isPendingUserInputCustomSelected(question.id)"
                        class="mt-1 flex items-center gap-2"
                      >
                        <input
                          :value="pendingUserInputDraftText(question)"
                          class="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          :placeholder="question.placeholder || $t('chat.tools.userInputPlaceholder')"
                          @input="setPendingUserInputDraftText(question, ($event.target as HTMLInputElement).value)"
                          @keydown.enter.prevent="handlePendingUserInputSubmit"
                        >
                      </div>
                    </div>
                  </div>
                </div>
                <div class="flex items-center justify-end gap-2 border-t border-border/60 bg-card px-3 py-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    class="text-xs text-muted-foreground hover:text-foreground"
                    @click="handlePendingUserInputCancel"
                  >
                    {{ $t('chat.tools.cancelUserInput') }}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    class="text-xs"
                    :disabled="!canSubmitPendingUserInput"
                    @click="handlePendingUserInputSubmit"
                  >
                    {{ $t('chat.tools.submitUserInput') }}
                  </Button>
                </div>
              </div>
            </Transition>
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
                      :disabled="!currentBotId || activeChatReadOnly || acpModelChanging"
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
                      v-if="activeIsPendingACP"
                      class="max-h-80 overflow-y-auto p-1"
                    >
                      <button
                        type="button"
                        class="flex min-h-8 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted"
                        @click="onPendingACPDefaultModelSelected"
                      >
                        <span class="min-w-0 flex-1 truncate">{{ $t('chat.modelDefault') }}</span>
                        <Check
                          v-if="!pendingACPModelId"
                          class="mt-0.5 size-3 shrink-0 text-muted-foreground"
                        />
                      </button>
                      <div
                        v-if="acpModelsLoading"
                        class="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
                      >
                        <LoaderCircle class="size-3 animate-spin" />
                        {{ $t('common.loading') }}
                      </div>
                      <div
                        v-else-if="!pendingACPModelOptions.length"
                        class="px-2 py-3 text-xs text-muted-foreground"
                      >
                        {{ $t('chat.noModels') }}
                      </div>
                      <template v-else>
                        <button
                          v-for="model in pendingACPModelOptions"
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
                            v-if="model.id === pendingACPModelId"
                            class="mt-0.5 size-3 shrink-0 text-muted-foreground"
                          />
                        </button>
                      </template>
                    </div>
                    <div
                      v-else-if="activeIsACP"
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
import {
  LoaderCircle,
  Image as ImageIcon,
  File as FileIcon,
  X,
  Paperclip,
  Send,
  ChevronDown,
  Lightbulb,
  CircleAlert,
  ArrowDown,
  MessageSquare,
  Check,
  FolderOpen,
  Square,
  SquareCheck,
  Circle,
  CircleDot,
} from 'lucide-vue-next'
import { ScrollArea, Button, InputGroup, InputGroupAddon, InputGroupTextarea, Popover, PopoverContent, PopoverTrigger } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useScroll, useElementBounding, useIntersectionObserver, useStorage } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getAcpProfiles, getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { AcpclientModelInfo, AcpprofilePublicProfile, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import MessageItem from './message-item.vue'
import ChatMinimap from './chat-minimap.vue'
import { animateScrollTo } from './chat-minimap'
import BgTaskPill from './bg-task-pill.vue'
import { provideBgTaskBeacons } from '../composables/useBgTaskBeacons'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import SessionInfoRing from './session-info-ring.vue'
import ModelOptions from '@/pages/bots/components/model-options.vue'
import ReasoningEffortSelect from '@/pages/bots/components/reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY, REASONING_EFFORT_ADAPTIVE, REASONING_EFFORT_DISABLE } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import type { ChatAttachment, UIUserInput, UIUserInputQuestion, WSUserInputAnswer } from '@/composables/api/useChat'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { useACPRuntime } from '@/composables/useACPRuntime'
import { acpAgentDisplayName, acpAgentIcon, isACPAgentEnabled, isACPNoProject, normalizeACPAgentID } from '@/utils/acp'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface PendingUserInputDraft {
  optionIds: string[]
  customSelected: boolean
  customText: string
  text: string
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
const { pill: bgTaskPill, scrollToOffscreen, cleanup: cleanupBgTaskBeacons } = provideBgTaskBeacons()
onBeforeUnmount(cleanupBgTaskBeacons)
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])
const composerError = ref('')
const pendingUserInputDrafts = ref<Record<string, PendingUserInputDraft>>({})
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
  startupSendFailure,
  pendingACPSessionMetadata,
  pendingACPModelId,
  pendingACPRuntimeStatus,
  pendingACPRuntimeEnsuring,
} = storeToRefs(chatStore)

const isActive = computed(() => props.active !== false)

const pendingUserInput = computed<UIUserInput | null>(() => {
  for (let msgIndex = messages.value.length - 1; msgIndex >= 0; msgIndex--) {
    const message = messages.value[msgIndex]
    if (!message || message.role !== 'assistant') continue
    for (let blockIndex = message.messages.length - 1; blockIndex >= 0; blockIndex--) {
      const block = message.messages[blockIndex]
      if (
        block?.type === 'tool'
        && block.userInput?.user_input_id
        && block.userInput.status === 'pending'
        && block.userInput.can_respond !== false
      ) {
        return block.userInput
      }
    }
  }
  return null
})

const pendingUserInputQuestions = computed(() => pendingUserInput.value?.questions ?? [])

// All questions must be answered per kind before submit; null means incomplete.
const pendingUserInputAnswers = computed<WSUserInputAnswer[] | null>(() => {
  const questions = pendingUserInputQuestions.value
  if (!questions.length) return null
  const answers: WSUserInputAnswer[] = []
  for (const question of questions) {
    const answer = pendingUserInputAnswerFor(question)
    if (!answer) return null
    answers.push(answer)
  }
  return answers
})

const canSubmitPendingUserInput = computed(() => pendingUserInputAnswers.value !== null)

watch(
  () => pendingUserInput.value?.user_input_id,
  () => {
    pendingUserInputDrafts.value = {}
  },
)

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
    : pendingACPSessionMetadata.value ?? {},
)
const activeIsPendingACP = computed(() => !activeSession.value && !!pendingACPSessionMetadata.value)
const activeIsACP = computed(() => activeSession.value?.type === 'acp_agent' || activeIsPendingACP.value)
const activeACPAgentId = computed(() => normalizeACPAgentID(activeSessionMetadata.value.acp_agent_id))
const selectedAgentIcon = computed(() => activeIsACP.value ? acpAgentIcon(activeACPAgentId.value, true) : MessageSquare)
const selectedAgentLabel = computed(() =>
  activeIsACP.value
    ? acpAgentDisplayName(activeACPAgentId.value, t('chat.sessionTypeACPAgent'))
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
  activeIsPendingACP.value
    ? !pendingACPRuntimeStatus.value?.models && (agentChanging.value || pendingACPRuntimeEnsuring.value)
    : activeIsACP.value && !acpRuntime.value?.models && (agentChanging.value || acpRuntimeEnsuring.value),
)

const pendingACPModelOptions = computed<AcpclientModelInfo[]>(() => {
  return activeIsPendingACP.value ? pendingACPRuntimeStatus.value?.models?.available_models ?? [] : []
})

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
  if (activeIsPendingACP.value) {
    const pending = pendingACPModelId.value
    if (pending) {
      const current = pendingACPModelOptions.value.find(model => model.id === pending)
      return current?.name || current?.id || pending
    }
    return t('chat.modelDefault')
  }
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

watch(activeIsPendingACP, (isPending) => {
  if (!isPending) return
  void chatStore.ensurePendingACPRuntime().catch((error) => {
    composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
  })
}, { immediate: true })

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
    if (chatStore.sessionId) {
      await chatStore.updateCurrentSessionAgent({
        agentId,
      })
    } else {
      chatStore.stageACPSession({
        agentId,
      })
      await chatStore.ensurePendingACPRuntime()
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
  if (!activeIsACP.value) return
  if (!chatStore.sessionId) {
    chatStore.clearPendingACPSession()
    pendingFiles.value = []
    return
  }
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
  if (activeIsPendingACP.value) {
    acpModelChanging.value = true
    composerError.value = ''
    try {
      await chatStore.setPendingACPModel(modelId)
    } catch (error) {
      composerError.value = resolveApiErrorMessage(error, t('chat.modelSwitchFailed'))
    } finally {
      acpModelChanging.value = false
    }
    return
  }
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

async function onPendingACPDefaultModelSelected() {
  if (acpModelChanging.value) return
  modelPopoverOpen.value = false
  acpModelChanging.value = true
  composerError.value = ''
  try {
    // May reset the warm runtime back to the agent default model.
    await chatStore.setPendingACPModel('')
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
let highlightTimer: ReturnType<typeof setTimeout> | null = null
let cancelScrollTween: (() => void) | null = null

onBeforeUnmount(() => {
  stopAuthSessionCleanup()
  if (highlightTimer) clearTimeout(highlightTimer)
  cancelScrollTween?.()
})

// The tween re-reads its target every frame, so positions shifted by
// content-visibility materializing rows mid-flight still land exactly.
function startScrollTween(root: HTMLElement, getTarget: () => number) {
  cancelScrollTween?.()
  const stop = animateScrollTo(root, () => {
    const max = Math.max(root.scrollHeight - root.clientHeight, 0)
    return Math.min(Math.max(getTarget(), 0), max)
  })
  const cancel = () => {
    stop()
    root.removeEventListener('wheel', cancel)
    root.removeEventListener('touchstart', cancel)
    cancelScrollTween = null
  }
  root.addEventListener('wheel', cancel, { passive: true })
  root.addEventListener('touchstart', cancel, { passive: true })
  cancelScrollTween = cancel
}

const showJumpToBottom = computed(() =>
  isActive.value
  && !loadingChats.value
  && messages.value.length > 0
  && !arrivedState.bottom,
)

function getElementAbsoluteTop(target: HTMLElement, root: HTMLElement) {
  return root.scrollTop + target.getBoundingClientRect().top - root.getBoundingClientRect().top
}

function scrollViewportTo(getTop: () => number) {
  const root = scrollEl.value
  if (!root) return
  startScrollTween(root, getTop)
}

function handleMinimapNavigate(messageId: string) {
  void scrollToMessage(messageId)
}

function scrollToBottom() {
  const root = scrollEl.value
  if (!root) return
  isAutoScroll.value = true
  isInstant.value = true
  scrollViewportTo(() => root.scrollHeight)
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

watch(isScrolling, (scrolling) => {
  if (scrolling || lockScroll.value || !isActive.value) return
  for (const item of elId) {
    const el = findMessageElement(item.id)
    if (el) item.top = el.getBoundingClientRect().top - 48
  }
})

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
  return root.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(messageId)}"]`)
}

async function scrollToMessage(messageId: string): Promise<boolean> {
  await nextTick()
  const root = scrollEl.value
  const target = findMessageElement(messageId)
  if (!root || !target) return false
  isAutoScroll.value = false
  isInstant.value = false
  const scrollMargin = Number.parseFloat(getComputedStyle(target).scrollMarginTop) || 0
  startScrollTween(root, () => {
    const el = findMessageElement(messageId)
    return el ? getElementAbsoluteTop(el, root) - scrollMargin : root.scrollTop
  })
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

function ensurePendingUserInputDraft(questionId: string): PendingUserInputDraft {
  let draft = pendingUserInputDrafts.value[questionId]
  if (!draft) {
    draft = { optionIds: [], customSelected: false, customText: '', text: '' }
    pendingUserInputDrafts.value[questionId] = draft
  }
  return draft
}

function isPendingUserInputOptionSelected(questionId: string, optionId: string) {
  return pendingUserInputDrafts.value[questionId]?.optionIds.includes(optionId) ?? false
}

function isPendingUserInputCustomSelected(questionId: string) {
  return pendingUserInputDrafts.value[questionId]?.customSelected ?? false
}

function pendingUserInputOptionIcon(question: UIUserInputQuestion, selected: boolean) {
  if (question.kind === 'multi_select') return selected ? SquareCheck : Square
  return selected ? CircleDot : Circle
}

function togglePendingUserInputOption(question: UIUserInputQuestion, optionId: string) {
  const draft = ensurePendingUserInputDraft(question.id)
  if (question.kind === 'multi_select') {
    draft.optionIds = draft.optionIds.includes(optionId)
      ? draft.optionIds.filter(id => id !== optionId)
      : [...draft.optionIds, optionId]
    return
  }
  draft.optionIds = [optionId]
  draft.customSelected = false
  draft.customText = ''
}

function togglePendingUserInputCustom(question: UIUserInputQuestion) {
  const draft = ensurePendingUserInputDraft(question.id)
  if (question.kind === 'multi_select') {
    draft.customSelected = !draft.customSelected
  } else {
    draft.customSelected = true
    draft.optionIds = []
  }
  if (!draft.customSelected) {
    draft.customText = ''
  }
}

function pendingUserInputDraftText(question: UIUserInputQuestion) {
  const draft = pendingUserInputDrafts.value[question.id]
  if (!draft) return ''
  return question.kind === 'text' ? draft.text : draft.customText
}

function setPendingUserInputDraftText(question: UIUserInputQuestion, value: string) {
  const draft = ensurePendingUserInputDraft(question.id)
  if (question.kind === 'text') {
    draft.text = value
    return
  }
  draft.customText = value
}

function pendingUserInputAnswerFor(question: UIUserInputQuestion): WSUserInputAnswer | null {
  const draft = pendingUserInputDrafts.value[question.id]
  const customText = draft?.customSelected ? draft.customText.trim() : ''
  const text = draft?.text.trim() ?? ''
  if (!draft) return null
  if (question.kind === 'text') {
    return text ? { question_id: question.id, text } : null
  }
  if (draft.customSelected && !customText) return null
  if (question.kind === 'single_select' && draft.optionIds.length + (customText ? 1 : 0) !== 1) return null
  if (draft.optionIds.length === 0 && !customText) return null
  const answer: WSUserInputAnswer = { question_id: question.id }
  if (draft.optionIds.length > 0) answer.option_ids = [...draft.optionIds]
  if (customText) answer.custom_text = customText
  return answer
}

function handlePendingUserInputSubmit() {
  const userInput = pendingUserInput.value
  const answers = pendingUserInputAnswers.value
  if (!userInput || !answers) return
  void chatStore.respondUserInput(userInput, { answers })
}

function handlePendingUserInputCancel() {
  const userInput = pendingUserInput.value
  if (!userInput) return
  void chatStore.respondUserInput(userInput, {
    canceled: true,
    reason: 'user_canceled',
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
