<template>
  <div class="max-w-3xl mx-auto space-y-6">
    <div class="space-y-1">
      <h2 class="text-lg font-semibold text-foreground">
        {{ $t('bots.access.title') }}
      </h2>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.access.subtitle') }}
      </p>
    </div>

    <section class="rounded-lg border border-border bg-card p-4 space-y-4">
      <div class="flex items-start justify-between gap-4">
        <div class="space-y-1">
          <p class="text-sm font-medium text-foreground">
            {{ $t('bots.settings.allowGuest') }}
          </p>
          <p class="text-sm text-muted-foreground">
            {{ $t('bots.access.allowGuestDescription') }}
          </p>
          <p
            v-if="isPersonalBot"
            class="text-xs text-muted-foreground"
          >
            {{ $t('bots.settings.allowGuestPersonalHint') }}
          </p>
        </div>
        <Switch
          :model-value="allowGuestDraft"
          :disabled="isPersonalBot || isSavingGuestAccess"
          @update:model-value="(val) => allowGuestDraft = !!val"
        />
      </div>
      <div class="flex justify-end">
        <Button
          :disabled="isPersonalBot || !hasGuestAccessChanges || isSavingGuestAccess"
          @click="handleSaveGuestAccess"
        >
          <Spinner
            v-if="isSavingGuestAccess"
            class="mr-1.5"
          />
          {{ $t('bots.access.saveGuestAccess') }}
        </Button>
      </div>
    </section>

    <div class="rounded-lg border border-border bg-card p-4 space-y-2">
      <p class="text-sm font-medium text-foreground">
        {{ $t('bots.access.guestRulesTitle') }}
      </p>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.access.guestRulesDescription') }}
      </p>
    </div>

    <section class="rounded-lg border border-border bg-card p-4 space-y-4">
      <div>
        <h3 class="text-base font-semibold text-foreground">
          {{ $t('bots.access.whitelistTitle') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.access.whitelistDescription') }}
        </p>
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.access.userSelector') }}</Label>
          <SearchableSelectPopover
            v-model="whitelistSelection.userId"
            :options="userOptions"
            :placeholder="$t('bots.access.selectUser')"
            :aria-label="$t('bots.access.selectUser')"
            :search-placeholder="$t('bots.access.searchUser')"
            :search-aria-label="$t('bots.access.searchUser')"
            :empty-text="$t('bots.access.noUserCandidates')"
          >
            <template #option-label="{ option }">
              <div class="flex min-w-0 items-center gap-2 text-left">
                <Avatar class="size-7 shrink-0">
                  <AvatarImage
                    v-if="candidateAvatar(option.meta as AclUserCandidate | undefined)"
                    :src="candidateAvatar(option.meta as AclUserCandidate | undefined)"
                    :alt="option.label"
                  />
                  <AvatarFallback class="text-[10px]">
                    {{ initials(option.label) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <div class="truncate">
                    {{ option.label }}
                  </div>
                  <div class="truncate text-xs text-muted-foreground">
                    {{ option.description }}
                  </div>
                </div>
              </div>
            </template>
            <template #option-suffix>
              <span />
            </template>
          </SearchableSelectPopover>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.access.identitySelector') }}</Label>
          <SearchableSelectPopover
            v-model="whitelistSelection.channelIdentityId"
            :options="identityOptions"
            :placeholder="$t('bots.access.selectIdentity')"
            :aria-label="$t('bots.access.selectIdentity')"
            :search-placeholder="$t('bots.access.searchIdentity')"
            :search-aria-label="$t('bots.access.searchIdentity')"
            :empty-text="$t('bots.access.noIdentityCandidates')"
          >
            <template #option-label="{ option }">
              <div class="flex min-w-0 items-center gap-2 text-left">
                <Avatar class="size-7 shrink-0">
                  <AvatarImage
                    v-if="identityAvatar(option.meta as AclChannelIdentityCandidate | undefined)"
                    :src="identityAvatar(option.meta as AclChannelIdentityCandidate | undefined)"
                    :alt="option.label"
                  />
                  <AvatarFallback class="text-[10px]">
                    {{ initials(option.label) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <div class="truncate">
                    {{ option.label }}
                  </div>
                  <div class="truncate text-xs text-muted-foreground">
                    {{ option.description }}
                  </div>
                </div>
              </div>
            </template>
            <template #option-suffix>
              <span />
            </template>
          </SearchableSelectPopover>
        </div>
      </div>

      <div class="rounded-md border border-border bg-muted/40 p-4 space-y-3">
        <div class="space-y-1">
          <p class="text-sm font-medium text-foreground">
            {{ $t('bots.access.sourceScopeTitle') }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.access.sourceScopeDescription') }}
          </p>
        </div>

        <div class="grid gap-3 md:grid-cols-2">
          <div class="space-y-2">
            <Label>{{ $t('bots.access.sourceChannel') }}</Label>
            <div
              v-if="whitelistSelection.channelIdentityId"
              class="flex h-9 items-center rounded-md border border-border bg-background px-3 text-sm text-foreground"
            >
              {{ whitelistSelection.sourceChannel || $t('bots.access.anyChannel') }}
            </div>
            <NativeSelect
              v-else
              v-model="whitelistSelection.sourceChannel"
              :disabled="isObservedConversationLocked(whitelistSelection)"
              class="h-9 w-full text-sm"
            >
              <option value="">
                {{ $t('bots.access.anyChannel') }}
              </option>
              <option
                v-for="channel in knownChannels"
                :key="channel"
                :value="channel"
              >
                {{ channel }}
              </option>
            </NativeSelect>
          </div>

          <div class="space-y-2">
            <Label>{{ $t('bots.access.conversationType') }}</Label>
            <NativeSelect
              v-model="whitelistSelection.sourceConversationType"
              :disabled="isObservedConversationLocked(whitelistSelection)"
              class="h-9 w-full text-sm"
            >
              <option value="">
                {{ $t('bots.access.anyConversationType') }}
              </option>
              <option value="private">
                {{ $t('bots.access.privateConversationType') }}
              </option>
              <option value="group">
                {{ $t('bots.access.groupConversationType') }}
              </option>
              <option value="thread">
                {{ $t('bots.access.threadConversationType') }}
              </option>
            </NativeSelect>
          </div>

          <div
            v-if="whitelistSelection.sourceConversationType !== 'private'"
            class="space-y-2 md:col-span-2"
          >
            <Label>{{ $t('bots.access.observedConversation') }}</Label>
            <SearchableSelectPopover
              v-if="whitelistSelection.channelIdentityId"
              v-model="whitelistSelection.observedConversationRouteId"
              :options="whitelistObservedConversationOptions"
              :placeholder="$t('bots.access.selectObservedConversation')"
              :aria-label="$t('bots.access.selectObservedConversation')"
              :search-placeholder="$t('bots.access.searchObservedConversation')"
              :search-aria-label="$t('bots.access.searchObservedConversation')"
              :empty-text="$t('bots.access.noObservedConversations')"
            />
            <div
              v-else
              class="flex h-9 items-center rounded-md border border-border bg-background px-3 text-sm text-muted-foreground"
            >
              {{ $t('bots.access.selectIdentityFirst') }}
            </div>
            <p class="text-xs text-muted-foreground">
              <template v-if="isLoadingWhitelistObserved">
                {{ $t('common.loading') }}
              </template>
              <template v-else>
                {{ $t('bots.access.observedConversationHint') }}
              </template>
            </p>
          </div>

          <div
            v-if="whitelistSelection.sourceConversationType !== 'private'"
            class="space-y-2"
          >
            <Label>{{ $t('bots.access.conversationId') }}</Label>
            <Input
              v-model="whitelistSelection.sourceConversationId"
              :disabled="isObservedConversationLocked(whitelistSelection)"
              :placeholder="$t('bots.access.conversationIdPlaceholder')"
            />
          </div>

          <div
            v-if="whitelistSelection.sourceConversationType !== 'private'"
            class="space-y-2"
          >
            <Label>{{ $t('bots.access.threadId') }}</Label>
            <Input
              v-model="whitelistSelection.sourceThreadId"
              :disabled="isObservedConversationLocked(whitelistSelection)"
              :placeholder="$t('bots.access.threadIdPlaceholder')"
            />
          </div>
        </div>

        <div class="flex justify-end">
          <Button
            variant="outline"
            @click="clearSourceScope(whitelistSelection)"
          >
            {{ $t('bots.access.clearScope') }}
          </Button>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <Button
          variant="outline"
          @click="resetSelection(whitelistSelection)"
        >
          {{ $t('bots.access.clearSelection') }}
        </Button>
        <Button
          :disabled="isSavingWhitelist"
          @click="handleAddWhitelist"
        >
          <Spinner
            v-if="isSavingWhitelist"
            class="mr-1.5"
          />
          {{ $t('bots.access.addWhitelist') }}
        </Button>
      </div>

      <Separator />

      <div
        v-if="isLoadingWhitelist"
        class="text-sm text-muted-foreground"
      >
        {{ $t('common.loading') }}
      </div>
      <div
        v-else-if="whitelist.length === 0"
        class="text-sm text-muted-foreground"
      >
        {{ $t('bots.access.whitelistEmpty') }}
      </div>
      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="item in whitelist"
          :key="item.id"
          class="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
        >
          <div class="flex min-w-0 items-center gap-3">
            <div class="relative shrink-0">
              <Avatar class="size-9 shrink-0">
                <AvatarImage
                  v-if="ruleAvatar(item)"
                  :src="ruleAvatar(item)"
                  :alt="formatRuleLabel(item)"
                />
                <AvatarFallback class="text-xs">
                  {{ initials(formatRuleLabel(item)) }}
                </AvatarFallback>
              </Avatar>
              <ChannelBadge
                v-if="rulePlatform(item)"
                :platform="rulePlatform(item)"
              />
            </div>
            <div class="min-w-0">
              <div class="truncate text-sm font-medium text-foreground">
                {{ formatRuleLabel(item) }}
              </div>
              <div class="truncate text-xs text-muted-foreground">
                {{ formatRuleMeta(item) }}
              </div>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            :disabled="deletingRuleId === item.id"
            @click="handleDeleteWhitelist(item.id)"
          >
            {{ $t('common.delete') }}
          </Button>
        </div>
      </div>
    </section>

    <section class="rounded-lg border border-border bg-card p-4 space-y-4">
      <div>
        <h3 class="text-base font-semibold text-foreground">
          {{ $t('bots.access.blacklistTitle') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.access.blacklistDescription') }}
        </p>
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.access.userSelector') }}</Label>
          <SearchableSelectPopover
            v-model="blacklistSelection.userId"
            :options="userOptions"
            :placeholder="$t('bots.access.selectUser')"
            :aria-label="$t('bots.access.selectUser')"
            :search-placeholder="$t('bots.access.searchUser')"
            :search-aria-label="$t('bots.access.searchUser')"
            :empty-text="$t('bots.access.noUserCandidates')"
          >
            <template #option-label="{ option }">
              <div class="flex min-w-0 items-center gap-2 text-left">
                <Avatar class="size-7 shrink-0">
                  <AvatarImage
                    v-if="candidateAvatar(option.meta as AclUserCandidate | undefined)"
                    :src="candidateAvatar(option.meta as AclUserCandidate | undefined)"
                    :alt="option.label"
                  />
                  <AvatarFallback class="text-[10px]">
                    {{ initials(option.label) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <div class="truncate">
                    {{ option.label }}
                  </div>
                  <div class="truncate text-xs text-muted-foreground">
                    {{ option.description }}
                  </div>
                </div>
              </div>
            </template>
            <template #option-suffix>
              <span />
            </template>
          </SearchableSelectPopover>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.access.identitySelector') }}</Label>
          <SearchableSelectPopover
            v-model="blacklistSelection.channelIdentityId"
            :options="identityOptions"
            :placeholder="$t('bots.access.selectIdentity')"
            :aria-label="$t('bots.access.selectIdentity')"
            :search-placeholder="$t('bots.access.searchIdentity')"
            :search-aria-label="$t('bots.access.searchIdentity')"
            :empty-text="$t('bots.access.noIdentityCandidates')"
          >
            <template #option-label="{ option }">
              <div class="flex min-w-0 items-center gap-2 text-left">
                <Avatar class="size-7 shrink-0">
                  <AvatarImage
                    v-if="identityAvatar(option.meta as AclChannelIdentityCandidate | undefined)"
                    :src="identityAvatar(option.meta as AclChannelIdentityCandidate | undefined)"
                    :alt="option.label"
                  />
                  <AvatarFallback class="text-[10px]">
                    {{ initials(option.label) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <div class="truncate">
                    {{ option.label }}
                  </div>
                  <div class="truncate text-xs text-muted-foreground">
                    {{ option.description }}
                  </div>
                </div>
              </div>
            </template>
            <template #option-suffix>
              <span />
            </template>
          </SearchableSelectPopover>
        </div>
      </div>

      <div class="rounded-md border border-border bg-muted/40 p-4 space-y-3">
        <div class="space-y-1">
          <p class="text-sm font-medium text-foreground">
            {{ $t('bots.access.sourceScopeTitle') }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.access.sourceScopeDescription') }}
          </p>
        </div>

        <div class="grid gap-3 md:grid-cols-2">
          <div class="space-y-2">
            <Label>{{ $t('bots.access.sourceChannel') }}</Label>
            <div
              v-if="blacklistSelection.channelIdentityId"
              class="flex h-9 items-center rounded-md border border-border bg-background px-3 text-sm text-foreground"
            >
              {{ blacklistSelection.sourceChannel || $t('bots.access.anyChannel') }}
            </div>
            <NativeSelect
              v-else
              v-model="blacklistSelection.sourceChannel"
              :disabled="isObservedConversationLocked(blacklistSelection)"
              class="h-9 w-full text-sm"
            >
              <option value="">
                {{ $t('bots.access.anyChannel') }}
              </option>
              <option
                v-for="channel in knownChannels"
                :key="channel"
                :value="channel"
              >
                {{ channel }}
              </option>
            </NativeSelect>
          </div>

          <div class="space-y-2">
            <Label>{{ $t('bots.access.conversationType') }}</Label>
            <NativeSelect
              v-model="blacklistSelection.sourceConversationType"
              :disabled="isObservedConversationLocked(blacklistSelection)"
              class="h-9 w-full text-sm"
            >
              <option value="">
                {{ $t('bots.access.anyConversationType') }}
              </option>
              <option value="private">
                {{ $t('bots.access.privateConversationType') }}
              </option>
              <option value="group">
                {{ $t('bots.access.groupConversationType') }}
              </option>
              <option value="thread">
                {{ $t('bots.access.threadConversationType') }}
              </option>
            </NativeSelect>
          </div>

          <div
            v-if="blacklistSelection.sourceConversationType !== 'private'"
            class="space-y-2 md:col-span-2"
          >
            <Label>{{ $t('bots.access.observedConversation') }}</Label>
            <SearchableSelectPopover
              v-if="blacklistSelection.channelIdentityId"
              v-model="blacklistSelection.observedConversationRouteId"
              :options="blacklistObservedConversationOptions"
              :placeholder="$t('bots.access.selectObservedConversation')"
              :aria-label="$t('bots.access.selectObservedConversation')"
              :search-placeholder="$t('bots.access.searchObservedConversation')"
              :search-aria-label="$t('bots.access.searchObservedConversation')"
              :empty-text="$t('bots.access.noObservedConversations')"
            />
            <div
              v-else
              class="flex h-9 items-center rounded-md border border-border bg-background px-3 text-sm text-muted-foreground"
            >
              {{ $t('bots.access.selectIdentityFirst') }}
            </div>
            <p class="text-xs text-muted-foreground">
              <template v-if="isLoadingBlacklistObserved">
                {{ $t('common.loading') }}
              </template>
              <template v-else>
                {{ $t('bots.access.observedConversationHint') }}
              </template>
            </p>
          </div>

          <div
            v-if="blacklistSelection.sourceConversationType !== 'private'"
            class="space-y-2"
          >
            <Label>{{ $t('bots.access.conversationId') }}</Label>
            <Input
              v-model="blacklistSelection.sourceConversationId"
              :disabled="isObservedConversationLocked(blacklistSelection)"
              :placeholder="$t('bots.access.conversationIdPlaceholder')"
            />
          </div>

          <div
            v-if="blacklistSelection.sourceConversationType !== 'private'"
            class="space-y-2"
          >
            <Label>{{ $t('bots.access.threadId') }}</Label>
            <Input
              v-model="blacklistSelection.sourceThreadId"
              :disabled="isObservedConversationLocked(blacklistSelection)"
              :placeholder="$t('bots.access.threadIdPlaceholder')"
            />
          </div>
        </div>

        <div class="flex justify-end">
          <Button
            variant="outline"
            @click="clearSourceScope(blacklistSelection)"
          >
            {{ $t('bots.access.clearScope') }}
          </Button>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <Button
          variant="outline"
          @click="resetSelection(blacklistSelection)"
        >
          {{ $t('bots.access.clearSelection') }}
        </Button>
        <Button
          :disabled="isSavingBlacklist"
          @click="handleAddBlacklist"
        >
          <Spinner
            v-if="isSavingBlacklist"
            class="mr-1.5"
          />
          {{ $t('bots.access.addBlacklist') }}
        </Button>
      </div>

      <Separator />

      <div
        v-if="isLoadingBlacklist"
        class="text-sm text-muted-foreground"
      >
        {{ $t('common.loading') }}
      </div>
      <div
        v-else-if="blacklist.length === 0"
        class="text-sm text-muted-foreground"
      >
        {{ $t('bots.access.blacklistEmpty') }}
      </div>
      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="item in blacklist"
          :key="item.id"
          class="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
        >
          <div class="flex min-w-0 items-center gap-3">
            <div class="relative shrink-0">
              <Avatar class="size-9 shrink-0">
                <AvatarImage
                  v-if="ruleAvatar(item)"
                  :src="ruleAvatar(item)"
                  :alt="formatRuleLabel(item)"
                />
                <AvatarFallback class="text-xs">
                  {{ initials(formatRuleLabel(item)) }}
                </AvatarFallback>
              </Avatar>
              <ChannelBadge
                v-if="rulePlatform(item)"
                :platform="rulePlatform(item)"
              />
            </div>
            <div class="min-w-0">
              <div class="truncate text-sm font-medium text-foreground">
                {{ formatRuleLabel(item) }}
              </div>
              <div class="truncate text-xs text-muted-foreground">
                {{ formatRuleMeta(item) }}
              </div>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            :disabled="deletingRuleId === item.id"
            @click="handleDeleteBlacklist(item.id)"
          >
            {{ $t('common.delete') }}
          </Button>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Avatar, AvatarFallback, AvatarImage, Button, Input, Label, NativeSelect, Separator, Spinner, Switch } from '@memohai/ui'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import {
  getBotsByBotIdAccessChannelIdentities,
  getBotsByBotIdAccessChannelIdentitiesByChannelIdentityIdConversations,
  getBotsByBotIdAccessUsers,
  deleteBotsByBotIdBlacklistByRuleId,
  deleteBotsByBotIdWhitelistByRuleId,
  getBotsByBotIdBlacklist,
  getBotsByBotIdSettings,
  getBotsByBotIdWhitelist,
  putBotsByBotIdBlacklist,
  putBotsByBotIdSettings,
  putBotsByBotIdWhitelist,
} from '@memohai/sdk'
import type {
  AclChannelIdentityCandidate,
  AclObservedConversationCandidate,
  AclRule,
  AclSourceScope,
  AclUpsertRuleRequest,
  AclUserCandidate,
} from '@memohai/sdk'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatRelativeTime } from '@/utils/date-time'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'

const props = defineProps<{
  botId: string
  botType?: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const deletingRuleId = ref('')
const allowGuestDraft = ref(false)

const isPersonalBot = computed(() => props.botType === 'personal')

type RuleSelection = {
  userId: string
  channelIdentityId: string
  observedConversationRouteId: string
  sourceChannel: string
  sourceConversationType: string
  sourceConversationId: string
  sourceThreadId: string
}

const commonChannels = ['discord', 'feishu', 'qq', 'telegram', 'wecom', 'web', 'cli']

function createSelection(): RuleSelection {
  return {
    userId: '',
    channelIdentityId: '',
    observedConversationRouteId: '',
    sourceChannel: '',
    sourceConversationType: '',
    sourceConversationId: '',
    sourceThreadId: '',
  }
}

const whitelistSelection = reactive(createSelection())
const blacklistSelection = reactive(createSelection())

function identityChannelById(id: string): string {
  const matched = identities.value.find(item => item.id === id)
  return matched?.channel || ''
}

function bindSelectionWatchers(selection: RuleSelection) {
  watch(() => selection.userId, (value) => {
    if (value) {
      selection.channelIdentityId = ''
      selection.observedConversationRouteId = ''
      selection.sourceChannel = ''
    }
  })
  watch(() => selection.channelIdentityId, (value, previous) => {
    if (value) {
      selection.userId = ''
      selection.sourceChannel = identityChannelById(value)
    }
    else if (previous) {
      selection.sourceChannel = ''
    }
    if (value !== previous) {
      selection.observedConversationRouteId = ''
      selection.sourceConversationId = ''
      selection.sourceThreadId = ''
    }
  })
  watch(() => selection.sourceConversationType, (value) => {
    if (value === 'private') {
      selection.observedConversationRouteId = ''
      selection.sourceConversationId = ''
      selection.sourceThreadId = ''
    }
    if (value !== 'thread') {
      selection.sourceThreadId = ''
    }
  })
}

bindSelectionWatchers(whitelistSelection)
bindSelectionWatchers(blacklistSelection)

const { data: settings } = useQuery({
  key: () => ['bot-settings', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

watch(settings, (value) => {
  allowGuestDraft.value = !!value?.allow_guest
}, { immediate: true })

const { data: whitelistData, isLoading: isLoadingWhitelist } = useQuery({
  key: () => ['bot-whitelist', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdWhitelist({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: blacklistData, isLoading: isLoadingBlacklist } = useQuery({
  key: () => ['bot-blacklist', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdBlacklist({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: userCandidates } = useQuery({
  key: () => ['bot-access-users', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAccessUsers({
      path: { bot_id: props.botId },
      query: { limit: 100 },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: identityCandidates } = useQuery({
  key: () => ['bot-access-identities', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAccessChannelIdentities({
      path: { bot_id: props.botId },
      query: { limit: 100 },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const whitelist = computed(() => whitelistData.value?.items ?? [])
const blacklist = computed(() => blacklistData.value?.items ?? [])
const users = computed(() => userCandidates.value?.items ?? [])
const identities = computed(() => identityCandidates.value?.items ?? [])

const whitelistIdentityId = computed(() => whitelistSelection.channelIdentityId.trim())
const blacklistIdentityId = computed(() => blacklistSelection.channelIdentityId.trim())

const { data: whitelistObservedData, isLoading: isLoadingWhitelistObserved } = useQuery({
  key: () => ['bot-access-observed-conversations', props.botId, whitelistIdentityId.value],
  query: async () => {
    const { data } = await getBotsByBotIdAccessChannelIdentitiesByChannelIdentityIdConversations({
      path: {
        bot_id: props.botId,
        channel_identity_id: whitelistIdentityId.value,
      },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId && !!whitelistIdentityId.value,
})

const { data: blacklistObservedData, isLoading: isLoadingBlacklistObserved } = useQuery({
  key: () => ['bot-access-observed-conversations', props.botId, blacklistIdentityId.value],
  query: async () => {
    const { data } = await getBotsByBotIdAccessChannelIdentitiesByChannelIdentityIdConversations({
      path: {
        bot_id: props.botId,
        channel_identity_id: blacklistIdentityId.value,
      },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId && !!blacklistIdentityId.value,
})

const whitelistObservedConversations = computed(() => whitelistObservedData.value?.items ?? [])
const blacklistObservedConversations = computed(() => blacklistObservedData.value?.items ?? [])

const userOptions = computed<SearchableSelectOption[]>(() =>
  users.value.map(item => ({
    value: item.id || '',
    label: item.display_name || item.username || item.id || '',
    description: item.username || item.email || item.id || '',
    keywords: [item.display_name ?? '', item.username ?? '', item.email ?? '', item.id ?? ''],
    meta: item,
  })),
)

const identityOptions = computed<SearchableSelectOption[]>(() =>
  identities.value.map(item => ({
    value: item.id || '',
    label: item.display_name || item.linked_display_name || item.channel_subject_id || item.id || '',
    description: formatIdentityCandidateMeta(item),
    group: item.channel || 'identity',
    groupLabel: item.channel || 'identity',
    keywords: [
      item.display_name ?? '',
      item.linked_display_name ?? '',
      item.linked_username ?? '',
      item.channel_subject_id ?? '',
      item.id ?? '',
    ],
    meta: item,
  })),
)

const knownChannels = computed(() => {
  const values = new Set<string>(commonChannels)
  for (const item of identities.value) {
    if (item.channel) values.add(item.channel)
  }
  for (const item of whitelist.value) {
    if (item.source_scope?.channel) values.add(item.source_scope.channel)
  }
  for (const item of blacklist.value) {
    if (item.source_scope?.channel) values.add(item.source_scope.channel)
  }
  for (const item of whitelistObservedConversations.value) {
    if (item.channel) values.add(item.channel)
  }
  for (const item of blacklistObservedConversations.value) {
    if (item.channel) values.add(item.channel)
  }
  return Array.from(values).filter(Boolean).sort()
})

const hasGuestAccessChanges = computed(() =>
  allowGuestDraft.value !== !!settings.value?.allow_guest,
)

const { mutateAsync: saveGuestAccess, isLoading: isSavingGuestAccess } = useMutation({
  mutation: async () => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: props.botId },
      body: { allow_guest: allowGuestDraft.value },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-settings', props.botId] })
  },
})

const { mutateAsync: saveWhitelist, isLoading: isSavingWhitelist } = useMutation({
  mutation: async (body: AclUpsertRuleRequest) => {
    const { data } = await putBotsByBotIdWhitelist({
      path: { bot_id: props.botId },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-whitelist', props.botId] })
  },
})

const { mutateAsync: saveBlacklist, isLoading: isSavingBlacklist } = useMutation({
  mutation: async (body: AclUpsertRuleRequest) => {
    const { data } = await putBotsByBotIdBlacklist({
      path: { bot_id: props.botId },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-blacklist', props.botId] })
  },
})

function buildSourceScope(selection: RuleSelection): AclSourceScope | undefined {
  const channel = selection.sourceChannel.trim()
  const conversation_type = selection.sourceConversationType.trim()
  const conversation_id = selection.sourceConversationId.trim()
  const thread_id = selection.sourceThreadId.trim()
  if (!channel && !conversation_type && !conversation_id && !thread_id) {
    return undefined
  }
  return { channel, conversation_type, conversation_id, thread_id }
}

function normalizePayload(selection: RuleSelection): AclUpsertRuleRequest | null {
  const user_id = selection.userId.trim()
  const channel_identity_id = selection.channelIdentityId.trim()
  const sourceConversationType = selection.sourceConversationType.trim()
  if ((user_id && channel_identity_id) || (!user_id && !channel_identity_id)) {
    toast.error(t('bots.access.validation'))
    return null
  }
  if (selection.sourceConversationId.trim() && !['group', 'thread'].includes(sourceConversationType)) {
    toast.error(t('bots.access.validationConversationRequiresGroupOrThread'))
    return null
  }
  if (selection.sourceThreadId.trim() && !selection.sourceConversationId.trim()) {
    toast.error(t('bots.access.validationThreadRequiresConversation'))
    return null
  }
  if (selection.sourceThreadId.trim() && sourceConversationType !== 'thread') {
    toast.error(t('bots.access.validationThreadRequiresThreadType'))
    return null
  }
  if ((selection.sourceConversationId.trim() || selection.sourceThreadId.trim()) && !selection.sourceChannel.trim()) {
    toast.error(t('bots.access.validationConversationRequiresChannel'))
    return null
  }
  const source_scope = buildSourceScope(selection)
  return { user_id, channel_identity_id, source_scope }
}

async function handleSaveGuestAccess() {
  try {
    await saveGuestAccess()
    toast.success(t('bots.access.guestAccessSaved'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.saveFailed')))
  }
}

async function handleAddWhitelist() {
  const payload = normalizePayload(whitelistSelection)
  if (!payload) return
  try {
    await saveWhitelist(payload)
    resetSelection(whitelistSelection)
    toast.success(t('bots.access.whitelistSaved'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.saveFailed')))
  }
}

async function handleAddBlacklist() {
  const payload = normalizePayload(blacklistSelection)
  if (!payload) return
  try {
    await saveBlacklist(payload)
    resetSelection(blacklistSelection)
    toast.success(t('bots.access.blacklistSaved'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.saveFailed')))
  }
}

async function handleDeleteWhitelist(ruleId: string) {
  deletingRuleId.value = ruleId
  try {
    await deleteBotsByBotIdWhitelistByRuleId({
      path: { bot_id: props.botId, rule_id: ruleId },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-whitelist', props.botId] })
    toast.success(t('bots.access.deleteSuccess'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.deleteFailed')))
  }
  finally {
    deletingRuleId.value = ''
  }
}

async function handleDeleteBlacklist(ruleId: string) {
  deletingRuleId.value = ruleId
  try {
    await deleteBotsByBotIdBlacklistByRuleId({
      path: { bot_id: props.botId, rule_id: ruleId },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-blacklist', props.botId] })
    toast.success(t('bots.access.deleteSuccess'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.deleteFailed')))
  }
  finally {
    deletingRuleId.value = ''
  }
}

function resetSelection(selection: RuleSelection) {
  Object.assign(selection, createSelection())
}

function clearSourceScope(selection: RuleSelection) {
  selection.observedConversationRouteId = ''
  selection.sourceChannel = selection.channelIdentityId ? identityChannelById(selection.channelIdentityId) : ''
  selection.sourceConversationType = ''
  selection.sourceConversationId = ''
  selection.sourceThreadId = ''
}

function isObservedConversationLocked(selection: RuleSelection): boolean {
  return !!selection.observedConversationRouteId
}

function formatRuleLabel(item: AclRule): string {
  if (item.subject_kind === 'user') {
    return item.user_display_name || item.user_username || item.user_id || '-'
  }
  return item.channel_identity_display_name || item.linked_user_display_name || item.channel_identity_id || '-'
}

function formatRuleMeta(item: AclRule): string {
  const scope = formatRuleScope(item.source_scope)
  if (item.subject_kind === 'user') {
    const base = item.user_username || item.user_id || ''
    return scope ? `${base} · ${scope}` : base
  }
  const channel = item.channel_type || 'channel'
  const subject = item.channel_subject_id || item.channel_identity_id || ''
  const linked = item.linked_user_display_name || item.linked_user_username
  const base = linked ? `${channel}: ${subject} · ${linked}` : `${channel}: ${subject}`
  return scope ? `${base} · ${scope}` : base
}

function rulePlatform(item: AclRule): string {
  return item.source_scope?.channel || item.channel_type || ''
}

function formatIdentityCandidateMeta(item: AclChannelIdentityCandidate): string {
  const subject = item.channel_subject_id || item.id || ''
  const linked = item.linked_display_name || item.linked_username
  return linked ? `${item.channel}: ${subject} · ${linked}` : `${item.channel}: ${subject}`
}

function ruleAvatar(item: AclRule): string {
  if (item.subject_kind === 'user') {
    return item.user_avatar_url || ''
  }
  return item.channel_identity_avatar_url || item.linked_user_avatar_url || ''
}

function candidateAvatar(item?: AclUserCandidate): string {
  return item?.avatar_url || ''
}

function identityAvatar(item?: AclChannelIdentityCandidate): string {
  return item?.avatar_url || item?.linked_avatar_url || ''
}

function formatRuleScope(scope?: AclSourceScope): string {
  if (!scope) return ''
  const parts = [
    scope.channel || '',
    scope.conversation_type || '',
    scope.conversation_id || '',
    scope.thread_id ? `thread:${scope.thread_id}` : '',
  ].filter(Boolean)
  return parts.join(' · ')
}

function formatObservedConversationLabel(item: AclObservedConversationCandidate): string {
  return item.conversation_name || item.conversation_id || item.thread_id || item.route_id || '-'
}

function formatObservedConversationMeta(item: AclObservedConversationCandidate): string {
  const parts = [
    item.channel || '',
    item.conversation_type || '',
    item.conversation_id || '',
    item.thread_id ? `thread:${item.thread_id}` : '',
  ].filter(Boolean)
  const lastObserved = formatRelativeTime(item.last_observed_at, { fallback: '' })
  if (lastObserved) {
    parts.push(`${t('bots.access.lastObserved')}: ${lastObserved}`)
  }
  return parts.join(' · ')
}

function toObservedConversationOptions(items: AclObservedConversationCandidate[]): SearchableSelectOption[] {
  return items.map(item => ({
    value: item.route_id || '',
    label: formatObservedConversationLabel(item),
    description: formatObservedConversationMeta(item),
    group: item.channel || 'conversation',
    groupLabel: item.channel || 'conversation',
    keywords: [
      item.channel ?? '',
      item.conversation_name ?? '',
      item.conversation_id ?? '',
      item.thread_id ?? '',
      item.route_id ?? '',
    ],
    meta: item,
  }))
}

const whitelistObservedConversationOptions = computed(() =>
  toObservedConversationOptions(whitelistObservedConversations.value),
)

const blacklistObservedConversationOptions = computed(() =>
  toObservedConversationOptions(blacklistObservedConversations.value),
)

function applyObservedConversation(selection: RuleSelection, conversations: AclObservedConversationCandidate[], routeId: string) {
  const matched = conversations.find(item => item.route_id === routeId)
  if (!matched) return
  selection.sourceChannel = matched.channel || ''
  selection.sourceConversationType = matched.conversation_type || ''
  selection.sourceConversationId = matched.conversation_id || ''
  selection.sourceThreadId = matched.thread_id || ''
}

watch(() => whitelistSelection.observedConversationRouteId, (routeId) => {
  if (!routeId) return
  applyObservedConversation(whitelistSelection, whitelistObservedConversations.value, routeId)
})

watch(() => blacklistSelection.observedConversationRouteId, (routeId) => {
  if (!routeId) return
  applyObservedConversation(blacklistSelection, blacklistObservedConversations.value, routeId)
})

function initials(value: string): string {
  return value
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map(part => part[0] ?? '')
    .join('')
    .toUpperCase() || '?'
}
</script>
