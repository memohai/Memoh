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
import { Avatar, AvatarFallback, AvatarImage, Button, Label, Separator, Spinner, Switch } from '@memoh/ui'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import {
  getBotsByBotIdAccessChannelIdentities,
  getBotsByBotIdAccessUsers,
  deleteBotsByBotIdBlacklistByRuleId,
  deleteBotsByBotIdWhitelistByRuleId,
  getBotsByBotIdBlacklist,
  getBotsByBotIdSettings,
  getBotsByBotIdWhitelist,
  putBotsByBotIdBlacklist,
  putBotsByBotIdSettings,
  putBotsByBotIdWhitelist,
} from '@memoh/sdk'
import type { AclChannelIdentityCandidate, AclRule, AclUpsertRuleRequest, AclUserCandidate } from '@memoh/sdk'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
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

const whitelistSelection = reactive({
  userId: '',
  channelIdentityId: '',
})

const blacklistSelection = reactive({
  userId: '',
  channelIdentityId: '',
})

watch(() => whitelistSelection.userId, (value) => {
  if (value) whitelistSelection.channelIdentityId = ''
})
watch(() => whitelistSelection.channelIdentityId, (value) => {
  if (value) whitelistSelection.userId = ''
})
watch(() => blacklistSelection.userId, (value) => {
  if (value) blacklistSelection.channelIdentityId = ''
})
watch(() => blacklistSelection.channelIdentityId, (value) => {
  if (value) blacklistSelection.userId = ''
})

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

function normalizePayload(selection: { userId: string, channelIdentityId: string }): AclUpsertRuleRequest | null {
  const user_id = selection.userId.trim()
  const channel_identity_id = selection.channelIdentityId.trim()
  if ((user_id && channel_identity_id) || (!user_id && !channel_identity_id)) {
    toast.error(t('bots.access.validation'))
    return null
  }
  return { user_id, channel_identity_id }
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

function resetSelection(selection: { userId: string, channelIdentityId: string }) {
  selection.userId = ''
  selection.channelIdentityId = ''
}

function formatRuleLabel(item: AclRule): string {
  if (item.subject_kind === 'user') {
    return item.user_display_name || item.user_username || item.user_id || '-'
  }
  return item.channel_identity_display_name || item.linked_user_display_name || item.channel_identity_id || '-'
}

function formatRuleMeta(item: AclRule): string {
  if (item.subject_kind === 'user') {
    return item.user_username || item.user_id || ''
  }
  const channel = item.channel_type || 'channel'
  const subject = item.channel_subject_id || item.channel_identity_id || ''
  const linked = item.linked_user_display_name || item.linked_user_username
  return linked ? `${channel}: ${subject} · ${linked}` : `${channel}: ${subject}`
}

function rulePlatform(item: AclRule): string {
  return item.channel_type || ''
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
