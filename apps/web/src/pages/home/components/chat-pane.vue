<template>
  <div class="flex-1 flex flex-col h-full min-w-0 relative">
    <div
      v-if="!currentBotId"
      class="flex-1"
    >
      <PanePlaceholder :title="$t('chat.selectBot')">
        {{ $t('chat.selectBotHint') }}
      </PanePlaceholder>
    </div>

    <template v-else>
      <section class="flex-1 relative w-full px-3 sm:px-5 lg:px-8">
        <section class="absolute inset-0">
          <div
            aria-hidden="true"
            class="pointer-events-none absolute inset-x-0 top-0 z-(--z-raised) h-10 bg-gradient-to-b from-surface-editor to-transparent"
          />
          <ScrollArea
            ref="scrollContainer"
            class="h-full"
          >
            <!-- Same horizontal rhythm as the composer below (px-4 sm:px-6
                 lg:px-10) so the input box and the message column share one
                 width at every pane size — they must never diverge. -->
            <div
              class="w-full max-w-[840px] mx-auto px-4 pt-6 pb-28 space-y-6 sm:px-6 lg:px-10"
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
                <Spinner class="size-3.5" />
              </div>

              <!-- Read-only sessions (subagent / system / synced channel
                   threads) can't take new input, so an empty one states why it
                   has nothing. A fresh, writable chat instead gets the centered
                   welcome composer below, never a stray line in a blank pane. -->
              <div
                v-if="messages.length === 0 && !loadingChats && !loadingMessages && activeChatReadOnly"
                class="flex items-center justify-center min-h-75"
              >
                <p
                  v-if="activeSession?.type === 'subagent'"
                  class="text-muted-foreground text-xs"
                >
                  {{ $t('chat.emptySubagent') }}
                </p>
                <p
                  v-else
                  class="text-muted-foreground text-xs"
                >
                  {{ $t('chat.emptySystemSession') }}
                </p>
              </div>

              <!-- One persistent container per turn, keyed by the turn's
                   opening message id — a send APPENDS a container; previous
                   turns' DOM is never re-parented (see messageTurns for why
                   that is load-bearing). The send pin reserves viewport space
                   declaratively: turnReserveStyle projects a min-height for
                   the turn ids that currently hold a reserve (see
                   useChatScroll's Pin section). -->
              <div
                v-for="(turn, turnIndex) in messageTurns"
                :key="turn.id"
                :ref="turnIndex === messageTurns.length - 1 ? setLastTurnEl : undefined"
                class="space-y-6"
                :style="turnReserveStyle(turn.id)"
              >
                <template
                  v-for="(msg, msgIndex) in turn.messages"
                  :key="msg.id"
                >
                  <ForkSourceDivider
                    v-if="showForkSourceDividerBefore(turn.start + msgIndex)"
                    :title="forkSourceTitle"
                    :disabled="openingForkSource"
                    @open-source="handleForkSourceClick"
                  />

                  <div
                    :data-message-id="msg.id"
                    :data-external-message-id="(msg.role === 'user' || msg.role === 'assistant') ? msg.externalMessageId : undefined"
                    class="transition-[background-color] duration-500 scroll-mt-2 px-2 -mx-2"
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
                      :on-retry-message="handleRetryMessage"
                      :can-retry-latest-assistant="latestRetryableAssistantId === ((msg.serverId ?? msg.id).trim())"
                      :can-edit-latest-user="latestEditableUserId === ((msg.serverId ?? msg.id).trim())"
                      :can-fork-assistant="canForkAssistant"
                      :is-scrolling="isScrolling"
                      :is-last-message="msg.id === lastMessageId"
                      @active="onMessageActive"
                      @edit-message="handleEditMessage"
                      @fork-message="handleForkMessage"
                    />
                  </div>

                  <ForkSourceDivider
                    v-if="showForkSourceDividerAfter(msg, turn.start + msgIndex)"
                    :title="forkSourceTitle"
                    :disabled="openingForkSource"
                    @open-source="handleForkSourceClick"
                  />
                </template>
              </div>
            </div>
          </ScrollArea>

          <div
            v-if="showScrollRail"
            class="group/rail hidden md:flex absolute inset-y-0 right-4 z-(--z-raised) w-96 flex-col items-end justify-center pointer-events-none"
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

      <MediaGalleryLightbox
        :items="composerPreviewItems"
        :open-index="composerPreviewIndex"
        appearance="frost"
        @update:open-index="composerPreviewIndex = $event"
      />

      <Dialog v-model:open="pastedViewerOpen">
        <DialogContent class="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{{ $t('chat.pastedViewerTitle') }}</DialogTitle>
          </DialogHeader>
          <pre class="max-h-[60vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-surface-composer p-3 text-caption leading-relaxed text-foreground">{{ pastedViewerText }}</pre>
        </DialogContent>
      </Dialog>

      <Dialog v-model:open="forkDialogOpen">
        <DialogContent class="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{{ $t('chat.forkDialog.title') }}</DialogTitle>
            <DialogDescription>{{ $t('chat.forkDialog.description') }}</DialogDescription>
          </DialogHeader>
          <form
            class="space-y-4"
            @submit.prevent="handleCreateFork"
          >
            <Input
              v-model="forkSessionTitle"
              :aria-label="$t('chat.forkDialog.namePlaceholder')"
              :placeholder="$t('chat.forkDialog.namePlaceholder')"
              :disabled="forkSubmitting"
              maxlength="120"
              autofocus
            />
            <DialogFooter>
              <Button
                type="submit"
                :disabled="!forkSessionTitle.trim() || forkSubmitting"
              >
                <LoaderCircle
                  v-if="forkSubmitting"
                  class="mr-1 size-3 animate-spin"
                />
                {{ $t('common.create') }}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <!-- The composer is a single instance reused in both layouts: pinned to
           the bottom once a conversation exists, or lifted to the vertical
           centre (with a greeting above it) while the chat is still empty, so a
           fresh chat opens on an inviting page instead of a near-blank pane. -->
      <!-- No outer horizontal gutter here on purpose: the message list lives in a
           `section.absolute.inset-0` layer that fills its parent's padding box, so
           it bypasses the section's px-3/sm:px-5/lg:px-8 gutter and only carries the
           inner px-4/sm:px-6/lg:px-10. The composer must drop the same outer gutter
           so its inner padding is the ONLY horizontal inset — matching the message
           column edge-for-edge at every width. -->
      <div
        v-if="!activeChatReadOnly"
        class="pointer-events-none absolute z-(--z-panel)"
        :class="isWelcome
          ? 'inset-0 flex flex-col items-center justify-start pt-[38dvh]'
          : 'inset-x-0 bottom-0 pt-2 pb-8'"
      >
        <!-- Opaque backdrop, bottom-anchored, rising only to the box's widest point
             (its vertical centre). The box is solid and sits above the messages, so
             above that line its rounded top simply floats over whatever is there —
             that area is left unmasked on purpose. From the widest point down (where
             the box curves back in and would leave gaps by its bottom corners, plus
             the strip beneath it) this fill hides everything, so nothing bleeds out
             below the box. No fade: the top edge meets the box where it is already
             full width, so the seam is hidden behind the box itself. -->
        <div
          v-if="!isWelcome"
          aria-hidden="true"
          class="absolute inset-x-0 bottom-0 bg-surface-editor"
          :style="{ height: composerMaskHeight }"
        />
        <!-- welcome: top-anchored column — the greeting and the composer's top
             edge stay pinned at pt-[38dvh], so a growing composer (multiline
             text or attachments) only extends downward and never pushes the
             greeting up; normal: display:contents removes this from layout. -->
        <div :class="isWelcome ? 'flex flex-col items-center gap-10 w-full' : 'contents'">
          <div
            v-if="isWelcome"
            class="w-full max-w-[840px] mx-auto px-4 text-center sm:px-6 lg:px-10"
          >
            <h1 class="text-balance text-2xl font-semibold tracking-tight text-foreground">
              {{ welcomeGreeting }}
            </h1>
          </div>
          <!-- Mirror the message column's padding (px-4 sm:px-6 lg:px-10) exactly
               so the composer and the chat body always share one width — the inner
               gutter still relaxes on a cramped pane, but both edges move together. -->
          <div class="pointer-events-auto relative w-full max-w-[840px] mx-auto px-4 sm:px-6 lg:px-10">
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
                class="absolute left-0 bottom-full z-(--z-sticky) mb-2 max-w-[calc(50%-2rem)]"
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
                variant="ghost"
                class="absolute left-1/2 bottom-full z-(--z-sticky) mb-4 size-9 -translate-x-1/2 rounded-full border border-border bg-card text-foreground"
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
            <Transition
              enter-active-class="transition-opacity duration-150 ease-out"
              enter-from-class="opacity-0"
              enter-to-class="opacity-100"
              leave-active-class="transition-opacity duration-100 ease-in"
              leave-from-class="opacity-100"
              leave-to-class="opacity-0"
            >
              <Command
                v-if="slashPanelOpen"
                class="absolute inset-x-4 bottom-full z-(--z-panel) mb-2 h-auto w-auto"
              >
                <CommandKeyBridge ref="slashPickerBridge">
                  <CommandList class="max-h-[min(20rem,45dvh)] overscroll-contain [scrollbar-gutter:stable]">
                    <CommandGroup
                      v-if="visibleSlashQuickActions.length"
                      :heading="$t('chat.slash.quickActions')"
                    >
                      <CommandItem
                        v-for="action in visibleSlashQuickActions"
                        :key="action.id"
                        :value="action.label"
                        @select="selectSlashQuickAction(action)"
                      >
                        <component
                          :is="action.icon"
                          class="size-4 shrink-0 text-muted-foreground"
                        />
                        <span class="min-w-0 flex-1">
                          <span class="block truncate text-control">{{ action.label }}</span>
                          <span class="block truncate text-caption text-muted-foreground">{{ action.description }}</span>
                        </span>
                      </CommandItem>
                    </CommandGroup>
                    <CommandSeparator v-if="visibleSlashQuickActions.length && visibleSlashSkills.length" />
                    <CommandGroup
                      v-if="visibleSlashSkills.length"
                      :heading="$t('chat.slash.skills')"
                    >
                      <CommandItem
                        v-for="skill in visibleSlashSkills"
                        :key="skill.name"
                        :value="skill.name"
                        @select="addRequestedSkill(skill)"
                      >
                        <Sparkles class="size-4 shrink-0 text-muted-foreground" />
                        <span class="min-w-0 flex-1">
                          <span class="block truncate text-control">{{ skill.display_name || skill.name }}</span>
                          <span
                            v-if="skill.description"
                            class="block truncate text-caption text-muted-foreground"
                          >{{ skill.description }}</span>
                        </span>
                      </CommandItem>
                    </CommandGroup>
                    <div
                      v-if="safeSkillCatalogLoading"
                      class="py-6 text-center text-body text-muted-foreground"
                    >
                      {{ $t('chat.slash.loadingSkills') }}
                    </div>
                    <div
                      v-else-if="!slashPanelHasResults"
                      class="py-6 text-center text-body text-muted-foreground"
                    >
                      {{ $t('chat.slash.noResults') }}
                    </div>
                  </CommandList>
                </CommandKeyBridge>
              </Command>
            </Transition>
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
                v-if="commandPanelEvent"
                class="mb-2 overflow-hidden rounded-lg border bg-card"
                :class="commandPanelIsError ? 'border-destructive/30' : 'border-border'"
              >
                <div class="flex items-start gap-2 px-3 py-2">
                  <CircleAlert
                    v-if="commandPanelIsError"
                    class="mt-0.5 size-4 shrink-0 text-destructive"
                  />
                  <div class="min-w-0 flex-1">
                    <p
                      class="truncate text-label font-medium"
                      :class="commandPanelIsError ? 'text-destructive' : 'text-foreground'"
                    >
                      {{ commandPanelTitle }}
                    </p>
                    <p
                      v-if="commandPanelText"
                      class="mt-0.5 whitespace-pre-wrap break-words text-body text-muted-foreground"
                    >
                      {{ commandPanelText }}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    :aria-label="$t('chat.slash.dismiss')"
                    @click="clearCurrentCommandEvent"
                  >
                    <X class="size-3.5" />
                  </Button>
                </div>
                <Command
                  v-if="commandResultItems.length"
                  class="h-auto w-auto rounded-none border-0 border-t border-border bg-transparent shadow-none"
                >
                  <CommandKeyBridge ref="commandPanelBridge">
                    <CommandList class="max-h-[min(20rem,45dvh)] p-1 overscroll-contain [scrollbar-gutter:stable]">
                      <CommandGroup>
                        <CommandItem
                          v-for="item in commandResultItems"
                          :key="`${item.kind || 'item'}:${item.id || item.title}`"
                          :value="`${item.kind || 'item'}:${item.id || item.title}`"
                          :disabled="item.kind !== 'skill'"
                          @select="selectCommandResultItem(item)"
                        >
                          <Sparkles
                            v-if="item.kind === 'skill'"
                            class="size-3.5 shrink-0 text-muted-foreground"
                          />
                          <List
                            v-else
                            class="size-3.5 shrink-0 text-muted-foreground"
                          />
                          <span class="min-w-0 flex-1">
                            <span class="block truncate text-body text-foreground">{{ item.title }}</span>
                            <span
                              v-if="item.description"
                              class="block truncate text-caption text-muted-foreground"
                            >{{ item.description }}</span>
                          </span>
                        </CommandItem>
                      </CommandGroup>
                    </CommandList>
                  </CommandKeyBridge>
                </Command>
              </div>
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
                class="chat-composer-edge relative flex w-full flex-wrap items-center gap-1 bg-surface-composer px-2.5 py-2.5 transition-[border-radius] motion-reduce:transition-none"
                :class="(isMultiline || showAttachmentGrid) ? 'rounded-[20px]' : 'rounded-[28px]'"
                :style="{ transitionDuration: `${composerRadiusMs}ms`, transitionTimingFunction: composerRadiusEase }"
                @click.self="focusTextarea"
              >
                <!-- The attachment row reveals via a grid 0fr↔1fr track so a card
                   is unveiled in place — it never translates and is always
                   clipped, so it can't overflow the box — while the composer
                   grows around it. The inner min-h-0 + overflow-hidden is what
                   lets the grid track actually collapse below content height. -->
                <Transition
                  enter-active-class="transition-[grid-template-rows] motion-reduce:transition-none"
                  enter-from-class="grid-rows-[0fr]"
                  enter-to-class="grid-rows-[1fr]"
                  leave-active-class="transition-[grid-template-rows] motion-reduce:transition-none"
                  leave-from-class="grid-rows-[1fr]"
                  leave-to-class="grid-rows-[0fr]"
                  :duration="ATTACHMENT_ANIM_MS"
                >
                  <div
                    v-if="showAttachmentGrid"
                    class="order-first grid w-full basis-full"
                    :style="{ transitionDuration: `${ATTACHMENT_ANIM_MS}ms`, transitionTimingFunction: 'cubic-bezier(0.25, 0.1, 0.25, 1)' }"
                  >
                    <div class="min-h-0 overflow-hidden">
                      <div class="flex flex-wrap gap-2 pb-1.5">
                        <ChatAttachmentCard
                          v-for="preview in pendingPreviews"
                          :key="preview.key"
                          :kind="preview.isPasted ? 'pasted' : (preview.isMedia ? 'media' : 'file')"
                          :src="preview.url"
                          :video="preview.isVideo"
                          :name="preview.file.name"
                          :ext="preview.ext"
                          :lines="preview.lines"
                          :text="preview.pastedText"
                          :size="preview.size"
                          :loading="preview.loading"
                          removable
                          :clickable="preview.isPasted || (preview.isMedia && !!preview.url)"
                          @remove="removeAttachment(preview.i)"
                          @preview="preview.isPasted ? (pastedViewerText = preview.pastedText) : openComposerPreview(preview.url)"
                        />
                      </div>
                    </div>
                  </div>
                </Transition>

                <Transition
                  enter-active-class="transition-opacity duration-150 ease-out"
                  enter-from-class="opacity-0"
                  enter-to-class="opacity-100"
                  leave-active-class="transition-opacity duration-100 ease-in"
                  leave-from-class="opacity-100"
                  leave-to-class="opacity-0"
                >
                  <div
                    v-if="skillSlashEnabled && requestedSkills.length"
                    class="order-first flex w-full basis-full flex-wrap gap-1.5 pb-1.5"
                  >
                    <div
                      v-for="skill in requestedSkills"
                      :key="requestedSkillKey(skill)"
                      class="flex min-h-8 max-w-full items-center gap-1.5 rounded-full bg-accent py-0.5 pl-2.5 pr-0.5 text-label text-foreground"
                    >
                      <Sparkles class="size-3.5 shrink-0 text-muted-foreground" />
                      <span class="min-w-0 truncate">{{ skill.display_name || skill.name }}</span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        :aria-label="$t('chat.slash.removeSkill', { name: skill.display_name || skill.name })"
                        class="shrink-0"
                        @click="removeRequestedSkill(skill)"
                      >
                        <X class="size-3.5" />
                      </Button>
                    </div>
                  </div>
                </Transition>

                <textarea
                  ref="textareaEl"
                  v-model="inputText"
                  rows="1"
                  :placeholder="activeChatReadOnly ? $t('chat.readonlyHint') : $t('chat.inputPlaceholder')"
                  :disabled="!currentBotId || activeChatReadOnly || loadingMessages"
                  class="field-sizing-content resize-none break-words bg-transparent text-base leading-[var(--chat-leading)] text-foreground outline-none placeholder:text-[var(--field-placeholder)] disabled:cursor-not-allowed"
                  :class="isMultiline
                    ? 'order-none w-full basis-full pl-2 pr-1 pt-2 pb-1.5 max-h-52'
                    : 'order-2 min-w-0 flex-1 self-center overflow-hidden whitespace-nowrap pl-1 pr-1 py-1 max-h-32'"
                  @keydown="handleComposerKeydown"
                  @paste="handlePaste"
                  @input="syncMultiline"
                />

                <DropdownMenu
                  v-if="composerMenuHasItems"
                  v-model:open="agentPopoverOpen"
                >
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
                    <!-- The agent runtime is fixed once a session has any turns,
                       so the switcher only appears while the session is still
                       empty. Showing it disabled in an active chat just dangles
                       a choice that can't be made. -->
                    <template v-if="canChangeAgent && enabledACPProfiles.length">
                      <DropdownMenuLabel>{{ $t('chat.agent') }}</DropdownMenuLabel>
                      <DropdownMenuItem @select="selectMemohAgent">
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
                      <DropdownMenuSeparator v-if="canChangeAgent && enabledACPProfiles.length" />
                      <DropdownMenuItem
                        :disabled="!currentBotId || activeChatReadOnly || streaming || loadingMessages"
                        @select="fileInput?.click()"
                      >
                        <Paperclip />
                        <span class="min-w-0 flex-1 truncate">{{ $t('chat.attachFiles') }}</span>
                      </DropdownMenuItem>
                    </template>
                  </DropdownMenuContent>
                </DropdownMenu>

                <!-- Compact: content-sized and pushed right (ml-auto) so the
                     textarea (flex-1) owns the slack. Multiline: grows to fill the
                     controls row (flex-1) and right-aligns, so a long model name
                     truncates within the row instead of overflowing it. -->
                <div
                  class="order-3 flex min-w-0 items-center gap-2"
                  :class="isMultiline ? 'flex-1 justify-end self-end' : 'ml-auto self-center'"
                >
                  <Popover v-model:open="modelPopoverOpen">
                    <PopoverTrigger as-child>
                      <Button
                        type="button"
                        variant="ghost"
                        :disabled="!currentBotId || activeChatReadOnly || acpModelChanging"
                        class="composer-pill-press h-9 min-w-0 gap-1 rounded-full px-3 text-muted-foreground"
                        :style="{ maxWidth: `${modelTriggerMaxWidth}px` }"
                      >
                        <LoaderCircle
                          v-if="acpModelChanging || acpModelsLoading"
                          class="size-3.5 shrink-0 animate-spin"
                        />
                        <span
                          ref="modelLabelEl"
                          class="min-w-0 truncate text-label"
                        >{{ modelTriggerLabel }}</span>
                        <ChevronDown class="size-3.5 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent
                      class="w-80 overflow-hidden p-0"
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
                        <InlineLoadingRow
                          v-if="acpModelsLoading"
                          class="px-2 py-3"
                        >
                          {{ $t('common.loading') }}
                        </InlineLoadingRow>
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
                        <InlineLoadingRow
                          v-if="acpModelsLoading"
                          class="px-2 py-3"
                        >
                          {{ $t('common.loading') }}
                        </InlineLoadingRow>
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
                          @close="modelPopoverOpen = false"
                        />
                      </div>
                    </PopoverContent>
                  </Popover>

                  <Button
                    v-if="activeIsACP"
                    type="button"
                    variant="ghost"
                    class="h-9 min-w-0 max-w-40 gap-1 rounded-full px-3 text-muted-foreground"
                    disabled
                  >
                    <FolderOpen class="size-3.5 shrink-0" />
                    <span
                      ref="acpProjectLabelEl"
                      class="min-w-0 truncate text-label"
                    >{{ activeACPProjectLabel }}</span>
                  </Button>

                  <div class="relative size-9 shrink-0">
                    <SessionInfoRing
                      v-if="!activeIsACP"
                      :override-model-id="overrideModelId"
                      :fallback-context-window="activeModel?.config?.context_window ?? null"
                      class="absolute inset-0 size-9 transition-[opacity,scale] duration-200 ease-out motion-reduce:transition-none"
                      :class="(!showSend && !streaming) ? 'scale-100 opacity-100' : 'pointer-events-none scale-75 opacity-0'"
                    />
                    <!-- Send and stop are one brand circle: the surface never
                         changes between the two states, only the glyph cross-fades
                         (arrow ⇄ stop square), so the button can't blink color or
                         shape mid-turn. While streaming it stays clickable to abort. -->
                    <Button
                      type="button"
                      variant="brand"
                      :disabled="streaming ? false : (!showSend || !currentBotId || activeChatReadOnly || loadingMessages)"
                      :aria-label="streaming ? 'Stop generating response' : 'Send message'"
                      class="absolute inset-0 size-9 rounded-full transition-[opacity,scale] duration-200 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none"
                      :class="(sendButtonVisible || streaming) ? 'scale-100 opacity-100' : 'pointer-events-none scale-0 opacity-0'"
                      @click="streaming ? chatStore.abort() : handleSend()"
                    >
                      <span
                        class="grid size-[20px] shrink-0 place-items-center"
                        aria-hidden="true"
                      >
                        <svg
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          stroke-width="2.75"
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          class="col-start-1 row-start-1 size-[20px] transition-opacity duration-200 ease-out motion-reduce:transition-none"
                          :class="streaming ? 'opacity-0' : 'opacity-100'"
                        >
                          <path d="M12 19.5 V5" />
                          <path d="M6 10.5 L12 4.5 L18 10.5" />
                        </svg>
                        <svg
                          viewBox="0 0 24 24"
                          fill="currentColor"
                          class="col-start-1 row-start-1 size-[18px] transition-opacity duration-200 ease-out motion-reduce:transition-none"
                          :class="streaming ? 'opacity-100' : 'opacity-0'"
                        >
                          <rect
                            x="4"
                            y="4"
                            width="16"
                            height="16"
                            rx="3"
                          />
                        </svg>
                      </span>
                    </Button>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, onMounted, useTemplateRef, watch, nextTick, onActivated, onDeactivated } from 'vue'
import {
  LoaderCircle,
  Paperclip,
  Plus,
  ChevronDown,
  CircleAlert,
  ArrowDown,
  Check,
  FolderOpen,
  Square,
  SquareCheck,
  Circle,
  CircleDot,
  Sparkles,
  X,
  HelpCircle,
  List,
  Minimize2,
  Package,
  SquarePen,
} from 'lucide-vue-next'
import { ScrollArea, Button, Popover, PopoverContent, PopoverTrigger, DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuItem, DropdownMenuSeparator, Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, Input, Command, CommandGroup, CommandItem, CommandKeyBridge, CommandList, CommandSeparator, Spinner, toast } from '@memohai/ui'
import { useChatStore, type ACPAgentSessionInput, type ChatMessage } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useScroll, useIntersectionObserver, useStorage } from '@vueuse/core'
import { useQuery } from '@pinia/colada'
import { getAcpProfiles, getModels, getProviders, getBotsByBotIdSettings } from '@memohai/sdk'
import type { AcpclientModelInfo, AcpprofilePublicProfile, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import MessageItem from './message-item.vue'
import ChatAttachmentCard from './chat-attachment-card.vue'
import PanePlaceholder from '@/components/pane-placeholder/index.vue'
import InlineLoadingRow from '@/components/inline-loading-row/index.vue'
import { useChatScroll } from '../composables/useChatScroll'
import BgTaskPill from './bg-task-pill.vue'
import ForkSourceDivider from './fork-source-divider.vue'
import { provideBgTaskBeacons } from '../composables/useBgTaskBeacons'
import MediaGalleryLightbox, { type MediaGalleryItem } from './media-gallery-lightbox.vue'
import SessionInfoRing from './session-info-ring.vue'
import { useSessionInfo } from '../composables/useSessionInfo'
import ChatModelPicker from './chat-model-picker.vue'
import { EFFORT_LABELS, REASONING_EFFORT_DISABLE, availableEffortsForMode, resolveEffortLevels, resolveThinkingMode } from '@/pages/bots/components/reasoning-effort'
import { useMediaGallery } from '../composables/useMediaGallery'
import { fetchSafeSkillCatalog, fetchSession, type ChatAttachment, type CommandActionError, type CommandActionListItem, type RequestedSkillSelection, type UIUserInput, type UIUserInputQuestion, type WSUserInputAnswer } from '@/composables/api/useChat'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { useACPRuntime } from '@/composables/useACPRuntime'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH, acpAgentIcon, findMissingRequiredManagedField, isACPAgentEnabled, isACPNoProject, normalizeACPAgentID, readACPAgentConfig } from '@/utils/acp'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { hasBotPermission } from '@/utils/bot-permissions'

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
  // Stable dockview panel id (e.g. `chat:3`). Used for per-tab composer drafts and
  // the keep-alive key — it does NOT change when a draft acquires a real session.
  tabId?: string
  // The session this pane renders (null = unsaved draft). Decoupled from tabId so
  // a draft→real promotion never remounts this pane.
  sessionId?: string | null
  active?: boolean
}>(), {
  tabId: 'chat',
  sessionId: null,
  active: true,
})

const { t } = useI18n()
const chatStore = useChatStore()
const { pill: bgTaskPill, scrollToOffscreen, cleanup: cleanupBgTaskBeacons } = provideBgTaskBeacons()
onBeforeUnmount(cleanupBgTaskBeacons)
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFiles = ref<File[]>([])

// Pasting a large block of text floods the composer and buries the controls, so
// past a threshold we capture it as a "pasted content" attachment card instead
// (the raw text still rides along as a .txt file on send). The trigger is set
// deliberately high so ordinary multi-line snippets keep landing in the input.
const PASTE_LINE_THRESHOLD = 50
const PASTE_CHAR_THRESHOLD = 2000
const PASTED_FILE_NAME = 'pasted-text.txt'
// Original text for each pasted-content file, so its card can preview the body
// and the viewer can show it in full without re-reading the synthetic File.
const pastedTexts = new WeakMap<File, string>()
function makePastedFile(text: string): File {
  const file = new File([text], PASTED_FILE_NAME, { type: 'text/plain' })
  pastedTexts.set(file, text)
  return file
}

function isMediaFile(file: File): boolean {
  return file.type.startsWith('image/') || file.type.startsWith('video/')
}

// A stable, collision-free key per File object (two byte-identical files are
// still distinct instances) so a card keeps its identity across reorders and
// never replays its entry animation when a sibling is removed.
const fileKeys = new WeakMap<File, string>()
let fileKeySeq = 0
function keyForFile(file: File): string {
  let key = fileKeys.get(file)
  if (!key) {
    key = `f${++fileKeySeq}`
    fileKeys.set(file, key)
  }
  return key
}

// Text-like files get a line count on their card (e.g. a pasted snippet or a
// .yml config), mirroring how a code block reads. Binary blobs are skipped so
// we never surface a meaningless newline tally for a PDF or archive.
const TEXT_EXTENSIONS = new Set([
  'txt', 'md', 'markdown', 'json', 'jsonc', 'yaml', 'yml', 'xml', 'csv', 'tsv',
  'log', 'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'vue', 'py', 'go', 'rs', 'java',
  'c', 'cc', 'cpp', 'h', 'hpp', 'css', 'scss', 'less', 'html', 'svg', 'sh', 'bash',
  'zsh', 'toml', 'ini', 'conf', 'env', 'sql', 'rb', 'php', 'swift', 'kt', 'gradle',
])
const LINE_COUNT_MAX_BYTES = 2 * 1024 * 1024
function isTextLikeFile(file: File): boolean {
  if (isMediaFile(file)) return false
  if (file.size > LINE_COUNT_MAX_BYTES) return false
  const mime = file.type.toLowerCase()
  if (mime.startsWith('text/')) return true
  if (mime === 'application/json' || mime === 'application/xml' || mime.includes('yaml')) return true
  const dot = file.name.lastIndexOf('.')
  const ext = dot > 0 ? file.name.slice(dot + 1).toLowerCase() : ''
  if (ext && TEXT_EXTENSIONS.has(ext)) return true
  // Pasted content arrives without a mime/extension — treat it as text.
  return mime === '' && ext === ''
}

// Object-URL previews for pending image/video attachments, keyed by File so a
// URL is created once and revoked the moment its file leaves the tray (or the
// composer unmounts) — no leaks across sends or session switches.
const pendingPreviewUrls = ref(new Map<File, string>())
// Line counts for text-like pending files, resolved asynchronously via FileReader.
// A `-1` sentinel marks "reading in progress" so a file is read at most once.
const pendingLineCounts = ref(new Map<File, number>())
function syncPendingAttachmentMeta(files: File[]) {
  const urls = pendingPreviewUrls.value
  for (const [file, url] of urls) {
    if (!files.includes(file)) {
      URL.revokeObjectURL(url)
      urls.delete(file)
    }
  }
  for (const file of files) {
    if (!urls.has(file) && isMediaFile(file)) urls.set(file, URL.createObjectURL(file))
  }

  const counts = pendingLineCounts.value
  for (const file of [...counts.keys()]) {
    if (!files.includes(file)) counts.delete(file)
  }
  for (const file of files) {
    if (counts.has(file) || !isTextLikeFile(file)) continue
    counts.set(file, -1)
    const reader = new FileReader()
    reader.onload = (e) => {
      if (!pendingFiles.value.includes(file)) return
      counts.set(file, String(e.target?.result ?? '').split('\n').length)
    }
    // -2 marks "read failed": no count to show, but the card must still reveal.
    reader.onerror = () => { if (pendingFiles.value.includes(file)) counts.set(file, -2) }
    reader.readAsText(file)
  }
}
watch(pendingFiles, files => syncPendingAttachmentMeta(files), { deep: true, immediate: true })
onBeforeUnmount(() => {
  for (const url of pendingPreviewUrls.value.values()) URL.revokeObjectURL(url)
  pendingPreviewUrls.value.clear()
})

const pendingPreviews = computed(() =>
  pendingFiles.value.map((file, i) => {
    const isImage = file.type.startsWith('image/')
    const isVideo = file.type.startsWith('video/')
    const isMedia = isImage || isVideo
    const dot = file.name.lastIndexOf('.')
    const url = pendingPreviewUrls.value.get(file) ?? ''
    const lc = pendingLineCounts.value.get(file)
    const pastedText = pastedTexts.get(file)
    const isPasted = pastedText !== undefined
    return {
      i,
      file,
      key: keyForFile(file),
      isMedia,
      isVideo,
      isPasted,
      pastedText: pastedText ?? '',
      size: file.size,
      url,
      ext: dot > 0 ? file.name.slice(dot + 1).toUpperCase() : '',
      lines: lc != null && lc >= 0 ? lc : null,
      // A text-like file is still loading until its line count resolves (sentinel
      // `undefined`/`-1`); the card shimmers until then, like the media skeleton.
      // Pasted content is held in memory already, so it reveals immediately.
      loading: !isPasted && !isMedia && isTextLikeFile(file) && (lc === undefined || lc === -1),
    }
  }),
)

// Lightbox for pending composer media so attachments can be verified at full
// size before sending. Driven separately from the message gallery since these
// object URLs are not part of the sent history yet.
const composerPreviewItems = computed<MediaGalleryItem[]>(() =>
  pendingPreviews.value
    .filter(p => p.isMedia && p.url)
    .map(p => ({ src: p.url, type: p.isVideo ? 'video' : 'image', name: p.file.name })),
)
const composerPreviewIndex = ref<number | null>(null)
function openComposerPreview(url: string) {
  const idx = composerPreviewItems.value.findIndex(item => item.src === url)
  if (idx >= 0) composerPreviewIndex.value = idx
}

// Full-text viewer for a pending pasted-content card, opened from its preview.
const pastedViewerText = ref<string | null>(null)
const pastedViewerOpen = computed({
  get: () => pastedViewerText.value !== null,
  set: (open: boolean) => { if (!open) pastedViewerText.value = null },
})

// Attachment row reveal/collapse timing (the grid 0fr↔1fr transition).
const ATTACHMENT_ANIM_MS = 230
// While the last card is collapsing the row stays mounted (the card holds its
// place) until the animation ends; the grid is open whenever there are cards and
// we're not in that closing window.
const collapsingAttachments = ref(false)
const showAttachmentGrid = computed(() => pendingPreviews.value.length > 0 && !collapsingAttachments.value)
let attachmentCollapseTimer: ReturnType<typeof setTimeout> | null = null
function removeAttachment(index: number) {
  const file = pendingFiles.value[index]
  if (!file) return
  // Removing one of several cards just reflows the open row; removing the last
  // one collapses the row first, then drops the card so it doesn't pop out.
  if (pendingFiles.value.length > 1) {
    pendingFiles.value.splice(index, 1)
    return
  }
  collapsingAttachments.value = true
  if (attachmentCollapseTimer) clearTimeout(attachmentCollapseTimer)
  attachmentCollapseTimer = setTimeout(() => {
    const i = pendingFiles.value.indexOf(file)
    if (i >= 0) pendingFiles.value.splice(i, 1)
    collapsingAttachments.value = false
    attachmentCollapseTimer = null
  }, ATTACHMENT_ANIM_MS)
}
// A new file arriving mid-collapse cancels the close so it can reveal instead.
watch(() => pendingFiles.value.length, (n, o) => {
  if (n > o && collapsingAttachments.value) {
    if (attachmentCollapseTimer) {
      clearTimeout(attachmentCollapseTimer)
      attachmentCollapseTimer = null
    }
    collapsingAttachments.value = false
  }
})
onBeforeUnmount(() => {
  if (attachmentCollapseTimer) clearTimeout(attachmentCollapseTimer)
})

const composerError = ref('')
const forkDialogOpen = ref(false)
const pendingForkMessageId = ref('')
const forkSessionTitle = ref('')
const forkSubmitting = ref(false)
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
  activeChatTarget,
  activeChatReadOnly,
  activeChatCanFork,
  loadingOlder,
  loadingChats,
  loadingMessages,
  hasMoreOlder,
  overrideModelId,
  overrideReasoningEffort,
  startupSendFailure,
  pendingACPModelId,
  pendingACPRuntimeStatus,
  pendingACPRuntimeEnsuring,
  hasExplicitSessionSelection,
} = storeToRefs(chatStore)

const isActive = computed(() => props.active !== false)

// A fresh, writable chat opens with the composer centred and a greeting above
// it. Read-only sessions (subagent / system / synced channel threads) hide the
// composer entirely, so they never reach this state.
const isWelcome = computed(() =>
  !!currentBotId.value
  && !activeChatReadOnly.value
  && !loadingChats.value
  && messages.value.length === 0,
)

// Rotate the greeting per fresh chat so the entry point feels alive rather than
// a fixed banner; the pick stays stable while a single welcome screen is shown
// and re-rolls when a new empty chat (bot/session) is opened.
const WELCOME_GREETING_KEYS = [
  'chat.welcome.g1', 'chat.welcome.g2', 'chat.welcome.g3', 'chat.welcome.g4',
  'chat.welcome.g5', 'chat.welcome.g6', 'chat.welcome.g7', 'chat.welcome.g8',
  'chat.welcome.g9', 'chat.welcome.g10', 'chat.welcome.g11', 'chat.welcome.g12',
] as const
function pickWelcomeGreetingIndex() {
  return Math.floor(Math.random() * WELCOME_GREETING_KEYS.length)
}
const welcomeGreetingIndex = ref(pickWelcomeGreetingIndex())
const welcomeGreeting = computed(() =>
  t(WELCOME_GREETING_KEYS[welcomeGreetingIndex.value] ?? WELCOME_GREETING_KEYS[0]),
)
watch([isWelcome, currentBotId, () => activeSession.value?.id], ([welcome]) => {
  if (welcome) welcomeGreetingIndex.value = pickWelcomeGreetingIndex()
})

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

const canForkAssistant = computed(() =>
  !streaming.value
  && !loadingMessages.value
  && !activeChatReadOnly.value
  && activeChatCanFork.value,
)

const latestRetryableAssistantId = computed(() => {
  if (streaming.value || loadingMessages.value || activeChatReadOnly.value) return ''
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const message = messages.value[i]
    if (message?.role === 'assistant' && !message.streaming && !message.__optimistic) {
      return (message.serverId ?? message.id).trim()
    }
  }
  return ''
})

const latestEditableUserId = computed(() => {
  if (streaming.value || loadingMessages.value || activeChatReadOnly.value) return ''
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const message = messages.value[i]
    if (message?.role === 'user' && !message.streaming && !message.__optimistic) {
      return (message.serverId ?? message.id).trim()
    }
  }
  return ''
})

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

const { data: botSettings, isLoading: botSettingsLoading } = useQuery({
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

const { data: acpProfileData, isLoading: acpProfilesLoading } = useQuery({
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

interface ForkSourceMeta {
  sessionId: string
  title: string
  sourceMessageId?: string
  forkMessageId?: string
}

const acpProfiles = computed<AcpprofilePublicProfile[]>(() => acpProfileData.value?.items ?? [])
const currentBotMetadata = computed(() => currentBot.value?.metadata as Record<string, unknown> | undefined)
const enabledACPProfiles = computed(() =>
  acpProfiles.value.filter(profile => isACPAgentEnabled(currentBotMetadata.value, profile.id)),
)

const activeSessionMetadata = computed<Record<string, unknown>>(() => activeChatTarget.value.metadata)
const forkSource = computed<ForkSourceMeta | null>(() => {
  const raw = activeSessionMetadata.value.forked_from
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const sessionId = String(record.session_id ?? '').trim()
  if (!sessionId) return null
  const title = String(record.title ?? '').trim() || t('chat.unknownSession')
  const sourceMessageId = String(record.message_id ?? '').trim()
  const forkMessageId = String(record.fork_message_id ?? '').trim()
  return {
    sessionId,
    title,
    ...(sourceMessageId ? { sourceMessageId } : {}),
    ...(forkMessageId ? { forkMessageId } : {}),
  }
})
const forkSourceTitle = computed(() => forkSource.value?.title ?? '')
const openingForkSource = ref(false)
const forkSourceDividerAfterIndex = computed<number | null>(() => {
  const source = forkSource.value
  if (!source || messages.value.length === 0) return null
  const forkMessageId = source.forkMessageId?.trim()
  if (!forkMessageId) return null
  const index = messages.value.findIndex(messageMatchesForkSource)
  return index >= 0 ? index : null
})
const activeIsPendingACP = computed(() => activeChatTarget.value.isPendingACP)
const activeIsACP = computed(() => activeChatTarget.value.isACP)
const activeACPAgentId = computed(() => normalizeACPAgentID(activeSessionMetadata.value.acp_agent_id))
const activeACPProjectLabel = computed(() => {
  if (isACPNoProject(activeSessionMetadata.value)) return t('chat.noProject')
  const path = String(activeSessionMetadata.value.project_path ?? '').trim()
  const parts = path.split('/').filter(Boolean)
  return path ? parts[parts.length - 1] ?? path : t('chat.noProject')
})
const canChangeAgent = computed(() => !streaming.value && messages.value.length === 0)


function messageMatchesForkSource(message: ChatMessage): boolean {
  const forkMessageId = forkSource.value?.forkMessageId?.trim()
  if (!forkMessageId) return false
  const candidates = [
    message.serverId,
    message.id,
    message.role === 'system' ? undefined : message.externalMessageId,
  ]
  return candidates.some(candidate => candidate?.trim() === forkMessageId)
}

function showForkSourceDividerAfter(message: ChatMessage, index: number): boolean {
  return Boolean(forkSource.value)
    && index === forkSourceDividerAfterIndex.value
    && messages.value[index] === message
}

function showForkSourceDividerBefore(index: number): boolean {
  return Boolean(forkSource.value)
    && (!forkSource.value?.forkMessageId || forkSourceDividerAfterIndex.value === null)
    && index === 0
}

// The composer's "+" menu is worth showing only when it can do something:
// switch the agent (empty session with ACP profiles) or attach files (Memoh
// mode). An in-progress ACP chat has neither, so the trigger is hidden rather
// than opening an empty sheet.
const composerMenuHasItems = computed(() =>
  (canChangeAgent.value && enabledACPProfiles.value.length > 0) || !activeIsACP.value,
)
const activeSessionId = computed(() => activeSession.value?.id ?? '')
const requestedSkills = ref<RequestedSkillSelection[]>([])
const skillSlashEnabled = computed(() => !activeIsACP.value && !activeIsPendingACP.value)
const { data: safeSkillCatalog, isLoading: safeSkillCatalogLoading } = useQuery({
  key: () => ['bot-safe-skills-catalog', currentBotId.value ?? ''],
  query: () => fetchSafeSkillCatalog(currentBotId.value!),
  enabled: () => !!currentBotId.value && skillSlashEnabled.value,
  refetchOnWindowFocus: false,
})
const safeSkills = computed(() => skillSlashEnabled.value ? safeSkillCatalog.value ?? [] : [])

function requestedSkillKey(skill: Pick<RequestedSkillSelection, 'name'>): string {
  return skill.name.trim()
}

function addRequestedSkill(skill: RequestedSkillSelection) {
  if (!skillSlashEnabled.value) return
  const name = skill.name?.trim()
  if (!name) return
  const key = requestedSkillKey({ name })
  if (requestedSkills.value.some(item => requestedSkillKey(item) === key)) return
  requestedSkills.value = [...requestedSkills.value, {
    name,
    display_name: skill.display_name?.trim() || undefined,
    description: skill.description?.trim() || undefined,
    source_kind: skill.source_kind?.trim() || undefined,
    state: skill.state?.trim() || undefined,
  }]
  if (inputText.value.trimStart().startsWith('/')) {
    inputText.value = ''
    saveInputDraft(inputDraftKey.value, '')
  }
  chatStore.clearCommandEvent()
  void nextTick(focusTextarea)
}

function removeRequestedSkill(skill: RequestedSkillSelection) {
  const key = requestedSkillKey(skill)
  requestedSkills.value = requestedSkills.value.filter(item => requestedSkillKey(item) !== key)
  void nextTick(focusTextarea)
}

watch([currentBotId, activeSessionId], () => {
  requestedSkills.value = []
  chatStore.clearCommandEvent()
})

watch(skillSlashEnabled, (enabled) => {
  if (enabled) return
  requestedSkills.value = []
  chatStore.clearCommandEvent()
})

const slashQuickActions = computed(() => [
  {
    id: 'help',
    label: '/help',
    description: t('chat.slash.helpDescription'),
    icon: HelpCircle,
  },
  ...(skillSlashEnabled.value
    ? [{
        id: 'skill.list',
        label: '/skill list',
        description: t('chat.slash.skillListDescription'),
        icon: List,
      }]
    : []),
  {
    id: 'new',
    label: '/new',
    description: t('chat.slash.newDescription'),
    icon: SquarePen,
  },
  ...(canCompactViaSlash.value
    ? [{
        id: 'compact',
        label: '/compact',
        description: sessionContextPercentKnown.value
          ? t('chat.slash.compactDescription', { percent: Math.round(sessionContextPercent.value) })
          : t('chat.slash.compactDescriptionNoStats'),
        icon: Minimize2,
      }]
    : []),
  ...(!activeIsACP.value && !activeIsPendingACP.value
    ? [{
        id: 'model',
        label: '/model',
        description: t('chat.slash.modelDescription', { model: selectedModelLabel.value }),
        icon: Package,
      }]
    : []),
])

const slashQuery = computed(() => {
  const text = inputText.value
  if (!text.trimStart().startsWith('/')) return ''
  const slashIndex = text.indexOf('/')
  return text.slice(slashIndex + 1).trim().toLowerCase()
})
const slashPanelOpen = computed(() =>
  isActive.value
  && !!currentBotId.value
  && !activeChatReadOnly.value
  && !loadingMessages.value
  && inputText.value.trimStart().startsWith('/')
  && !inputText.value.includes('\n'),
)
function slashMatches(label: string, description = ''): boolean {
  const query = slashQuery.value
  if (!query) return true
  const haystack = `${label} ${description}`.toLowerCase()
  return haystack.includes(query)
}
const visibleSlashQuickActions = computed(() =>
  slashQuickActions.value.filter(action => slashMatches(action.label, action.description)),
)
const visibleSlashSkills = computed(() =>
  safeSkills.value.filter(skill => slashMatches(skill.name, skill.description ?? '')),
)
const slashPanelHasResults = computed(() =>
  visibleSlashQuickActions.value.length > 0 || visibleSlashSkills.value.length > 0,
)

// Session usage for the /compact quick action's live description ("42% full")
// and its availability. Shares the query key with SessionInfoRing/panel, so
// this adds no extra fetch.
const {
  usedTokens: sessionUsedTokens,
  contextWindow: sessionContextWindow,
  contextPercent: sessionContextPercent,
  isCompacting: isCompactingSession,
  triggerCompact: triggerSessionCompact,
} = useSessionInfo({
  overrideModelId,
  fallbackContextWindow: computed(() => activeModel.value?.config?.context_window ?? null),
})
const sessionContextPercentKnown = computed(() => sessionContextWindow.value != null && sessionContextWindow.value > 0)
const canCompactViaSlash = computed(() =>
  !!activeSessionId.value && !activeIsACP.value && sessionUsedTokens.value > 0 && !isCompactingSession.value,
)

// Client-side quick actions run an existing UI affordance directly instead of
// round-tripping text through send: /compact triggers the session-info
// panel's compaction, /model opens the composer's model picker. Everything
// else keeps the type-and-send flow (the store intercepts /new; /help and
// /skill list execute server-side).
function runLocalQuickAction(id: string): boolean {
  if (id === 'compact') {
    if (!canCompactViaSlash.value) {
      composerError.value = t('chat.slash.compactUnavailable')
      return true
    }
    void triggerSessionCompact()
    return true
  }
  if (id === 'model') {
    modelPopoverOpen.value = true
    return true
  }
  return false
}

function selectSlashQuickAction(action: { id: string, label: string }) {
  if (runLocalQuickAction(action.id)) {
    inputText.value = ''
    saveInputDraft(inputDraftKey.value, '')
    void nextTick(focusTextarea)
    return
  }
  inputText.value = action.label
  saveInputDraft(inputDraftKey.value, action.label)
  void nextTick(() => {
    focusTextarea()
    void handleSend()
  })
}

// Typed forms of the client-side quick actions ("/compact", "/model") — must
// be intercepted before the store send path, which would otherwise classify
// them as skill activation and fail with requested_skill_not_found.
function localQuickActionIDForSlash(text: string): string {
  switch (text.trim().toLowerCase()) {
    case '/compact':
      return 'compact'
    case '/model':
    case '/models':
      return 'model'
    default:
      return ''
  }
}

function currentPaneCommandScope() {
  const activeBotId = (currentBotId.value ?? '').trim()
  const renderedSessionId = (props.sessionId || activeSessionId.value || chatStore.sessionId || '').trim()
  const paneScope = activeBotId && props.tabId.trim() ? `${activeBotId}:${props.tabId.trim()}` : 'chat'
  return renderedSessionId
    ? { botId: activeBotId || undefined, sessionId: renderedSessionId, composerScope: paneScope }
    : { botId: activeBotId || undefined, composerScope: paneScope }
}

function clearCurrentCommandEvent() {
  chatStore.clearCommandEvent(currentPaneCommandScope())
}

const commandPanelEvent = computed(() => chatStore.commandEventForScope(currentPaneCommandScope()))
const commandResult = computed(() => commandPanelEvent.value?.type === 'command_result' ? commandPanelEvent.value.result : null)
const commandError = computed(() => commandPanelEvent.value?.type === 'command_error' ? commandPanelEvent.value.error : null)
const commandPanelIsError = computed(() => !!commandError.value)
const commandPanelTitle = computed(() => {
  if (commandError.value) return t('chat.slash.commandError')
  return commandResult.value?.title || t('chat.slash.commandResult')
})
function localizedCommandErrorMessage(error: CommandActionError): string {
  const code = error.code.trim()
  if (code) {
    const key = `chat.slash.errorMessages.${code}`
    const translated = t(key)
    if (translated !== key) return translated
  }
  return error.message || t('chat.slash.errorMessages.generic')
}

const commandPanelText = computed(() => commandError.value ? localizedCommandErrorMessage(commandError.value) : commandResult.value?.text || '')
const commandResultItems = computed(() => commandResult.value?.items ?? [])

function selectCommandResultItem(item: CommandActionListItem) {
  if (!skillSlashEnabled.value) return
  if (item.kind !== 'skill' || !item.id?.trim() || !item.title.trim()) return
  addRequestedSkill({
    name: item.id.trim(),
    display_name: item.title,
    description: item.description,
  })
}
const {
  runtime: acpRuntime,
  models: acpModels,
  currentModelId: currentACPModelId,
  isEnsuring: acpRuntimeEnsuring,
  ensure: ensureActiveACPRuntime,
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

type DefaultACPSettings = {
  chat_runtime?: string
  chat_acp_agent_id?: string
  chat_acp_project_path?: string
  chat_acp_project_mode?: string
}

type DefaultACPAvailability = {
  input: ACPAgentSessionInput | null
  messageKey: string
  loading: boolean
}

const defaultACPAvailability = computed<DefaultACPAvailability>(() => {
  const settings = botSettings.value as (DefaultACPSettings | undefined)
  if (!settings) {
    return { input: null, messageKey: '', loading: !!currentBotId.value && botSettingsLoading.value }
  }
  if (settings.chat_runtime !== 'acp_agent') return { input: null, messageKey: '', loading: false }
  if (!hasBotPermission(currentBot.value?.current_user_permissions, 'workspace_exec')) {
    return { input: null, messageKey: 'chat.defaultACPNoWorkspaceExec', loading: false }
  }
  const agentId = normalizeACPAgentID(settings.chat_acp_agent_id)
  if (!agentId) return { input: null, messageKey: 'chat.defaultACPAgentMissing', loading: false }
  if (!acpProfileData.value) {
    return {
      input: null,
      messageKey: acpProfilesLoading.value ? 'chat.defaultACPLoading' : 'chat.defaultACPAgentUnavailable',
      loading: acpProfilesLoading.value,
    }
  }
  const profile = acpProfiles.value.find(item => normalizeACPAgentID(item.id) === agentId)
  if (!profile) return { input: null, messageKey: 'chat.defaultACPAgentUnavailable', loading: false }
  if (!isACPAgentEnabled(currentBotMetadata.value, profile.id)) {
    return { input: null, messageKey: 'chat.defaultACPAgentDisabled', loading: false }
  }
  const config = readACPAgentConfig(currentBotMetadata.value, profile.id)
  if (config.setupModeSet && findMissingRequiredManagedField(profile, config.managed, config.setupMode)) {
    return { input: null, messageKey: 'chat.defaultACPAgentNotConfigured', loading: false }
  }
  return {
    input: {
      agentId,
      projectPath: settings.chat_acp_project_path?.trim() || ACP_DEFAULT_PROJECT_PATH,
      projectMode: settings.chat_acp_project_mode?.trim() || ACP_DEFAULT_PROJECT_MODE,
    },
    messageKey: '',
    loading: false,
  }
})
const defaultACPSessionInput = computed(() => defaultACPAvailability.value.input)
const defaultACPUnavailableMessage = computed(() =>
  defaultACPAvailability.value.messageKey ? t(defaultACPAvailability.value.messageKey) : '',
)
const defaultACPLoading = computed(() => defaultACPAvailability.value.loading)
const defaultACPComposerError = ref('')

function clearDefaultACPComposerError() {
  if (defaultACPComposerError.value && composerError.value === defaultACPComposerError.value) {
    composerError.value = ''
  }
  defaultACPComposerError.value = ''
}

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
    requestedSkills.value = []
  }
})

function pendingMatchesDefaultACP(input: ACPAgentSessionInput): boolean {
  const metadata = activeChatTarget.value.metadata
  return activeChatTarget.value.kind === 'draft-acp'
    && metadata?.acp_agent_id === input.agentId
    && metadata?.project_path === (input.projectPath || ACP_DEFAULT_PROJECT_PATH)
    && metadata?.acp_project_mode === (input.projectMode || ACP_DEFAULT_PROJECT_MODE)
}

watch([defaultACPUnavailableMessage, defaultACPLoading, currentBotId, hasExplicitSessionSelection], ([message, loading]) => {
  clearDefaultACPComposerError()
  if (!message || !currentBotId.value) return
  if (hasExplicitSessionSelection.value) return
  if (!loading) {
    chatStore.resetToEmptyComposer()
  }
  defaultACPComposerError.value = message
  composerError.value = message
}, { immediate: true })

watch([defaultACPSessionInput, defaultACPLoading, currentBotId, hasExplicitSessionSelection, activeChatTarget], ([input, loading]) => {
  if (!currentBotId.value) return
  if (!input) {
    if (!loading) {
      chatStore.cacheDefaultACPSession(null)
    }
    if (!loading && !hasExplicitSessionSelection.value && activeIsPendingACP.value) {
      chatStore.resetToEmptyComposer()
    }
    return
  }
  chatStore.cacheDefaultACPSession(input)
  if (hasExplicitSessionSelection.value) return
  clearDefaultACPComposerError()
  if (pendingMatchesDefaultACP(input)) return
  chatStore.stageDefaultACPSession(input)
}, { immediate: true })

watch([modelPopoverOpen, activeIsPendingACP, activeIsACP, activeSessionId], ([open, isPending, isACP, sessionID]) => {
  if (!open) return
  if (isPending) {
    if (pendingACPRuntimeStatus.value || pendingACPRuntimeEnsuring.value) return
    void chatStore.ensurePendingACPRuntime().catch((error) => {
      composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
    })
    return
  }
  if (!isACP || !sessionID || acpRuntime.value || acpRuntimeEnsuring.value) return
  void ensureActiveACPRuntime().catch((error) => {
    composerError.value = resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
  })
})

function normalizedProfileID(value: unknown): string {
  return normalizeACPAgentID(value)
}

// Starting an ACP runtime (spawning the agent process + protocol handshake) has
// no server-side deadline, so a wedged agent would leave the composer spinning
// indefinitely — the user's only escape was a full page reload. Bound the switch
// on the client so the controls re-enable and a retry hint surfaces instead.
const AGENT_SWITCH_TIMEOUT_MS = 30_000

class AgentSwitchTimeout extends Error {}

function withAgentSwitchTimeout<T>(work: Promise<T>): Promise<T> {
  // Keep a detached handler so a late settle (after the race is decided) never
  // bubbles up as an unhandled rejection.
  void work.catch(() => {})
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new AgentSwitchTimeout()), AGENT_SWITCH_TIMEOUT_MS)
    work.then(
      (value) => { clearTimeout(timer); resolve(value) },
      (error) => { clearTimeout(timer); reject(error) },
    )
  })
}

function agentSwitchErrorMessage(error: unknown): string {
  return error instanceof AgentSwitchTimeout
    ? t('chat.agentSwitchTimeout')
    : resolveApiErrorMessage(error, t('chat.agentSwitchFailed'))
}

async function selectACPAgent(profile: AcpprofilePublicProfile) {
  const agentId = normalizeACPAgentID(profile.id)
  if (!agentId || agentChanging.value || !canChangeAgent.value) return
  agentPopoverOpen.value = false
  agentChanging.value = true
  composerError.value = ''
  try {
    if (chatStore.sessionId) {
      await withAgentSwitchTimeout(chatStore.updateCurrentSessionAgent({
        agentId,
      }))
    } else {
      chatStore.stageACPSession({
        agentId,
      })
      await withAgentSwitchTimeout(chatStore.ensurePendingACPRuntime())
    }
    pendingFiles.value = []
  } catch (error) {
    composerError.value = agentSwitchErrorMessage(error)
  } finally {
    agentChanging.value = false
  }
}

async function selectMemohAgent() {
  if (agentChanging.value || !canChangeAgent.value) return
  agentPopoverOpen.value = false
  if (!chatStore.sessionId) {
    chatStore.resetToEmptyComposer({ explicitSelection: true })
    clearDefaultACPComposerError()
    composerError.value = ''
    pendingFiles.value = []
    return
  }
  if (!activeIsACP.value) return
  agentChanging.value = true
  composerError.value = ''
  try {
    await withAgentSwitchTimeout(chatStore.updateCurrentSessionToMemoh())
  } catch (error) {
    composerError.value = agentSwitchErrorMessage(error)
  } finally {
    agentChanging.value = false
  }
}

function onModelSelected() {
  // The picker drives dismissal via @close (so opening a model's reasoning
  // options can adopt it without collapsing the menu); here we only sanitise
  // the effort when the new model can't reason.
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
const modelLabelEl = ref<HTMLElement | null>(null)
const acpProjectLabelEl = ref<HTMLElement | null>(null)
// The composer lifts to its multiline layout (textarea on its own row, controls
// below) for two independent reasons: the typed text wraps or holds a newline
// (textMultiline), or the pane is too narrow to seat the input + model capsule +
// send on one pill row (narrowMultiline). Either trigger flips isMultiline, so a
// cramped pane reflows into multiline instead of letting the pill explode.
const textMultiline = ref(false)
const narrowMultiline = ref(false)
const isMultiline = computed(() => textMultiline.value || narrowMultiline.value)
const compactContentWidth = ref(0)
const composerInnerWidth = ref(0)
const composerBoxHeight = ref(0)
const showSend = computed(() => Boolean(inputText.value.trim()) || pendingFiles.value.length > 0 || requestedSkills.value.length > 0)

// Whether the trailing slot shows the send button at all. In standard chat the
// SessionInfoRing fills that slot while idle and the send button only reveals
// once there's content (showSend). ACP sessions have no ring, so without this
// the slot would sit empty on empty input — the button must stay put and just
// fall to its disabled (dimmed brand) state instead of vanishing.
const sendButtonVisible = computed(() => showSend.value || activeIsACP.value)

// Border-radius morph, kept on the SAME clock as whatever box change drives it.
// An attachment open/close keys off showAttachmentGrid — which flips at the START
// of the collapse, before the card is spliced out — and borrows the grid reveal's
// duration + curve, so the corner rounds in lockstep with the height instead of
// lagging a beat behind it (the height was finishing first, then the corner moved
// only once the card was finally removed). A pill↔multiline text change keeps the
// form ease, matched to the JS height morph. Pre-flush watchers set the timing
// before the radius class flips, so the corner uses it.
const RADIUS_EASE_FORM = 'cubic-bezier(0.33, 1, 0.68, 1)'
const composerRadiusMs = ref(220)
const composerRadiusEase = ref(RADIUS_EASE_FORM)
watch(showAttachmentGrid, () => {
  composerRadiusMs.value = ATTACHMENT_ANIM_MS
  composerRadiusEase.value = 'cubic-bezier(0.25, 0.1, 0.25, 1)'
})
watch(isMultiline, () => {
  if (!showAttachmentGrid.value) {
    composerRadiusMs.value = 220
    composerRadiusEase.value = RADIUS_EASE_FORM
  }
})

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
    textMultiline.value = true
    return
  }
  const el = textareaEl.value
  if (el && !isMultiline.value) {
    const cs = getComputedStyle(el)
    const padX = Number.parseFloat(cs.paddingLeft) + Number.parseFloat(cs.paddingRight)
    const w = el.clientWidth - padX
    if (w > 1) compactContentWidth.value = w
  }
  textMultiline.value = measureWraps(text, compactContentWidth.value)
}

// Pixel budget for the compact (pill) row. The right cluster's *natural* width is
// derived from intrinsic measurements (the model label's scrollWidth, which a
// `truncate` span still reports in full) so the verdict never depends on the
// current layout — switching to multiline can't change the inputs and oscillate.
// When the inline textarea would be squeezed under MIN_INLINE_TEXTAREA, the pill
// can't host input + capsule + send on one line, so we reflow to multiline.
const MIN_INLINE_TEXTAREA = 120
const MODEL_TRIGGER_MAX = 240 // max-w-60
const PLUS_SLOT = 40 // size-9 (36) + gap-1 (4)
const SEND_SLOT = 36 // send / ring size-9
const MODEL_CHROME = 46 // px-3 ×2 + gap-1 + chevron + a little slack
const CLUSTER_GAP = 8 // gap-2 between cluster children
const ROW_GAPS = 8 // gap-1 on each flank of the textarea

function rightClusterNaturalWidth(): number {
  const modelLabel = modelLabelEl.value?.scrollWidth ?? 0
  const modelWidth = modelLabel > 0
    ? Math.min(MODEL_TRIGGER_MAX, modelLabel + MODEL_CHROME)
    : MODEL_TRIGGER_MAX
  let width = modelWidth + CLUSTER_GAP + SEND_SLOT
  const acpLabel = acpProjectLabelEl.value?.scrollWidth ?? 0
  if (acpLabel > 0) width += Math.min(160, acpLabel + 28) + CLUSTER_GAP
  return width
}

function recomputeComposerFit() {
  const el = composerEl.value
  if (!el) return
  const cs = getComputedStyle(el)
  const padX = Number.parseFloat(cs.paddingLeft) + Number.parseFloat(cs.paddingRight)
  const inner = el.clientWidth - padX
  if (inner <= 1) return
  composerInnerWidth.value = inner
  const room = inner - PLUS_SLOT - ROW_GAPS - rightClusterNaturalWidth()
  narrowMultiline.value = room < MIN_INLINE_TEXTAREA
}

// The model trigger inherits the Button's `shrink-0`, so it won't yield in a
// flex row — a long name would push past the box instead of truncating. A hard
// max-width clamps it regardless of flex-shrink (the min-w-0 label then ellipses
// within), sized to whatever the controls row can spare after the ＋, send, and
// any project pill. It only bites when space is tight; otherwise it rests at the
// 240px cap and the button still hugs a short name.
const modelTriggerMaxWidth = computed(() => {
  const inner = composerInnerWidth.value
  if (inner <= 1) return MODEL_TRIGGER_MAX
  let reserved = PLUS_SLOT + ROW_GAPS + CLUSTER_GAP + SEND_SLOT
  const acpLabel = acpProjectLabelEl.value?.scrollWidth ?? 0
  if (acpLabel > 0) reserved += Math.min(160, acpLabel + 28) + CLUSTER_GAP
  return Math.max(72, Math.min(MODEL_TRIGGER_MAX, inner - reserved))
})

// The bottom mask rises only to the box's vertical centre — its widest point.
// pb-8 (32px) is the strip beneath the box; + half the box height reaches the
// centre line, which falls behind the box's full-width middle so the mask's top
// edge is hidden by the box itself (no visible seam, no fade). Above that line
// the box's rounded top is left to float over whatever is there; below it the
// fill hides the bottom-corner gaps and the strip beneath, so nothing bleeds out.
const COMPOSER_MASK_BELOW = 32 // pb-8
const composerMaskHeight = computed(() => `${COMPOSER_MASK_BELOW + composerBoxHeight.value / 2}px`)

let composerResizeObserver: ResizeObserver | null = null
onMounted(() => {
  void nextTick(() => {
    syncMultiline()
    recomputeComposerFit()
  })
  if (typeof ResizeObserver !== 'undefined' && textareaEl.value) {
    composerResizeObserver = new ResizeObserver(() => syncMultiline())
    composerResizeObserver.observe(textareaEl.value)
  }
})

// A different model name (or switching to/from an ACP project pill) changes the
// right cluster's natural width, so re-run the fit check when the labels change.
watch([modelTriggerLabel, activeIsACP, activeACPProjectLabel], () => {
  void nextTick(recomputeComposerFit)
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
  composerBoxHeight.value = target
  // Only a pill↔multiline form change earns the height morph. Attachment rows
  // now reveal via their own grid 0fr↔1fr track (card stays put, box grows), and
  // plain line-wraps within multiline snap, so they're deliberately excluded.
  const formChanged = isMultiline.value !== composerMultiline
  composerMultiline = isMultiline.value
  if (!composerHeightReady || composerSnapNext) {
    composerSnapNext = false
    return
  }
  if (!formChanged) return
  if (!isActive.value || !from || Math.abs(target - from) < 0.5 || prefersReducedMotion()) return
  // Pin every line to the bottom and clip the overflow: the control row stays
  // welded to the fixed bottom edge (no twitch) while the box grows/shrinks and
  // the textarea is revealed/concealed from the top.
  el.style.overflow = 'hidden'
  el.style.alignContent = 'flex-end'
  pinComposerChildrenBottom(el, true)
  // A gentle ease-out whose tail decelerates to a soft stop — monotonic, so the
  // height moves to its target and stops without overshooting and bouncing back.
  const anim = el.animate(
    [{ height: `${from}px` }, { height: `${target}px` }],
    { duration: 220, easing: 'cubic-bezier(0.33, 1, 0.68, 1)' },
  )
  composerHeightAnim = anim
  anim.onfinish = () => {
    if (composerHeightAnim === anim) {
      clearComposerMorphStyles(el)
      composerHeightAnim = null
    }
  }
}

watch([inputText, isMultiline], () => {
  void nextTick(animateComposerHeight)
})

onMounted(() => {
  void nextTick(() => {
    composerHeight = composerEl.value?.offsetHeight ?? 0
    composerBoxHeight.value = composerHeight
    composerMultiline = isMultiline.value
    composerHeightReady = true
    composerSnapNext = false
  })
  const el = composerEl.value
  if (el && typeof ResizeObserver !== 'undefined') {
    composerSizeObserver = new ResizeObserver(() => {
      // The fit check keys off width only, so the height swing of a pill↔multiline
      // morph (same width) can't feed back and re-toggle it.
      recomputeComposerFit()
      // Skip while we drive the height ourselves; only capture layout-driven
      // resizes so the next morph starts from the real current height. The
      // keystroke path sets composerHeightAnim before this fires, so normal
      // morphs are untouched.
      if (!composerHeightAnim) {
        composerHeight = el.offsetHeight
        composerBoxHeight.value = el.offsetHeight
      }
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
  const discardFailure = () => chatStore.clearStartupSendFailure(failure.id)
  if (failure.botId && failure.botId !== currentBotId.value) {
    discardFailure()
    return
  }
  const failureScope = failure.composerScope?.trim()
  const paneScope = inputDraftKey.value || 'chat'
  const renderedSessionId = (props.sessionId || activeSessionId.value || chatStore.sessionId || '').trim()
  if (failureScope) {
    if (failureScope !== paneScope) {
      discardFailure()
      return
    }
    if (failure.sessionId) {
      if (renderedSessionId !== failure.sessionId) {
        discardFailure()
        return
      }
    } else if (renderedSessionId) {
      discardFailure()
      return
    }
  } else {
    if (failure.sessionId && failure.sessionId !== chatStore.sessionId) {
      discardFailure()
      return
    }
    // This pane carries the session it renders directly now (was derived from tabId
    // which is the stable panel id, not the session). A draft pane (null) still
    // accepts the restore for the send it just attempted.
    if (failure.sessionId && props.sessionId && props.sessionId !== failure.sessionId) {
      discardFailure()
      return
    }
  }

  inputText.value = failure.restoreInput
  saveInputDraft(inputDraftKey.value, failure.restoreInput)
  pendingFiles.value = (failure.restoreAttachments ?? [])
    .map(attachmentToFile)
    .filter((file): file is File => file !== null)
  requestedSkills.value = skillSlashEnabled.value
    ? (failure.restoreRequestedSkills ?? []).map(skill => ({ ...skill }))
    : []
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

// The last turn's container. A function ref because template refs inside
// v-for collect into arrays — bind just the pinnable (last) turn by hand.
const lastTurnEl = ref<HTMLElement | null>(null)
function setLastTurnEl(el: unknown) {
  lastTurnEl.value = el as HTMLElement | null
}

// The message list rendered as TURNS: a user message opens a turn that holds
// everything up to the next user message (leading assistant/system rows before
// the first user message form their own head turn). Keyed by the opening
// message's id, which never changes for a given turn — so sending a new
// message APPENDS a fresh container and no previous turn's DOM is ever
// re-parented. This is load-bearing for scroll stability: re-parenting (the
// earlier "split at the last prompt into two chunks" design) remounts the
// whole previous turn on every send — markdown re-renders, code re-highlights
// async, expanded tool groups collapse — and the transient height collapse
// showed up as a hard scroll jump when sending from the bottom.
// `start` is each turn's offset into the flat list, for the fork-source
// dividers whose positions are flat-list indexes.
const messageTurns = computed(() => {
  const turns: { id: string, start: number, messages: ChatMessage[] }[] = []
  messages.value.forEach((msg, index) => {
    const last = turns[turns.length - 1]
    if (msg.role === 'user' || !last) {
      turns.push({ id: msg.id, start: index, messages: [msg] })
    } else {
      last.messages.push(msg)
    }
  })
  return turns
})
const lastMessageId = computed(() => messages.value[messages.value.length - 1]?.id ?? '')

const {
  isScrolling,
  highlightedMessageId,
  showJumpToBottom: showJumpToBottomFromScroll,
  turnReserveStyle,
  scrollToBottom,
  scrollToMessage,
  suppressAutoScrollForPrepend,
  markEscaped,
  pinAfterSend,
  onActivatedRestoreScroll,
  onDeactivatedResetScroll,
  onMessageActive,
  startScrollTween,
  findMessageElement,
  getElementAbsoluteTop,
} = useChatScroll({
  scrollEl,
  contentEl: descEl,
  lastTurnEl,
  messages,
  isActive,
  sessionId: computed(() => chatStore.sessionId),
})
const showJumpToBottom = computed(() => showJumpToBottomFromScroll.value && !loadingChats.value)

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
    // Rail navigation parks the reader on a chosen turn, so escape follow —
    // otherwise the next streamed mutation would drag them back to the bottom.
    markEscaped()
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
})

// Sentinel-based infinite scroll for older history. Position preservation
// across the prepend itself is owned by useChatScroll (see
// suppressAutoScrollForPrepend's doc comment for why no manual scrollTop
// correction is needed).
async function ensureOlderLoaded() {
  if (loadingOlder.value || !hasMoreOlder.value) return
  if (!messages.value.length) return
  suppressAutoScrollForPrepend()
  try {
    await chatStore.loadOlderMessages()
  } catch (error) {
    console.error('Failed to load older messages:', error)
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

onActivated(() => {
  onActivatedRestoreScroll(loadingMessages)
})

onDeactivated(() => {
  onDeactivatedResetScroll()
})

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

async function handleForkSourceClick() {
  const source = forkSource.value
  const botId = currentBotId.value?.trim() ?? ''
  if (!source || !botId || openingForkSource.value) return
  openingForkSource.value = true
  try {
    await fetchSession(botId, source.sessionId)
    await chatStore.selectSession(source.sessionId, { explicitSelection: true })
  } catch {
    toast.error(t('chat.forkSourceUnavailable'))
  } finally {
    openingForkSource.value = false
  }
}

function defaultForkSessionTitle() {
  const sourceTitle = activeSession.value?.title?.trim() || t('chat.unknownSession')
  return t('chat.forkDialog.defaultTitle', { session: sourceTitle })
}

function handleForkMessage(messageId: string) {
  composerError.value = ''
  const id = messageId.trim()
  if (!id) return
  pendingForkMessageId.value = id
  forkSessionTitle.value = defaultForkSessionTitle()
  forkDialogOpen.value = true
}

async function handleCreateFork() {
  const messageId = pendingForkMessageId.value.trim()
  const title = forkSessionTitle.value.trim()
  if (!messageId || !title || forkSubmitting.value) return
  composerError.value = ''
  forkSubmitting.value = true
  try {
    const ok = await chatStore.forkMessage(messageId, { title })
    if (ok) {
      forkDialogOpen.value = false
      pendingForkMessageId.value = ''
    }
  } finally {
    forkSubmitting.value = false
  }
}

// Keyboard bridges into the two composer list surfaces (slash picker, command
// panel results). The composer textarea keeps focus the whole time — like
// reka's ListboxFilter, the bridge runs the listbox in virtual-highlight mode
// and the textarea forwards navigation keys to whichever surface is showing.
const slashPickerBridge = ref<InstanceType<typeof CommandKeyBridge> | null>(null)
const commandPanelBridge = ref<InstanceType<typeof CommandKeyBridge> | null>(null)

function activeComposerListBridge() {
  if (slashPanelOpen.value && slashPanelHasResults.value) return slashPickerBridge.value
  if (commandPanelEvent.value && commandResultItems.value.length) return commandPanelBridge.value
  return null
}

function handleComposerKeydown(e: KeyboardEvent) {
  if (e.isComposing || e.keyCode === 229) return
  if (e.key === 'Escape') {
    // Dismiss the command result panel; the slash picker is input-driven and
    // closes by editing the text, so Escape only targets the panel.
    if (!slashPanelOpen.value && commandPanelEvent.value) {
      e.preventDefault()
      clearCurrentCommandEvent()
    }
    return
  }
  const bridge = activeComposerListBridge()
  if (bridge && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
    e.preventDefault()
    bridge.navigate(e)
    return
  }
  if (e.key !== 'Enter' || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return
  if (bridge?.hasHighlight) {
    e.preventDefault()
    bridge.select(e)
    return
  }
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
  const data = e.clipboardData
  if (!data) return
  let handledFile = false
  for (const item of Array.from(data.items ?? [])) {
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
  if (handledFile) {
    e.preventDefault()
    return
  }
  // A large text paste becomes a pasted-content card so it doesn't bury the
  // composer; anything below the threshold drops into the textarea as usual.
  const text = data.getData('text/plain')
  if (!text) return
  const lineCount = text.split('\n').length
  if (lineCount >= PASTE_LINE_THRESHOLD || text.length >= PASTE_CHAR_THRESHOLD) {
    e.preventDefault()
    pendingFiles.value.push(makePastedFile(text))
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

function attachmentToFile(attachment: ChatAttachment): File | null {
  const source = attachment.base64?.trim()
  if (!source) return null
  try {
    const commaIndex = source.indexOf(',')
    const payload = commaIndex >= 0 ? source.slice(commaIndex + 1) : source
    const meta = commaIndex >= 0 ? source.slice(0, commaIndex) : ''
    const bytes = atob(payload)
    const buffer = new Uint8Array(bytes.length)
    for (let i = 0; i < bytes.length; i += 1) {
      buffer[i] = bytes.charCodeAt(i)
    }
    const inferredMime = meta.match(/^data:([^;,]+)/)?.[1]
    return new File([buffer], attachment.name?.trim() || 'attachment', {
      type: attachment.mime?.trim() || inferredMime || 'application/octet-stream',
    })
  } catch {
    return null
  }
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

async function handleRetryMessage(messageId: string) {
  composerError.value = ''
  const result = await chatStore.retryLatestAssistant(messageId)
  if (!result.ok && result.error) {
    composerError.value = result.error
  }
}

async function handleEditMessage(messageId: string, text: string, done?: (started: boolean) => void) {
  composerError.value = ''
  try {
    const result = await chatStore.editLatestUser(messageId, text)
    if (!result.ok && result.error) {
      composerError.value = result.error
    }
    done?.(result.ok || result.stage === 'stream')
  } catch {
    done?.(false)
  }
}

async function handleSend() {
  if (!isActive.value) return
  if (!skillSlashEnabled.value && requestedSkills.value.length) {
    requestedSkills.value = []
  }
  const text = inputText.value.trim()
  const files = [...pendingFiles.value]
  const skills = [...requestedSkills.value]
  if ((!text && !files.length && !skills.length) || streaming.value || loadingMessages.value || activeChatReadOnly.value) return
  if (text.startsWith('/') && files.length) {
    composerError.value = ''
    chatStore.showCommandError('slash_attachments_unsupported', t('chat.slash.attachmentsUnsupported'), {
      botId: currentBotId.value ?? undefined,
      sessionId: activeSessionId.value || undefined,
      composerScope: inputDraftKey.value || 'chat',
    })
    return
  }
  const localAction = localQuickActionIDForSlash(text)
  if (localAction && runLocalQuickAction(localAction)) {
    inputText.value = ''
    saveInputDraft(inputDraftKey.value, '')
    return
  }
  const isNewCommand = /^\/new(?:\s|$)/i.test(text)
  if (defaultACPComposerError.value && !hasExplicitSessionSelection.value && !isNewCommand) {
    composerError.value = defaultACPComposerError.value
    return
  }
  if (activeIsACP.value && files.length) {
    composerError.value = t('chat.acpAttachmentsUnsupported')
    return
  }

  const sentDraftKey = inputDraftKey.value
  const sentComposerScope = inputDraftKey.value || 'chat'
  const sentBotId = currentBotId.value ?? ''
  const sentSessionId = activeSessionId.value || ''
  composerError.value = ''
  inputText.value = ''
  saveInputDraft(sentDraftKey, '')
  pendingFiles.value = []
  requestedSkills.value = []

  let attachments: ChatAttachment[] | undefined
  try {
    if (files.length) {
      attachments = await Promise.all(files.map(fileToAttachment))
    }
  } catch (error) {
    inputText.value = text
    pendingFiles.value = files
    requestedSkills.value = skills
    composerError.value = error instanceof Error ? error.message : t('chat.sendFailed')
    return
  }

  // Pin the just-sent turn: reserve viewport space under it and smooth-scroll
  // once (browser-native) so the prompt lands near the top with the reply
  // streaming into the blank below. The view then stays parked — no
  // auto-follow — until the user scrolls back to the bottom. Armed before the
  // append so the message's own DOM mutation applies the pin.
  pinAfterSend()
  const result = await chatStore.sendMessage(text, attachments, {
    requestedSkills: skills,
    composerScope: sentComposerScope,
  })
  if (!result.ok && result.stage === 'startup') {
    const restoreInput = result.restoreInput ?? text
    const activeRenderedSessionId = (props.sessionId || activeSessionId.value || chatStore.sessionId || '').trim()
    const stillOriginalComposer = (currentBotId.value ?? '') === sentBotId
      && (inputDraftKey.value || 'chat') === sentComposerScope
      && ((!sentSessionId && !activeRenderedSessionId) || (!!sentSessionId && activeRenderedSessionId === sentSessionId))
    if (!stillOriginalComposer) return
    inputText.value = restoreInput
    saveInputDraft(sentDraftKey, restoreInput)
    pendingFiles.value = files
    requestedSkills.value = skills
    if (commandPanelEvent.value?.type !== 'command_error') {
      composerError.value = result.error || t('chat.sendFailed')
    }
  }
}
</script>
