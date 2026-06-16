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
          <div
            aria-hidden="true"
            class="pointer-events-none absolute inset-x-0 top-0 z-10 h-10 bg-gradient-to-b from-surface-editor to-transparent"
          />
          <ScrollArea
            ref="scrollContainer"
            :class="`${transitionScroll?'opacity-100':'opacity-0'} h-full`"
          >
            <div
              class="w-full max-w-[840px] mx-auto px-10 pt-6 pb-28 space-y-6"
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
                v-for="(msg, index) in messages"
                :key="msg.id"
                :data-message-id="msg.id"
                :data-external-message-id="(msg.role === 'user' || msg.role === 'assistant') ? msg.externalMessageId : undefined"
                class="transition-[background-color] duration-500 scroll-mt-2 px-2 -mx-2 [content-visibility:auto] [contain-intrinsic-size:auto_600px]"
                :class="highlightedMessageId === msg.id ? 'bg-muted/45' : ''"
                :data-anchor="msg.id"
              >
                <MessageItem
                  :message="msg"
                  :session-type="activeSession?.type"
                  :bot-id="currentBotId"
                  :channel-thread="isChannelThread"
                  :channel-platform="channelPlatform"
                  :bot-name="currentBot?.name"
                  :bot-avatar-url="currentBot?.avatar_url"
                  :on-open-media="galleryOpenBySrc"
                  :on-reply-click="handleReplyJump"
                  :is-scrolling="isScrolling"
                  :is-last-message="index === messages.length - 1"
                  @active="isActiveEl"
                />
              </div>
            </div>
          </ScrollArea>

          <div
            v-if="showScrollRail"
            class="group/rail hidden md:flex absolute inset-y-0 right-4 z-10 w-96 flex-col items-end justify-center pointer-events-none"
            @mouseenter="scheduleRailOpen"
            @mouseleave="scheduleRailClose"
          >
            <!-- Collapsed: uniform tick marks -->
            <div
              class="flex max-h-[60vh] flex-col items-end justify-center gap-2 py-2 pointer-events-auto transition-opacity duration-150"
              :class="railOpen ? 'opacity-0 pointer-events-none' : 'opacity-100'"
            >
              <span
                v-for="seg in railSegments"
                :key="seg.id"
                class="h-0.5 w-4 shrink-0 rounded-full transition-colors duration-150"
                :class="seg.id === activeRailId
                  ? 'bg-foreground/70'
                  : 'bg-muted-foreground/30 group-hover/rail:bg-muted-foreground/55'"
              />
            </div>

            <!-- Expanded: user-prompt select panel -->
            <div
              v-if="railOpen"
              class="absolute right-0 top-1/2 w-80 -translate-y-1/2 overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg pointer-events-auto"
              @mouseenter="scheduleRailOpen"
              @mouseleave="scheduleRailClose"
            >
              <div
                class="max-h-[min(60vh,480px)] overflow-y-auto overscroll-contain p-1.5 outline-none [mask-image:linear-gradient(to_bottom,transparent,black_10px,black_calc(100%-10px),transparent)] scrollbar-none"
              >
                <button
                  v-for="seg in railSegments"
                  :key="seg.id"
                  type="button"
                  class="flex h-8 w-full items-center rounded-md px-3 text-left text-[13px] text-foreground hover:bg-[var(--overlay-hover)]"
                  @click="scrollToRailSegment(seg)"
                >
                  <span class="truncate">{{ seg.preview }}</span>
                </button>
              </div>
            </div>
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
        class="pointer-events-none absolute inset-x-0 bottom-0 z-30 px-3 sm:px-5 lg:px-8 pt-2 pb-7"
      >
        <div
          aria-hidden="true"
          class="absolute inset-x-0 bottom-0 h-7 bg-surface-editor"
        />
        <div class="pointer-events-auto relative w-full max-w-[840px] mx-auto px-10">
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
              size="icon-sm"
              variant="secondary"
              class="absolute left-1/2 bottom-full z-20 mb-2 size-8 -translate-x-1/2 rounded-full"
              aria-label="Scroll to latest message"
              @click="scrollToBottom"
            >
              <ArrowDown class="size-4" />
            </Button>
          </Transition>

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
            <!--
              Compact uses a CONCRETE 28px radius (= half the compact height:
              button 36px + py-2.5 ×2 = 56px), so a short composer still reads as
              a perfect pill — but, unlike rounded-full (9999px), the value can be
              animated. Multiline shrinks the corners to 20px; transitioning
              between two concrete radii interpolates smoothly, whereas animating
              out of 9999px snapped mid-way (the value stayed clamped-round until
              it crossed half-height, then jumped the corner in one step).
            -->
            <div
              ref="composerEl"
              data-slot="input-group"
              role="group"
              class="relative flex w-full flex-wrap items-center gap-1 bg-surface-composer px-2.5 py-2.5 transition-[border-radius] duration-[220ms] ease-[cubic-bezier(0.33,1,0.68,1)] motion-reduce:transition-none"
              :class="(isMultiline || pendingPreviews.length) ? 'rounded-[20px]' : 'rounded-[28px]'"
              :style="{ '--field-edge-solid': 'var(--field-edge-engaged)' }"
              @click.self="focusTextarea"
            >
              <div
                v-if="pendingPreviews.length"
                class="order-first flex w-full basis-full flex-wrap gap-2 pb-1.5"
              >
                <ChatAttachmentCard
                  v-for="preview in pendingPreviews"
                  :key="preview.i"
                  :kind="preview.isMedia ? 'media' : 'file'"
                  :src="preview.url"
                  :video="preview.isVideo"
                  :name="preview.file.name"
                  :ext="preview.ext"
                  removable
                  @remove="pendingFiles.splice(preview.i, 1)"
                />
              </div>

              <textarea
                ref="textareaEl"
                v-model="inputText"
                rows="1"
                :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
                :disabled="!currentBotId || activeChatReadOnly"
                class="field-sizing-content resize-none break-words bg-transparent text-sm leading-6 text-foreground outline-none placeholder:text-[var(--field-placeholder)] disabled:cursor-not-allowed"
                :class="isMultiline
                  ? 'order-none w-full basis-full pl-2 pr-1 pt-2 pb-1.5 max-h-52'
                  : 'order-2 min-w-0 flex-1 self-center pl-1 pr-1 py-1 max-h-32'"
                @keydown.enter.exact="handleKeydown"
                @paste="handlePaste"
                @input="syncMultiline"
              />

              <DropdownMenu v-model:open="agentPopoverOpen">
                <DropdownMenuTrigger as-child>
                  <Button
                    type="button"
                    variant="ghost"
                    :disabled="!currentBotId || activeChatReadOnly || agentChanging"
                    :title="$t('chat.composerActions')"
                    class="order-1 size-9 rounded-full text-foreground/85"
                    :class="isMultiline ? 'self-end' : 'self-center'"
                    :aria-label="$t('chat.composerActions')"
                  >
                    <LoaderCircle
                      v-if="agentChanging"
                      class="size-4 animate-spin"
                    />
                    <Plus
                      v-else
                      class="size-[22px]"
                      :stroke-width="1.75"
                    />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  class="w-56"
                  align="start"
                  side="top"
                >
                  <template v-if="enabledACPProfiles.length">
                    <DropdownMenuLabel>{{ $t('chat.agent') }}</DropdownMenuLabel>
                    <DropdownMenuItem
                      :disabled="!canChangeAgent"
                      @select="selectMemohAgent"
                    >
                      <img
                        src="/logo.svg"
                        alt=""
                        class="size-4 shrink-0"
                      >
                      <span class="min-w-0 flex-1 truncate">{{ $t('chat.agentMemoh') }}</span>
                      <Check
                        v-if="!activeIsACP"
                        class="ml-auto"
                      />
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      v-for="profile in enabledACPProfiles"
                      :key="profile.id"
                      :disabled="!canChangeAgent"
                      @select="selectACPAgent(profile)"
                    >
                      <component :is="acpAgentIcon(profile.id, true)" />
                      <span class="min-w-0 flex-1 truncate">{{ profile.display_name || profile.id }}</span>
                      <Check
                        v-if="activeACPAgentId === normalizedProfileID(profile.id)"
                        class="ml-auto"
                      />
                    </DropdownMenuItem>
                  </template>
                  <template v-if="!activeIsACP">
                    <DropdownMenuSeparator v-if="enabledACPProfiles.length" />
                    <DropdownMenuItem
                      :disabled="!currentBotId || activeChatReadOnly || streaming"
                      @select="fileInput?.click()"
                    >
                      <Paperclip />
                      <span class="min-w-0 flex-1 truncate">{{ $t('chat.attachFiles') }}</span>
                    </DropdownMenuItem>
                  </template>
                </DropdownMenuContent>
              </DropdownMenu>

              <div
                class="order-3 ml-auto flex min-w-0 items-center gap-0.5"
                :class="isMultiline ? 'self-end' : 'self-center'"
              >
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
                      <Lightbulb
                        v-else-if="reasoningActive"
                        class="size-3.5 shrink-0"
                        :style="{ opacity: reasoningTriggerOpacity }"
                      />
                      <span class="truncate text-[11px]">{{ modelTriggerLabel }}</span>
                      <ChevronDown class="size-3 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    class="w-72 max-h-[60vh] overflow-y-auto p-0"
                    align="end"
                    side="top"
                    :side-offset="4"
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
                    <div v-else>
                      <ChatModelPicker
                        v-model="overrideModelId"
                        v-model:reasoning-effort="overrideReasoningEffort"
                        :models="models"
                        :providers="providers"
                        model-type="chat"
                        :open="modelPopoverOpen"
                        @update:model-value="onModelSelected"
                      />
                    </div>
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

                <div class="relative size-9 shrink-0">
                  <SessionInfoRing
                    v-if="!activeIsACP"
                    :override-model-id="overrideModelId"
                    class="absolute inset-0 size-9 transition-[opacity,scale] duration-200 ease-out motion-reduce:transition-none"
                    :class="(!showSend && !streaming) ? 'scale-100 opacity-100' : 'pointer-events-none scale-75 opacity-0'"
                  />
                  <Button
                    v-if="!streaming"
                    type="button"
                    variant="brand"
                    :disabled="!showSend || !currentBotId || activeChatReadOnly"
                    aria-label="Send message"
                    class="absolute inset-0 size-9 rounded-full transition-[opacity,scale] duration-200 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none"
                    :class="showSend ? 'scale-100 opacity-100' : 'pointer-events-none scale-0 opacity-0'"
                    @click="handleSend"
                  >
                    <svg
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.25"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      class="size-[18px]"
                      aria-hidden="true"
                    >
                      <path d="M12 19.5 V5" />
                      <path d="M6 10.5 L12 4.5 L18 10.5" />
                    </svg>
                  </Button>
                  <Button
                    v-else
                    type="button"
                    variant="destructive"
                    class="absolute inset-0 size-9 rounded-full"
                    aria-label="Stop generating response"
                    @click="chatStore.abort()"
                  >
                    <LoaderCircle class="size-[18px] animate-spin" />
                  </Button>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, onMounted, useTemplateRef, watchEffect, watch, nextTick, onActivated, onDeactivated } from 'vue'
import {
  LoaderCircle,
  Paperclip,
  Plus,
  ChevronDown,
  Lightbulb,
  CircleAlert,
  ArrowDown,
  Check,
  FolderOpen,
  Square,
  SquareCheck,
  Circle,
  CircleDot,
} from 'lucide-vue-next'
import { ScrollArea, Button, Popover, PopoverContent, PopoverTrigger, DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuItem, DropdownMenuSeparator } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useScroll, useElementBounding, useIntersectionObserver, useStorage } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getAcpProfiles, getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { AcpclientModelInfo, AcpprofilePublicProfile, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import MessageItem from './message-item.vue'
import ChatAttachmentCard from './chat-attachment-card.vue'
import { animateScrollTo } from './chat-minimap'
import BgTaskPill from './bg-task-pill.vue'
import { provideBgTaskBeacons } from '../composables/useBgTaskBeacons'
import MediaGalleryLightbox from './media-gallery-lightbox.vue'
import SessionInfoRing from './session-info-ring.vue'
import ChatModelPicker from './chat-model-picker.vue'
import { EFFORT_LABELS, EFFORT_OPACITY, REASONING_EFFORT_DISABLE, availableEffortsForMode, resolveEffortLevels, resolveThinkingMode } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import type { ChatAttachment, UIUserInput, UIUserInputQuestion, WSUserInputAnswer } from '@/composables/api/useChat'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { useACPRuntime } from '@/composables/useACPRuntime'
import { acpAgentIcon, isACPAgentEnabled, isACPNoProject, normalizeACPAgentID } from '@/utils/acp'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface PendingUserInputDraft {
  optionIds: string[]
  customSelected: boolean
  customText: string
  text: string
}

interface ScrollRailSegment {
  id: string
  label: string
  preview: string
  index: number
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

// Object-URL previews for pending image/video attachments, keyed by File so a
// URL is created once and revoked the moment its file leaves the tray (or the
// composer unmounts) — no leaks across sends or session switches.
const pendingPreviewUrls = ref(new Map<File, string>())
function syncPendingPreviewUrls(files: File[]) {
  const map = pendingPreviewUrls.value
  for (const [file, url] of map) {
    if (!files.includes(file)) {
      URL.revokeObjectURL(url)
      map.delete(file)
    }
  }
  for (const file of files) {
    if (map.has(file)) continue
    if (file.type.startsWith('image/') || file.type.startsWith('video/')) {
      map.set(file, URL.createObjectURL(file))
    }
  }
}
watch(pendingFiles, files => syncPendingPreviewUrls(files), { deep: true, immediate: true })
onBeforeUnmount(() => {
  for (const url of pendingPreviewUrls.value.values()) URL.revokeObjectURL(url)
  pendingPreviewUrls.value.clear()
})

const pendingPreviews = computed(() =>
  pendingFiles.value.map((file, i) => {
    const isImage = file.type.startsWith('image/')
    const isVideo = file.type.startsWith('video/')
    const dot = file.name.lastIndexOf('.')
    return {
      i,
      file,
      isMedia: isImage || isVideo,
      isVideo,
      url: pendingPreviewUrls.value.get(file) ?? '',
      ext: dot > 0 ? file.name.slice(dot + 1).toUpperCase() : '',
    }
  }),
)

const composerError = ref('')
const pendingUserInputDrafts = ref<Record<string, PendingUserInputDraft>>({})
const modelPopoverOpen = ref(false)
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

// A third-party synced thread (Telegram/Discord/...) is a multi-participant
// group conversation rather than the local 1:1 chat. The message list switches
// to a group layout for these: every turn is left-aligned with an avatar +
// sender name + channel badge, including the bot's own replies.
const channelPlatform = computed(() => (activeSession.value?.channel_type ?? '').trim().toLowerCase())
const isChannelThread = computed(() => !!channelPlatform.value && channelPlatform.value !== 'local')

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

const activeThinkingMode = computed(() => resolveThinkingMode(activeModel.value?.config))

const activeModelSupportsReasoning = computed(() => activeThinkingMode.value !== 'none')

const activeModelClientType = computed(() =>
  providers.value.find((p) => p.id === activeModel.value?.provider_id)?.client_type,
)

const availableReasoningEfforts = computed(() =>
  availableEffortsForMode(activeThinkingMode.value, resolveEffortLevels(activeModel.value?.config, activeModelClientType.value)),
)

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

const reasoningActive = computed(() =>
  !activeIsACP.value
  && !activeIsPendingACP.value
  && activeModelSupportsReasoning.value
  && Boolean(overrideReasoningEffort.value)
  && overrideReasoningEffort.value !== REASONING_EFFORT_DISABLE,
)

const modelTriggerLabel = computed(() =>
  reasoningActive.value
    ? `${selectedModelLabel.value} · ${selectedReasoningLabel.value}`
    : selectedModelLabel.value,
)

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

watch(availableReasoningEfforts, (efforts) => {
  const current = overrideReasoningEffort.value
  if (!current || current === REASONING_EFFORT_DISABLE || efforts.includes(current)) return
  overrideReasoningEffort.value = efforts.includes('medium') ? 'medium' : efforts[0] ?? REASONING_EFFORT_DISABLE
}, { immediate: true })

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

const {
  items: galleryItems,
  openIndex: galleryOpenIndex,
  setOpenIndex: gallerySetOpenIndex,
  openBySrc: galleryOpenBySrc,
} = useMediaGallery(messages)

const inputText = ref('')
const textareaEl = ref<HTMLTextAreaElement | null>(null)
const composerEl = ref<HTMLElement | null>(null)
const isMultiline = ref(false)
const compactContentWidth = ref(0)
const showSend = computed(() => Boolean(inputText.value.trim()) || pendingFiles.value.length > 0)

function focusTextarea() {
  textareaEl.value?.focus()
}

function measureWraps(text: string, width: number): boolean {
  const el = textareaEl.value
  if (!el || width <= 1) return false
  const cs = getComputedStyle(el)
  const mirror = document.createElement('div')
  const s = mirror.style
  s.position = 'fixed'
  s.left = '-9999px'
  s.top = '0'
  s.visibility = 'hidden'
  s.pointerEvents = 'none'
  s.whiteSpace = 'pre-wrap'
  s.overflowWrap = 'anywhere'
  s.wordBreak = 'break-word'
  s.boxSizing = 'content-box'
  s.width = `${width}px`
  s.fontFamily = cs.fontFamily
  s.fontSize = cs.fontSize
  s.fontWeight = cs.fontWeight
  s.fontStyle = cs.fontStyle
  s.letterSpacing = cs.letterSpacing
  s.lineHeight = cs.lineHeight
  mirror.textContent = text.length ? text : 'x'
  document.body.appendChild(mirror)
  const h = mirror.getBoundingClientRect().height
  mirror.remove()
  const lh = Number.parseFloat(cs.lineHeight) || 20
  return h > lh * 1.5
}

function syncMultiline() {
  const text = inputText.value
  if (text.includes('\n')) {
    isMultiline.value = true
    return
  }
  const el = textareaEl.value
  if (el && !isMultiline.value) {
    const cs = getComputedStyle(el)
    const padX = Number.parseFloat(cs.paddingLeft) + Number.parseFloat(cs.paddingRight)
    const w = el.clientWidth - padX
    if (w > 1) compactContentWidth.value = w
  }
  isMultiline.value = measureWraps(text, compactContentWidth.value)
}

let composerResizeObserver: ResizeObserver | null = null
onMounted(() => {
  void nextTick(syncMultiline)
  if (typeof ResizeObserver !== 'undefined' && textareaEl.value) {
    composerResizeObserver = new ResizeObserver(() => syncMultiline())
    composerResizeObserver.observe(textareaEl.value)
  }
})
onBeforeUnmount(() => {
  composerResizeObserver?.disconnect()
  composerResizeObserver = null
})
watch(inputText, () => {
  void nextTick(syncMultiline)
})

// Smooth height morph for the compact↔multiline change. The composer is
// bottom-anchored, so animating its height makes the top edge rise and the text
// appears to slide up. Pure CSS can't transition between two content-driven
// (auto) heights, so we measure the natural height and let the browser's
// animation engine fill the gap — no permanent inline height, no fight with the
// textarea's field-sizing. During the morph the box is clipped and its content is
// bottom-pinned: the left (＋) and right (model) controls stay welded to the
// bottom edge — which never moves — so they don't twitch, while the textarea
// grows above them and the text is revealed from the top.
let composerHeight = 0
let composerHeightAnim: Animation | null = null
let composerHeightReady = false
// Last-seen layout mode, so we can tell a pill↔multiline form change from a
// grow/shrink that happens entirely within multiline.
let composerMultiline = false
// A session/draft switch replaces the text wholesale — snap to the new size
// once instead of animating between two unrelated drafts.
let composerSnapNext = false
// Tracks layout-driven height changes (e.g. window/pane resize re-wrapping a
// multiline draft) that don't go through inputText/isMultiline, so the next
// morph starts from the real current height instead of a stale baseline.
let composerSizeObserver: ResizeObserver | null = null

function prefersReducedMotion() {
  return typeof window !== 'undefined'
    && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true
}

// Bottom-pin the controls directly: the compact row carries `self-center`
// (align-self) on each control, which would override a container-level
// align-items and let the ＋ jump to center mid-shrink. Overriding each child's
// align-self welds the controls to the bottom in both directions. The textarea
// is skipped on purpose — it stays centered in the compact row, so it slides
// smoothly instead of snapping from bottom-pinned back to centered when the
// morph ends (which made the placeholder jump on shrink).
function pinComposerChildrenBottom(el: HTMLElement, pinned: boolean) {
  const value = pinned ? 'flex-end' : ''
  for (const child of Array.from(el.children)) {
    if (child instanceof HTMLElement && child.tagName !== 'TEXTAREA') {
      child.style.alignSelf = value
    }
  }
}

function clearComposerMorphStyles(el: HTMLElement) {
  el.style.overflow = ''
  el.style.alignContent = ''
  pinComposerChildrenBottom(el, false)
}

function animateComposerHeight() {
  const el = composerEl.value
  if (!el) return
  // Start from the live visual height when a morph is already running, so a
  // fresh trigger continues from where the eye is instead of snapping back.
  const from = composerHeightAnim ? el.offsetHeight : composerHeight
  composerHeightAnim?.cancel()
  composerHeightAnim = null
  clearComposerMorphStyles(el)
  const target = el.offsetHeight
  composerHeight = target
  // A pill↔multiline form change is a deliberate transition; growing/shrinking
  // within multiline (adding a line) is a small nudge that must keep up with the
  // already-placed text, so the latter runs much faster.
  const formChanged = isMultiline.value !== composerMultiline
  composerMultiline = isMultiline.value
  if (!composerHeightReady || composerSnapNext) {
    composerSnapNext = false
    return
  }
  if (!isActive.value || !from || Math.abs(target - from) < 0.5 || prefersReducedMotion()) return
  // Pin every line to the bottom and clip the overflow: the control row stays
  // welded to the fixed bottom edge (no twitch) while the box grows/shrinks and
  // the textarea is revealed/concealed from the top.
  el.style.overflow = 'hidden'
  el.style.alignContent = 'flex-end'
  pinComposerChildrenBottom(el, true)
  // Form change: a pure ease-out (no lead-in) starts moving immediately and
  // completes promptly, but the tail decelerates to a gentle stop so the finish
  // visibly slows down rather than snapping. In-multiline grow stays short and
  // snappy so the box keeps up with the already-placed text.
  const anim = el.animate(
    [{ height: `${from}px` }, { height: `${target}px` }],
    {
      duration: formChanged ? 220 : 110,
      easing: formChanged ? 'cubic-bezier(0.33, 1, 0.68, 1)' : 'cubic-bezier(0.16, 1, 0.3, 1)',
    },
  )
  composerHeightAnim = anim
  anim.onfinish = () => {
    if (composerHeightAnim === anim) {
      clearComposerMorphStyles(el)
      composerHeightAnim = null
    }
  }
}

watch([inputText, isMultiline, () => pendingFiles.value.length], () => {
  void nextTick(animateComposerHeight)
})

onMounted(() => {
  void nextTick(() => {
    composerHeight = composerEl.value?.offsetHeight ?? 0
    composerMultiline = isMultiline.value
    composerHeightReady = true
    composerSnapNext = false
  })
  const el = composerEl.value
  if (el && typeof ResizeObserver !== 'undefined') {
    composerSizeObserver = new ResizeObserver(() => {
      // Skip while we drive the height ourselves; only capture layout-driven
      // resizes so the next morph starts from the real current height. The
      // keystroke path sets composerHeightAnim before this fires, so normal
      // morphs are untouched.
      if (!composerHeightAnim) composerHeight = el.offsetHeight
    })
    composerSizeObserver.observe(el)
  }
})

onBeforeUnmount(() => {
  composerSizeObserver?.disconnect()
  composerSizeObserver = null
  composerHeightAnim?.cancel()
  composerHeightAnim = null
})

onDeactivated(() => {
  composerHeightAnim?.cancel()
  composerHeightAnim = null
  if (composerEl.value) clearComposerMorphStyles(composerEl.value)
  composerSnapNext = true
})

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
  composerSnapNext = true
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

// --- Scroll rail ---
const railSegments = ref<ScrollRailSegment[]>([])
const activeRailId = ref('')
const railOpen = ref(false)
let railRaf = 0
let railOpenTimer: ReturnType<typeof setTimeout> | null = null
let railCloseTimer: ReturnType<typeof setTimeout> | null = null

function getRailSegmentText(msg: (typeof messages.value)[number]): string {
  if (msg.role === 'user') return msg.text?.trim().replace(/\s+/g, ' ') || ''
  return ''
}

function rebuildRailSegments() {
  const segments: ScrollRailSegment[] = []
  messages.value.forEach((msg) => {
    if (msg.role !== 'user') return
    const preview = getRailSegmentText(msg)
    if (!preview) return
    segments.push({
      id: msg.id,
      label: `Message ${segments.length + 1}`,
      preview,
      index: segments.length,
    })
  })
  railSegments.value = segments
}

function syncActiveRailFromScroll() {
  const root = scrollEl.value
  if (!root || !railSegments.value.length) return
  const viewAnchor = root.scrollTop + 8
  let best = railSegments.value[0]!.id
  let bestDist = Number.POSITIVE_INFINITY
  for (const seg of railSegments.value) {
    const el = root.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(seg.id)}"]`)
    if (!el) continue
    const top = root.scrollTop + el.getBoundingClientRect().top - root.getBoundingClientRect().top
    const dist = Math.abs(top - viewAnchor)
    if (dist < bestDist) { bestDist = dist; best = seg.id }
  }
  activeRailId.value = best
}

watch(() => messages.value.map(m => `${m.id}:${m.role}`).join('|'), () => {
  rebuildRailSegments()
}, { flush: 'post', immediate: true })

useScroll(scrollEl, {
  onScroll() {
    if (railRaf) return
    railRaf = requestAnimationFrame(() => {
      railRaf = 0
      syncActiveRailFromScroll()
    })
  },
})

function scheduleRailOpen() {
  if (railCloseTimer) { clearTimeout(railCloseTimer); railCloseTimer = null }
  if (railOpen.value || railOpenTimer) return
  railOpenTimer = setTimeout(() => { railOpen.value = true; railOpenTimer = null }, 80)
}

function scheduleRailClose() {
  if (railOpenTimer) { clearTimeout(railOpenTimer); railOpenTimer = null }
  if (!railOpen.value || railCloseTimer) return
  railCloseTimer = setTimeout(() => { railOpen.value = false; railCloseTimer = null }, 150)
}

const showScrollRail = computed(() =>
  isActive.value && !loadingChats.value && railSegments.value.length > 1,
)

function scrollToRailSegment(seg: ScrollRailSegment) {
  activeRailId.value = seg.id
  railOpen.value = false
  void nextTick(() => {
    const root = scrollEl.value
    const target = findMessageElement(seg.id)
    if (!root || !target) return
    isAutoScroll.value = false
    isInstant.value = false
    const scrollMargin = Number.parseFloat(getComputedStyle(target).scrollMarginTop) || 0
    startScrollTween(root, () => {
      const el = findMessageElement(seg.id)
      return el ? getElementAbsoluteTop(el, root) - scrollMargin : root.scrollTop
    })
  })
}
// --- End scroll rail ---

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
      } else {
        transitionScroll.value=true
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
  let handledFile = false
  for (const item of Array.from(items)) {
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file) {
        pendingFiles.value.push(file)
        handledFile = true
      }
    }
  }
  // A file paste from the OS also carries a text item (its name); without this
  // the textarea would insert that filename alongside the attachment card.
  if (handledFile) e.preventDefault()
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
