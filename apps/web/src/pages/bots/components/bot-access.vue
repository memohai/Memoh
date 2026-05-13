<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground">
          {{ $t('bots.access.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.access.subtitle') }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          v-if="!formVisible"
          size="sm"
          class="h-8 text-[11px] font-medium px-3 shadow-none"
          @click="openAddDialog"
        >
          <Plus class="mr-1.5 size-3.5" />
          {{ addListEntryLabel }}
        </Button>
      </div>
    </header>

    <!-- Access Mode (Posture Style) -->
    <section class="space-y-4 rounded-md border p-4 bg-background/50">
      <div class="space-y-1">
        <p class="text-xs font-medium text-foreground">
          {{ $t('bots.access.modeTitle') }}
        </p>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('bots.access.modeDescription') }}
        </p>
      </div>
      <div class="grid gap-3 md:grid-cols-2">
        <button
          type="button"
          class="flex flex-col items-start rounded-lg border p-4 text-left transition-all duration-200 group shadow-none"
          :class="defaultEffectDraft === 'allow'
            ? 'border-foreground bg-muted text-foreground'
            : 'border-border/60 bg-background/70 text-foreground hover:bg-accent/50'"
          :disabled="isSavingDefaultEffect"
          @click="handleSetDefaultEffect('allow')"
        >
          <div
            class="size-8 rounded flex items-center justify-center mb-3 transition-colors shadow-none"
            :class="defaultEffectDraft === 'allow' ? 'bg-foreground text-background' : 'bg-muted text-muted-foreground group-hover:bg-background'"
          >
            <ShieldAlert class="size-5" />
          </div>
          <span class="text-sm font-semibold mb-1">{{ $t('bots.access.blacklistMode') }}</span>
          <span class="text-[11px] leading-relaxed text-muted-foreground">
            {{ $t('bots.access.blacklistModeDescription') }}
          </span>
        </button>

        <button
          type="button"
          class="flex flex-col items-start rounded-lg border p-4 text-left transition-all duration-200 group shadow-none"
          :class="defaultEffectDraft === 'deny'
            ? 'border-foreground bg-muted text-foreground'
            : 'border-border/60 bg-background/70 text-foreground hover:bg-accent/50'"
          :disabled="isSavingDefaultEffect"
          @click="handleSetDefaultEffect('deny')"
        >
          <div
            class="size-8 rounded flex items-center justify-center mb-3 transition-colors shadow-none"
            :class="defaultEffectDraft === 'deny' ? 'bg-foreground text-background' : 'bg-muted text-muted-foreground group-hover:bg-background'"
          >
            <ShieldCheck class="size-5" />
          </div>
          <span class="text-sm font-semibold mb-1">{{ $t('bots.access.whitelistMode') }}</span>
          <span class="text-[11px] leading-relaxed text-muted-foreground">
            {{ $t('bots.access.whitelistModeDescription') }}
          </span>
        </button>
      </div>
    </section>

    <!-- Rules List -->
    <section class="space-y-4">
      <div class="flex items-center justify-between">
        <div class="space-y-1">
          <h3 class="text-xs font-medium text-foreground">
            {{ listTitle }}
          </h3>
          <p class="text-[11px] text-muted-foreground">
            {{ listDescription }}
          </p>
        </div>
      </div>

      <div
        v-if="isLoadingRules"
        class="flex justify-center py-12"
      >
        <Spinner class="size-6 text-muted-foreground/50" />
      </div>

      <Empty
        v-else-if="visibleRules.length === 0"
        :title="emptyTitle"
        :description="emptyDescription"
        class="border border-dashed border-border/60 bg-muted/5 rounded-lg py-10"
      >
        <Button
          variant="outline"
          size="sm"
          class="mt-4 h-8 text-[11px] border-border/60 shadow-none"
          @click="openAddDialog"
        >
          <Plus class="mr-1.5 size-3.5" />
          {{ addListEntryLabel }}
        </Button>
      </Empty>

      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="rule in visibleRules"
          :key="rule.id"
          class="group flex items-center gap-3 rounded-md border border-border/60 bg-background/50 px-3 py-2.5 hover:bg-muted/30 transition-all duration-200"
        >
          <!-- Avatar size-8 (32px) -->
          <Avatar
            v-if="rule.channel_identity_id"
            class="size-8 shrink-0 border border-border/40"
          >
            <AvatarImage
              :src="rule.channel_identity_avatar_url"
              :alt="describeRuleTarget(rule)"
            />
            <AvatarFallback class="text-[10px]">
              {{ ruleTargetFallback(rule) }}
            </AvatarFallback>
          </Avatar>
          <div
            v-else-if="rule.subject_channel_type"
            class="flex size-8 shrink-0 items-center justify-center rounded bg-background border border-border/50 text-muted-foreground"
          >
            <ChannelIcon
              :channel="rule.subject_channel_type"
              size="1em"
            />
          </div>
          <div
            v-else
            class="flex size-8 shrink-0 items-center justify-center rounded bg-background border border-border/50 text-muted-foreground"
          >
            <Users class="size-4" />
          </div>

          <div class="min-w-0 flex-1 space-y-0.5">
            <div class="flex min-w-0 items-center gap-2">
              <p class="truncate text-[11px] font-semibold text-foreground">
                {{ describeRuleTarget(rule) }}
              </p>
              <Badge
                variant="outline"
                class="h-4 px-1.5 text-[9px] font-medium rounded-full shadow-none"
                :class="rule.enabled ? 'bg-foreground/5 text-foreground border-foreground/20' : 'bg-muted/40 text-muted-foreground border-border/40'"
              >
                {{ rule.enabled ? $t('bots.access.ruleEnabled') : $t('bots.access.ruleDisabled') }}
              </Badge>
            </div>
            <div class="flex min-w-0 items-center text-[10px] text-muted-foreground">
              <span class="shrink-0">
                {{ ruleScopePrefix(rule) }}
              </span>
              <template v-if="ruleScopeDetail(rule)">
                <span class="shrink-0 mx-1">: </span>
                <Avatar
                  v-if="rule.source_conversation_avatar_url"
                  class="mr-1 size-3.5 shrink-0"
                >
                  <AvatarImage
                    :src="rule.source_conversation_avatar_url"
                    :alt="rule.source_conversation_name || rule.source_scope?.conversation_id"
                  />
                  <AvatarFallback class="text-[8px]">
                    {{ ruleScopeFallback(rule) }}
                  </AvatarFallback>
                </Avatar>
                <span class="truncate">
                  {{ ruleScopeDetail(rule) }}
                </span>
              </template>
            </div>
            <p
              v-if="rule.description"
              class="truncate text-[10px] text-muted-foreground/70 italic leading-relaxed"
            >
              {{ rule.description }}
            </p>
          </div>

          <!-- Icon-Only Actions -->
          <div class="shrink-0 flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
            <Button
              variant="ghost"
              size="icon-sm"
              class="size-7 shadow-none"
              :class="rule.enabled ? 'text-muted-foreground' : 'text-foreground hover:bg-foreground/5'"
              :aria-label="rule.enabled ? $t('bots.access.disableRule') : $t('bots.access.enableRule')"
              @click="handleToggleEnabled(rule, !(rule.enabled ?? false))"
            >
              <Power class="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              class="size-7"
              :aria-label="$t('common.edit')"
              @click="openEditDialog(rule)"
            >
              <SquarePen class="size-3.5 text-muted-foreground" />
            </Button>
            <ConfirmPopover
              :message="$t('bots.access.deleteConfirmDescription')"
              :confirm-text="$t('common.delete')"
              @confirm="handleDeleteRule(rule.id!)"
            >
              <template #trigger>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  class="size-7 text-destructive/70 hover:text-destructive hover:bg-destructive/10"
                  :aria-label="$t('common.delete')"
                >
                  <Trash2 class="size-3.5" />
                </Button>
              </template>
            </ConfirmPopover>
          </div>
        </div>
      </div>
    </section>

    <!-- Inline Add/Edit Rule Form -->
    <Transition name="fade">
      <section
        v-if="formVisible"
        class="space-y-5 rounded-md border border-border/60 bg-muted/10 p-4 relative"
      >
        <div class="flex items-center justify-between">
          <h3 class="text-xs font-semibold text-foreground">
            {{ editingRule ? $t('bots.access.editRule') : addListEntryLabel }}
          </h3>
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7"
            @click="formVisible = false"
          >
            <X class="size-4" />
          </Button>
        </div>

        <form
          class="space-y-4"
          @submit.prevent="handleSaveRule(false)"
        >
          <!-- Rule Preview (Code Block Style) -->
          <div class="rounded-md bg-background/80 border border-border/40 px-3 py-2.5 font-mono text-[10px] leading-relaxed text-muted-foreground">
            <div class="flex items-center gap-2 mb-1">
              <div class="size-1.5 rounded-full bg-foreground/40" />
              <span class="text-foreground/60 uppercase tracking-wider font-bold">Preview</span>
            </div>
            {{ rulePreviewText }}
          </div>

          <!-- Platform Scope -->
          <div class="space-y-1.5">
            <div class="flex items-center justify-between gap-2">
              <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.access.platformQuestion') }}</Label>
              <Button
                v-if="ruleForm.subjectChannelType"
                type="button"
                variant="ghost"
                size="sm"
                class="h-5 px-1.5 text-[10px] text-muted-foreground/60"
                @click="setPlatformScope('')"
              >
                {{ $t('bots.access.allPlatforms') }}
              </Button>
            </div>
            <SearchableSelectPopover
              v-model="ruleForm.subjectChannelType"
              :options="platformOptions"
              :placeholder="$t('bots.access.allPlatforms')"
              :search-placeholder="$t('bots.access.searchPlatform')"
              :empty-text="$t('bots.access.noPlatformCandidates')"
              :show-group-headers="false"
              @update:model-value="setPlatformScope"
            >
              <template #trigger="{ open, displayLabel }">
                <Button
                  variant="outline"
                  role="combobox"
                  :aria-expanded="open"
                  class="w-full justify-between font-normal h-8 text-[11px] bg-background/50 border-border/50 shadow-none focus-visible:ring-ring/30"
                >
                  <span class="flex min-w-0 items-center gap-2 truncate">
                    <span
                      v-if="ruleForm.subjectChannelType"
                      class="flex size-5 shrink-0 items-center justify-center rounded bg-muted/40 text-muted-foreground"
                    >
                      <ChannelIcon
                        :channel="ruleForm.subjectChannelType"
                        size="1em"
                      />
                    </span>
                    <span class="truncate">
                      {{ displayLabel || $t('bots.access.allPlatforms') }}
                    </span>
                  </span>
                  <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground/60" />
                </Button>
              </template>
              <template #option-label="{ option }">
                <div class="flex min-w-0 items-center gap-2 text-left py-0.5">
                  <span
                    v-if="option.value"
                    class="flex size-5 shrink-0 items-center justify-center rounded bg-muted/40 text-muted-foreground"
                  >
                    <ChannelIcon
                      :channel="option.value"
                      size="1em"
                    />
                  </span>
                  <span
                    v-else
                    class="size-5 shrink-0"
                  />
                  <span class="truncate text-[11px]">
                    {{ option.label }}
                  </span>
                </div>
              </template>
            </SearchableSelectPopover>
          </div>

          <!-- Channel Identity -->
          <div class="space-y-1.5">
            <div class="flex items-center justify-between gap-2">
              <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.access.userQuestion') }}</Label>
              <Button
                v-if="ruleForm.channelIdentityId"
                type="button"
                variant="ghost"
                size="sm"
                class="h-5 px-1.5 text-[10px] text-muted-foreground/60"
                @click="setChannelIdentity('')"
              >
                {{ $t('bots.access.allUsers') }}
              </Button>
            </div>
            <SearchableSelectPopover
              v-model="ruleForm.channelIdentityId"
              :options="filteredIdentityOptions"
              :placeholder="$t('bots.access.selectIdentity')"
              :search-placeholder="$t('bots.access.searchIdentity')"
              :empty-text="$t('bots.access.noIdentityCandidates')"
              @update:model-value="setChannelIdentity"
            >
              <template #trigger="{ open, displayLabel }">
                <Button
                  variant="outline"
                  role="combobox"
                  :aria-expanded="open"
                  class="w-full justify-between font-normal h-8 text-[11px] bg-background/50 border-border/50 shadow-none focus-visible:ring-ring/30"
                >
                  <span class="flex min-w-0 items-center gap-2 truncate">
                    <Avatar
                      v-if="selectedIdentityOption"
                      class="size-5 shrink-0 border border-border/40"
                    >
                      <AvatarImage
                        :src="selectedIdentityOption.meta.avatarUrl"
                        :alt="selectedIdentityOption.label"
                      />
                      <AvatarFallback class="text-[8px]">{{ selectedIdentityOption.label.slice(0, 2).toUpperCase() }}</AvatarFallback>
                    </Avatar>
                    <span class="truncate">
                      {{ displayLabel || $t('bots.access.selectIdentity') }}
                    </span>
                  </span>
                  <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground/60" />
                </Button>
              </template>
              <template #option-label="{ option }">
                <div class="flex min-w-0 items-center gap-2 text-left py-0.5">
                  <Avatar class="size-6 shrink-0 border border-border/40">
                    <AvatarImage
                      :src="option.meta?.avatarUrl"
                      :alt="option.label"
                    />
                    <AvatarFallback class="text-[8px]">
                      {{ option.label.slice(0, 2).toUpperCase() }}
                    </AvatarFallback>
                  </Avatar>
                  <div class="min-w-0">
                    <div class="truncate text-[11px]">
                      {{ option.label }}
                    </div>
                    <div
                      v-if="option.meta?.channelLabel"
                      class="truncate text-[10px] text-muted-foreground/60"
                    >
                      {{ formatIdentityOptionSubtitle(option.meta) }}
                    </div>
                  </div>
                </div>
              </template>
            </SearchableSelectPopover>
          </div>

          <!-- Chat Scope -->
          <div class="space-y-2">
            <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.access.scopeQuestion') }}</Label>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
              <button
                v-for="scope in chatScopeOptions"
                :key="scope.value || 'any'"
                type="button"
                class="rounded border px-2 py-1.5 text-[10px] font-semibold transition-all text-center h-8 flex items-center justify-center"
                :class="ruleForm.sourceConversationType === scope.value
                  ? 'border-foreground/30 bg-foreground/10 text-foreground'
                  : 'border-border/50 bg-background/50 text-muted-foreground hover:bg-accent hover:text-foreground'"
                @click="setChatScope(scope.value)"
              >
                {{ scope.label }}
              </button>
            </div>
          </div>

          <!-- Specific Conversation Section -->
          <section
            v-if="showSpecificConversationSection"
            class="space-y-4 border-l-2 border-border/40 pl-4 py-1"
          >
            <div class="space-y-1">
              <p class="text-[11px] font-semibold text-foreground/80">
                {{ $t('bots.access.specificConversationTitle') }}
              </p>
              <p class="text-[10px] text-muted-foreground leading-relaxed">
                {{ $t('bots.access.specificConversationDescription') }}
              </p>
            </div>

            <div
              v-if="showConversationSearch"
              class="space-y-1.5"
            >
              <Label class="text-[10px] font-medium text-muted-foreground uppercase tracking-tight">{{ $t('bots.access.existingConversation') }}</Label>
              <SearchableSelectPopover
                v-model="ruleForm.observedConversationRouteId"
                :options="observedConversationOptions"
                :placeholder="$t('bots.access.selectConversationSource')"
                :empty-text="observedConversationEmptyText"
                @update:model-value="onConversationSourceChange"
              >
                <template #trigger="{ open, displayLabel, selectedOption }">
                  <Button
                    variant="outline"
                    role="combobox"
                    :aria-expanded="open"
                    class="w-full justify-between font-normal h-8 text-[11px] bg-background/50 border-border/50 shadow-none"
                  >
                    <span class="flex min-w-0 items-center gap-2 truncate">
                      <Avatar
                        v-if="observedConversationAvatar(selectedOption?.meta)"
                        class="size-5 shrink-0 border border-border/40"
                      >
                        <AvatarImage
                          :src="observedConversationAvatar(selectedOption?.meta)"
                          :alt="displayLabel"
                        />
                        <AvatarFallback class="text-[8px]">{{ displayLabel.slice(0, 2).toUpperCase() }}</AvatarFallback>
                      </Avatar>
                      <span class="truncate">
                        {{ displayLabel || $t('bots.access.selectConversationSource') }}
                      </span>
                    </span>
                    <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground/60" />
                  </Button>
                </template>
                <template #option-label="{ option }">
                  <div class="flex min-w-0 flex-1 items-center gap-2 text-left py-1">
                    <Avatar
                      v-if="observedConversationAvatar(option.meta)"
                      class="size-7 shrink-0 border border-border/40"
                    >
                      <AvatarImage
                        :src="observedConversationAvatar(option.meta)"
                        :alt="option.label"
                      />
                      <AvatarFallback class="text-[9px]">
                        {{ option.label.slice(0, 2).toUpperCase() }}
                      </AvatarFallback>
                    </Avatar>
                    <div class="min-w-0">
                      <div class="truncate text-[11px] font-medium">
                        {{ option.label }}
                      </div>
                      <div class="truncate text-[9px] text-muted-foreground/60 font-mono">
                        {{ buildConversationStableId(option.meta as AclObservedConversationCandidate | undefined) }}
                      </div>
                    </div>
                  </div>
                </template>
              </SearchableSelectPopover>
            </div>

            <p
              v-else
              class="text-[10px] text-muted-foreground/70 italic"
            >
              {{ $t('bots.access.pickTargetForConversationSearch') }}
            </p>

            <div
              v-if="hasConversationTarget"
              class="space-y-3"
            >
              <p class="text-[11px] font-medium text-muted-foreground">
                {{ $t('bots.access.manualConversationIds') }}
              </p>
              <div class="grid gap-3 sm:grid-cols-2">
                <div class="space-y-1.5">
                  <Label class="text-[10px] font-medium text-muted-foreground">{{ $t('bots.access.conversationId') }}</Label>
                  <Input
                    v-model="ruleForm.sourceConversationId"
                    class="h-8 text-[11px] bg-background/50 border-border/50 focus-visible:ring-ring/30 shadow-none"
                    :placeholder="$t('bots.access.conversationIdPlaceholder')"
                  />
                </div>
                <div
                  v-if="ruleForm.sourceConversationType === 'thread'"
                  class="space-y-1.5"
                >
                  <Label class="text-[10px] font-medium text-muted-foreground">{{ $t('bots.access.threadId') }}</Label>
                  <Input
                    v-model="ruleForm.sourceThreadId"
                    class="h-8 text-[11px] bg-background/50 border-border/50 focus-visible:ring-ring/30 shadow-none"
                    :placeholder="$t('bots.access.threadIdPlaceholder')"
                  />
                </div>
              </div>
            </div>

            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-7 px-2 text-[10px] text-muted-foreground hover:text-foreground border-border/50 bg-transparent shadow-none"
              @click="clearScopeFields"
            >
              <X class="size-3 mr-1" />
              {{ $t('bots.access.clearSpecificConversation') }}
            </Button>
          </section>

          <!-- Description -->
          <div class="space-y-1.5">
            <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.access.description') }}</Label>
            <Input
              v-model="ruleForm.description"
              class="h-8 text-[11px] bg-background/50 border-border/50 focus-visible:ring-ring/30 shadow-none"
              :placeholder="$t('bots.access.descriptionPlaceholder')"
            />
          </div>

          <p
            v-if="formError"
            class="text-[10px] text-destructive bg-destructive/5 px-2 py-1 rounded"
          >
            {{ formError }}
          </p>

          <div class="flex justify-end gap-2 pt-2 border-t border-border/30">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              class="h-8 px-4 text-[11px]"
              @click="formVisible = false"
            >
              {{ $t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              variant="outline"
              size="sm"
              class="h-8 px-4 text-[11px]"
              :disabled="isSavingRule"
            >
              <Spinner
                v-if="savingRuleAction === 'save'"
                class="mr-1.5 size-3.5"
              />
              {{ editingRule ? $t('common.save') : $t('bots.access.saveOnly') }}
            </Button>
            <Button
              v-if="!editingRule"
              type="button"
              size="sm"
              class="h-8 px-4 text-[11px] shadow-none bg-foreground text-background hover:bg-foreground/90"
              :disabled="isSavingRule"
              @click="handleSaveRule(true)"
            >
              <Spinner
                v-if="savingRuleAction === 'enable'"
                class="mr-1.5 size-3.5"
              />
              {{ $t('bots.access.saveAndEnable') }}
            </Button>
          </div>
        </form>
      </section>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Plus,
  SquarePen,
  Trash2,
  X,
  Search,
  Users,
  Power,
  ShieldCheck,
  ShieldAlert,
} from 'lucide-vue-next'
import {
  Button,
  Input,
  Label,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Spinner,
  Empty,
  Badge,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { channelTypeDisplayName } from '@/utils/channel-type-label'
import type { AclObservedConversationCandidate, AclRule, AclSourceScope, HandlersChannelMeta } from '@memohai/sdk'
import { formatRelativeTime } from '@/utils/date-time'
import {
  getChannels,
  getBotsByBotIdAclRules,
  getBotsByBotIdAclDefaultEffect,
  putBotsByBotIdAclDefaultEffect,
  postBotsByBotIdAclRules,
  putBotsByBotIdAclRulesByRuleId,
  deleteBotsByBotIdAclRulesByRuleId,
  getBotsByBotIdAclChannelIdentities,
  getBotsByBotIdAclChannelIdentitiesByChannelIdentityIdConversations,
  getBotsByBotIdAclChannelTypesByChannelTypeConversations,
} from '@memohai/sdk'

// ---- props ----

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

// ---- constants ----

const chatScopeOptions = computed(() => [
  { value: '', label: t('bots.access.chatScopeAny') },
  { value: 'private', label: describeConversationEffect(listEntryEffect.value, 'private') },
  { value: 'group', label: describeConversationEffect(listEntryEffect.value, 'group') },
  { value: 'thread', label: describeConversationEffect(listEntryEffect.value, 'thread') },
])

const aclExcludedChannelTypes = new Set(['web'])

// ---- queries ----

const { data: rulesData, isLoading: isLoadingRules } = useQuery({
  key: () => ['bot-acl-rules', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAclRules({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: channelMetas } = useQuery({
  key: () => ['channels'],
  query: async (): Promise<HandlersChannelMeta[]> => {
    const { data } = await getChannels({ throwOnError: true })
    return data ?? []
  },
})

const { data: defaultEffectData } = useQuery({
  key: () => ['bot-acl-default-effect', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAclDefaultEffect({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: identityCandidates } = useQuery({
  key: () => ['bot-acl-identities', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAclChannelIdentities({
      path: { bot_id: props.botId },
      query: { limit: 100 },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

interface RuleForm {
  enabled: boolean
  effect: string
  subjectChannelType: string
  channelIdentityId: string
  observedConversationRouteId: string
  sourceConversationType: string
  sourceConversationId: string
  sourceThreadId: string
  description: string
}

function createRuleForm(effect = 'deny'): RuleForm {
  return {
    enabled: false,
    effect,
    subjectChannelType: '',
    channelIdentityId: '',
    observedConversationRouteId: '',
    sourceConversationType: '',
    sourceConversationId: '',
    sourceThreadId: '',
    description: '',
  }
}

const ruleForm = reactive(createRuleForm())

const dialogIdentityId = computed(() =>
  ruleForm.channelIdentityId.trim(),
)

const dialogChannelTypeTrimmed = computed(() =>
  ruleForm.subjectChannelType.trim(),
)

const { data: observedByIdentityData, isLoading: isLoadingObservedIdentity } = useQuery({
  key: () => ['bot-acl-observed', props.botId, dialogIdentityId.value],
  query: async () => {
    const { data } = await getBotsByBotIdAclChannelIdentitiesByChannelIdentityIdConversations({
      path: { bot_id: props.botId, channel_identity_id: dialogIdentityId.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId && !!dialogIdentityId.value,
})

const { data: observedByChannelTypeData, isLoading: isLoadingObservedChannelType } = useQuery({
  key: () => ['bot-acl-observed-channel-type', props.botId, dialogChannelTypeTrimmed.value],
  query: async () => {
    const { data } = await getBotsByBotIdAclChannelTypesByChannelTypeConversations({
      path: { bot_id: props.botId, channel_type: dialogChannelTypeTrimmed.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId && !!dialogChannelTypeTrimmed.value,
})

/** Active observed-conversation list for the current subject (identity or platform type). */
const observedConversationsForRule = computed(() => {
  if (dialogIdentityId.value) {
    return observedByIdentityData.value
  }
  if (dialogChannelTypeTrimmed.value) {
    return observedByChannelTypeData.value
  }
  return undefined
})

const showConversationSearch = computed(
  () => !!dialogIdentityId.value || !!dialogChannelTypeTrimmed.value,
)

const observedConversationEmptyText = computed(() => {
  if (dialogIdentityId.value && isLoadingObservedIdentity.value) {
    return t('common.loading')
  }
  if (!dialogIdentityId.value && dialogChannelTypeTrimmed.value && isLoadingObservedChannelType.value) {
    return t('common.loading')
  }
  return t('bots.access.noObservedConversations')
})

// ---- derived ----

const rules = computed(() => rulesData.value?.items ?? [])
const identities = computed(() => identityCandidates.value?.items ?? [])
const platformMetaByType = computed(() => {
  const items = new Map<string, HandlersChannelMeta>()
  for (const meta of channelMetas.value ?? []) {
    const type = meta.type?.trim()
    if (type) items.set(type, meta)
  }
  return items
})
const platformOptions = computed(() =>
  [...platformMetaByType.value.values()]
    .map(meta => ({
      value: meta.type?.trim() ?? '',
      label: formatPlatformName(meta.type, meta.display_name),
    }))
    .filter(option => option.value && !aclExcludedChannelTypes.has(option.value))
    .sort((a, b) => a.label.localeCompare(b.label)),
)
const isBlacklistMode = computed(() => defaultEffectDraft.value === 'allow')
const listEntryEffect = computed(() => isBlacklistMode.value ? 'deny' : 'allow')
const listTitle = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.blacklistTitle')
    : t('bots.access.whitelistTitle'),
)
const listDescription = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.blacklistDescription')
    : t('bots.access.whitelistDescription'),
)
const addListEntryLabel = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.addBlacklistEntry')
    : t('bots.access.addWhitelistEntry'),
)
const emptyTitle = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.blacklistEmpty')
    : t('bots.access.whitelistEmpty'),
)
const emptyDescription = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.blacklistEmptyDescription')
    : t('bots.access.whitelistEmptyDescription'),
)
const selectedIdentityLabel = computed(() =>
  selectedIdentityOption.value?.label ?? '',
)
const selectedIdentityOption = computed(() =>
  identityOptions.value.find(option => option.value === ruleForm.channelIdentityId),
)
const selectedPlatformLabel = computed(() =>
  ruleForm.subjectChannelType
    ? formatPlatformName(ruleForm.subjectChannelType)
    : '',
)
const hasConversationTarget = computed(() =>
  !!dialogIdentityId.value || !!dialogChannelTypeTrimmed.value,
)
const showSpecificConversationSection = computed(() =>
  ruleForm.sourceConversationType === 'group'
  || ruleForm.sourceConversationType === 'thread'
  || !!ruleForm.sourceConversationId
  || !!ruleForm.sourceThreadId,
)
const ruleTargetPreview = computed(() => {
  const platform = selectedPlatformLabel.value
  const user = selectedIdentityLabel.value
  if (platform && user) {
    return t('bots.access.platformUserTargetPreview', { platform, user })
  }
  if (platform) {
    return t('bots.access.platformTargetPreview', { platform })
  }
  if (user) {
    return t('bots.access.userTargetPreview', { user })
  }
  return t('bots.access.subjectAllLabel')
})
const ruleScopePreview = computed(() => {
  if (ruleForm.sourceConversationId || ruleForm.sourceThreadId) {
    return t('bots.access.previewScopeSpecific')
  }
  switch (ruleForm.sourceConversationType) {
    case 'private':
      return t('bots.access.previewScopePrivate')
    case 'group':
      return t('bots.access.previewScopeGroup')
    case 'thread':
      return t('bots.access.previewScopeThread')
    default:
      return t('bots.access.previewScopeAny')
  }
})
const rulePreviewText = computed(() =>
  isBlacklistMode.value
    ? t('bots.access.blacklistPreview', {
        target: ruleTargetPreview.value,
        scope: ruleScopePreview.value,
      })
    : t('bots.access.whitelistPreview', {
        target: ruleTargetPreview.value,
        scope: ruleScopePreview.value,
      }),
)
const visibleRules = computed(() =>
  rules.value.filter(rule => rule.effect === listEntryEffect.value),
)

const identityOptions = computed(() =>
  identities.value
    .filter(i => !aclExcludedChannelTypes.has(i.channel ?? ''))
    .map(i => ({
      value: i.id ?? '',
      label: i.display_name || i.channel_subject_id || i.id || '',
      meta: {
        avatarUrl: i.avatar_url,
        channel: i.channel,
        channelLabel: formatPlatformName(i.channel),
      },
    })),
)

const filteredIdentityOptions = computed(() => {
  const platform = dialogChannelTypeTrimmed.value
  if (!platform) return identityOptions.value
  return identityOptions.value.filter(option => option.meta.channel === platform)
})

function formatIdentityOptionSubtitle(meta?: { channelLabel?: string }): string {
  return meta?.channelLabel ?? ''
}

const observedConversationOptions = computed(() =>
  (observedConversationsForRule.value?.items ?? []).filter(conversationMatchesSelectedScope).map((c) => {
    const label = buildConversationLabel(c)
    const keywords = [
      c.conversation_name,
      c.conversation_id,
      c.thread_id,
      c.channel,
      c.conversation_type,
    ].filter((x): x is string => Boolean(x && String(x).trim()))
    return {
      value: c.route_id ?? '',
      label,
      description: c.last_observed_at ? formatRelativeTime(c.last_observed_at) : undefined,
      group: c.conversation_type || 'unknown',
      groupLabel: describeObservedConversationType(c.conversation_type),
      keywords,
      meta: c,
    }
  }),
)

function conversationMatchesSelectedScope(c: AclObservedConversationCandidate): boolean {
  const scope = ruleForm.sourceConversationType
  if (scope !== 'group' && scope !== 'thread') {
    return true
  }
  return c.conversation_type === scope
}

/** Primary display label: name when available, stable ID otherwise. */
function buildConversationLabel(c: AclObservedConversationCandidate | undefined): string {
  if (!c) return ''
  const name = c.conversation_name?.trim()
  if (name) return name
  return c.conversation_id || c.route_id || ''
}

/** Subtitle always shows the stable platform identifiers for verification. */
function buildConversationStableId(c: AclObservedConversationCandidate | undefined): string {
  if (!c) return ''
  const parts: string[] = []
  if (c.channel) parts.push(c.channel)
  if (c.conversation_type) parts.push(describeObservedConversationType(c.conversation_type))
  if (c.conversation_id) parts.push(c.conversation_id)
  if (c.thread_id) parts.push(`thread:${c.thread_id}`)
  return parts.join(' · ')
}

function observedConversationAvatar(meta: unknown): string {
  const item = meta as AclObservedConversationCandidate | undefined
  return item?.conversation_avatar_url?.trim() ?? ''
}

function onConversationSourceChange(routeId: string) {
  ruleForm.observedConversationRouteId = routeId
  if (!routeId.trim()) {
    ruleForm.sourceConversationType = ''
    ruleForm.sourceConversationId = ''
    ruleForm.sourceThreadId = ''
    return
  }
  applyObservedConversation(routeId)
}

// ---- default effect ----

const defaultEffectDraft = ref('allow')
const isSavingDefaultEffect = ref(false)

watch(defaultEffectData, (data) => {
  if (data?.default_effect) {
    defaultEffectDraft.value = data.default_effect
  }
}, { immediate: true })

async function handleSetDefaultEffect(effect: string) {
  const previousEffect = defaultEffectDraft.value
  if (effect === previousEffect || isSavingDefaultEffect.value) return
  defaultEffectDraft.value = effect
  isSavingDefaultEffect.value = true
  try {
    await putBotsByBotIdAclDefaultEffect({
      path: { bot_id: props.botId },
      body: { default_effect: effect },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-acl-default-effect', props.botId] })
    toast.success(t('bots.access.defaultEffectSaved'))
  }
  catch (e) {
    defaultEffectDraft.value = previousEffect
    toast.error(resolveApiErrorMessage(e, t('bots.access.saveFailed')))
  }
  finally {
    isSavingDefaultEffect.value = false
  }
}

// ---- rule form ----

const formVisible = ref(false)
const editingRule = ref<AclRule | null>(null)
const formError = ref('')
const savingRuleAction = ref<'save' | 'enable' | ''>('')
const isSavingRule = computed(() => savingRuleAction.value !== '')

watch(
  () => [
    formVisible.value,
    dialogIdentityId.value,
    dialogChannelTypeTrimmed.value,
    ruleForm.sourceConversationType,
    ruleForm.sourceConversationId,
    ruleForm.sourceThreadId,
    observedByIdentityData.value,
    observedByChannelTypeData.value,
  ] as const,
  () => {
    if (!formVisible.value) return
    const hasIdentity = !!dialogIdentityId.value
    const hasChannelType = !!dialogChannelTypeTrimmed.value
    if (!hasIdentity && !hasChannelType) return
    const items = hasIdentity
      ? (observedByIdentityData.value?.items ?? [])
      : (observedByChannelTypeData.value?.items ?? [])
    const match = items.find(
      c =>
        (c.conversation_type ?? '') === (ruleForm.sourceConversationType ?? '')
        && (c.conversation_id ?? '') === (ruleForm.sourceConversationId ?? '')
        && (c.thread_id ?? '') === (ruleForm.sourceThreadId ?? ''),
    )
    const nextRoute = match?.route_id ?? ''
    if (nextRoute !== ruleForm.observedConversationRouteId) {
      ruleForm.observedConversationRouteId = nextRoute
    }
  },
)

watch(
  () => ruleForm.channelIdentityId,
  (id, prev) => {
    if (!formVisible.value) return
    if (prev !== undefined && prev !== '' && id !== prev) {
      ruleForm.observedConversationRouteId = ''
      ruleForm.sourceConversationType = ''
      ruleForm.sourceConversationId = ''
      ruleForm.sourceThreadId = ''
    }
  },
)

watch(
  () => ruleForm.subjectChannelType,
  (channelType, prev) => {
    if (!formVisible.value) return
    if (ruleForm.channelIdentityId) {
      const selected = identityOptions.value.find(option => option.value === ruleForm.channelIdentityId)
      if (channelType && selected?.meta.channel !== channelType) {
        ruleForm.channelIdentityId = ''
      }
    }
    if (prev !== undefined && prev.trim() !== '' && channelType !== prev) {
      ruleForm.observedConversationRouteId = ''
      ruleForm.sourceConversationType = ''
      ruleForm.sourceConversationId = ''
      ruleForm.sourceThreadId = ''
    }
  },
)

watch(listEntryEffect, (effect) => {
  if (formVisible.value && !editingRule.value) {
    ruleForm.effect = effect
  }
})

function openAddDialog() {
  editingRule.value = null
  Object.assign(ruleForm, createRuleForm(listEntryEffect.value))
  formError.value = ''
  formVisible.value = true
}

function openEditDialog(rule: AclRule) {
  editingRule.value = rule
  ruleForm.enabled = rule.enabled ?? true
  ruleForm.effect = rule.effect ?? 'deny'
  ruleForm.subjectChannelType = rule.subject_channel_type ?? ''
  ruleForm.channelIdentityId = rule.channel_identity_id ?? ''
  ruleForm.observedConversationRouteId = ''
  ruleForm.sourceConversationType = rule.source_scope?.conversation_type ?? ''
  ruleForm.sourceConversationId = rule.source_scope?.conversation_id ?? ''
  ruleForm.sourceThreadId = rule.source_scope?.thread_id ?? ''
  ruleForm.description = rule.description ?? ''
  formError.value = ''
  formVisible.value = true
}

function setChatScope(scope: string) {
  if (scope === '' || scope === 'private' || scope !== ruleForm.sourceConversationType) {
    ruleForm.observedConversationRouteId = ''
    ruleForm.sourceConversationId = ''
    ruleForm.sourceThreadId = ''
  }
  ruleForm.sourceConversationType = scope
}

function setPlatformScope(channelType: string) {
  ruleForm.subjectChannelType = channelType
}

function setChannelIdentity(identityId: string) {
  ruleForm.channelIdentityId = identityId
  ruleForm.observedConversationRouteId = ''
  ruleForm.sourceConversationType = ''
  ruleForm.sourceConversationId = ''
  ruleForm.sourceThreadId = ''
}

function clearScopeFields() {
  ruleForm.observedConversationRouteId = ''
  ruleForm.sourceConversationType = ''
  ruleForm.sourceConversationId = ''
  ruleForm.sourceThreadId = ''
}

function applyObservedConversation(routeId: string) {
  const item = (observedConversationsForRule.value?.items ?? []).find(c => c.route_id === routeId)
  if (!item) return
  ruleForm.sourceConversationType = item.conversation_type ?? ''
  ruleForm.sourceConversationId = item.conversation_id ?? ''
  ruleForm.sourceThreadId = item.thread_id ?? ''
}

function buildSourceScope(): AclSourceScope | undefined {
  const scope: AclSourceScope = {}
  if (ruleForm.sourceConversationType) scope.conversation_type = ruleForm.sourceConversationType
  if (ruleForm.sourceConversationId) scope.conversation_id = ruleForm.sourceConversationId
  if (ruleForm.sourceThreadId) scope.thread_id = ruleForm.sourceThreadId
  if (!scope.conversation_type && !scope.conversation_id && !scope.thread_id) {
    return undefined
  }
  return scope
}

async function handleSaveRule(enable: boolean) {
  formError.value = ''
  savingRuleAction.value = enable ? 'enable' : 'save'
  try {
    const body = {
      enabled: enable ? true : ruleForm.enabled,
      effect: ruleForm.effect,
      channel_identity_id: ruleForm.channelIdentityId || undefined,
      subject_channel_type: ruleForm.subjectChannelType || undefined,
      source_scope: buildSourceScope(),
      description: ruleForm.description || undefined,
    }
    if (editingRule.value?.id) {
      await putBotsByBotIdAclRulesByRuleId({
        path: { bot_id: props.botId, rule_id: editingRule.value.id },
        body,
        throwOnError: true,
      })
    }
    else {
      await postBotsByBotIdAclRules({
        path: { bot_id: props.botId },
        body,
        throwOnError: true,
      })
    }
    queryCache.invalidateQueries({ key: ['bot-acl-rules', props.botId] })
    toast.success(t('bots.access.ruleSaved'))
    formVisible.value = false
  }
  catch (e) {
    formError.value = resolveApiErrorMessage(e, t('bots.access.saveFailed'))
  }
  finally {
    savingRuleAction.value = ''
  }
}

async function handleDeleteRule(ruleId: string) {
  try {
    await deleteBotsByBotIdAclRulesByRuleId({
      path: { bot_id: props.botId, rule_id: ruleId },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-acl-rules', props.botId] })
    toast.success(t('bots.access.deleteSuccess'))
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.deleteFailed')))
  }
}

async function handleToggleEnabled(rule: AclRule, enabled: boolean) {
  try {
    await putBotsByBotIdAclRulesByRuleId({
      path: { bot_id: props.botId, rule_id: rule.id! },
      body: {
        enabled,
        effect: rule.effect ?? 'deny',
        channel_identity_id: rule.channel_identity_id,
        subject_channel_type: rule.subject_channel_type,
        source_scope: rule.source_scope,
        description: rule.description,
      },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-acl-rules', props.botId] })
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.saveFailed')))
  }
}

// ---- display helpers ----

function describeRuleTarget(rule: AclRule): string {
  return describeSubject(rule)
}

function describeSubject(rule: AclRule): string {
  const platformType = rule.subject_channel_type || rule.channel_type
  const platform = platformType ? formatPlatformName(platformType) : ''
  const user = rule.channel_identity_display_name || rule.channel_subject_id || rule.channel_identity_id || ''
  if (rule.subject_channel_type && rule.channel_identity_id) {
    return t('bots.access.platformUserTargetPreview', { platform, user: user || '?' })
  }
  if (rule.subject_channel_type) {
    return t('bots.access.platformTargetPreview', { platform })
  }
  if (rule.channel_identity_id) {
    return t('bots.access.userTargetPreview', { user: user || '?' })
  }
  return t('bots.access.subjectAllLabel')
}

function formatPlatformName(type?: string | null, displayName?: string | null): string {
  const raw = type?.trim() ?? ''
  const meta = raw ? platformMetaByType.value.get(raw) : undefined
  return channelTypeDisplayName(t, raw, displayName ?? meta?.display_name)
}

function ruleTargetFallback(rule: AclRule): string {
  const label = describeRuleTarget(rule).trim()
  if (!label) return '?'
  return label.slice(0, 2).toUpperCase()
}

function ruleScopeFallback(rule: AclRule): string {
  const label = rule.source_conversation_name || rule.source_scope?.conversation_id || ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function ruleScopePrefix(rule: AclRule): string {
  const scope = rule.source_scope
  if (!scope) return t('bots.access.chatScopeAny')

  return scope.conversation_type
    ? describeConversationEffect(rule.effect ?? '', scope.conversation_type)
    : t('bots.access.chatScopeAny')
}

function ruleScopeDetail(rule: AclRule): string {
  const scope = rule.source_scope
  if (!scope) return ''

  const conversationID = scope.conversation_id?.trim()
  if (!conversationID) return ''

  const name = rule.source_conversation_name?.trim()
  const displayName = name ? `${name} (${conversationID})` : conversationID
  const thread = scope.thread_id ? ` · thread:${scope.thread_id}` : ''
  return `${displayName}${thread}`
}

function describeConversationEffect(effect: string, type: string): string {
  const normalizedEffect = effect === 'allow' ? 'allow' : 'deny'
  switch (type) {
    case 'private':
      return normalizedEffect === 'allow'
        ? t('bots.access.allowPrivateConversation')
        : t('bots.access.denyPrivateConversation')
    case 'group':
      return normalizedEffect === 'allow'
        ? t('bots.access.allowGroupConversation')
        : t('bots.access.denyGroupConversation')
    case 'thread':
      return normalizedEffect === 'allow'
        ? t('bots.access.allowThreadConversation')
        : t('bots.access.denyThreadConversation')
    default:
      return type
  }
}

function describeObservedConversationType(type?: string): string {
  switch (type) {
    case 'private':
      return t('bots.access.privateConversationGroup')
    case 'group':
      return t('bots.access.groupConversationGroup')
    case 'thread':
      return t('bots.access.threadConversationGroup')
    default:
      return t('bots.access.unknownConversationGroup')
  }
}
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
