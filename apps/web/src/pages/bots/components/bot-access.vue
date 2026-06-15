<template>
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <div class="mb-6 px-2">
      <h1 class="text-lg font-semibold text-foreground">
        {{ $t('bots.access.title') }}
      </h1>
      <p class="mt-1 max-w-2xl text-xs text-muted-foreground">
        {{ $t('bots.access.subtitle') }}
      </p>
    </div>

    <Tabs
      v-model="activeTab"
      class="w-full"
    >
      <!-- This is scope navigation (two sub-pages), not a value switch — so it's
           underline Tabs, not a SegmentedControl. pl-1 cancels the trigger's own
           px-1 so the tab TEXT lands on the px-2 content rail (aligned with the
           H1 / card titles); the trigger's padding stays as an over-reaching hit
           target without breaking that visual alignment. -->
      <TabsList class="mb-6 pl-1">
        <TabsTrigger value="channel">
          {{ $t('bots.access.channelTab') }}
        </TabsTrigger>
        <TabsTrigger value="workspace">
          {{ $t('bots.access.workspaceTab') }}
        </TabsTrigger>
      </TabsList>

      <!-- Channel members (IM) -->
      <TabsContent
        value="channel"
        class="space-y-8"
      >
        <SettingsSection>
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.access.modeTitle') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ isBlacklistMode ? $t('bots.access.blacklistModeDescription') : $t('bots.access.whitelistModeDescription') }}
              </p>
            </div>
            <SegmentedControl
              :model-value="defaultEffectDraft"
              :items="accessModeItems"
              :aria-label="$t('bots.access.modeTitle')"
              class="shrink-0"
              @update:model-value="(value) => handleSetDefaultEffect(String(value))"
            />
          </div>
        </SettingsSection>

        <SettingsSection>
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.access.members.title') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.access.members.subtitle') }}
              </p>
            </div>
            <Button
              v-if="!memberFormVisible"
              size="sm"
              variant="outline"
              class="shrink-0"
              @click="openMemberForm"
            >
              <Plus class="size-4" />
              {{ memberAddLabel }}
            </Button>
          </div>

          <div
            v-if="memberFormVisible"
            class="mx-4 space-y-3 border-b border-border py-4"
          >
            <SearchableSelectPopover
              v-model="memberFormIdentityId"
              :options="memberCandidateOptions"
              :placeholder="$t('bots.access.members.selectIdentity')"
              :search-placeholder="$t('bots.access.members.searchIdentity')"
              :empty-text="$t('bots.access.members.noIdentityCandidates')"
              :show-group-headers="false"
            >
              <template #option-label="{ option }">
                <div class="flex min-w-0 items-center gap-2 py-0.5 text-left">
                  <Avatar class="size-6 shrink-0">
                    <AvatarImage
                      :src="optionMeta(option.meta).avatarUrl || ''"
                      :alt="option.label"
                    />
                    <AvatarFallback class="text-caption">
                      {{ option.label.slice(0, 2).toUpperCase() }}
                    </AvatarFallback>
                  </Avatar>
                  <div class="min-w-0">
                    <div class="truncate text-xs">
                      {{ option.label }}
                    </div>
                    <div
                      v-if="optionMeta(option.meta).channelLabel"
                      class="truncate text-xs text-muted-foreground"
                    >
                      {{ optionMeta(option.meta).channelLabel }}
                    </div>
                  </div>
                </div>
              </template>
            </SearchableSelectPopover>
            <div class="flex items-center justify-end gap-2 pt-1">
              <Button
                variant="ghost"
                size="sm"
                @click="closeMemberForm"
              >
                {{ $t('common.cancel') }}
              </Button>
              <Button
                size="sm"
                :disabled="!memberFormIdentityId"
                @click="confirmAddMember"
              >
                {{ memberAddConfirmLabel }}
              </Button>
            </div>
          </div>

          <div
            v-if="isLoadingRules || isLoadingManagers"
            class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 text-sm text-muted-foreground last:border-b-0"
          >
            <Spinner class="size-4" />
            {{ $t('common.loading') }}
          </div>

          <Empty
            v-else-if="members.length === 0"
            class="py-12"
          >
            <EmptyHeader>
              <EmptyTitle>{{ $t('bots.access.members.title') }}</EmptyTitle>
              <EmptyDescription>{{ memberEmptyDescription }}</EmptyDescription>
            </EmptyHeader>
          </Empty>

          <template v-else>
            <div
              v-for="member in members"
              :key="member.channelIdentityId"
              class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 last:border-b-0"
            >
              <Avatar class="size-7 shrink-0">
                <AvatarImage
                  :src="member.avatarUrl || ''"
                  :alt="member.label"
                />
                <AvatarFallback class="text-caption">
                  {{ member.label.slice(0, 2).toUpperCase() }}
                </AvatarFallback>
              </Avatar>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="truncate text-sm font-medium text-foreground">
                    {{ member.label }}
                  </span>
                </div>
                <div
                  v-if="member.channelType"
                  class="mt-0.5 flex items-center gap-1 truncate text-xs text-muted-foreground"
                >
                  <ChannelIcon
                    :channel="member.channelType"
                    size="1em"
                  />
                  <span>{{ formatPlatformName(member.channelType) }}</span>
                </div>
              </div>

              <div class="flex shrink-0 items-center gap-3">
                <label class="flex cursor-pointer items-center gap-1.5 text-xs text-foreground">
                  <Checkbox
                    :model-value="member.chat"
                    :disabled="isRowBusy(member)"
                    @update:model-value="(v) => toggleChat(member, v === true)"
                  />
                  {{ $t('bots.access.members.chat') }}
                </label>
                <label class="flex cursor-pointer items-center gap-1.5 text-xs text-foreground">
                  <Checkbox
                    :model-value="member.manage"
                    :disabled="isRowBusy(member)"
                    @update:model-value="(v) => toggleManage(member, v === true)"
                  />
                  {{ $t('bots.access.members.manage') }}
                </label>

                <!-- Info icon next to Manage: its presence marks a platform member
                 (linked to a workspace account). The popover explains where the
                 Manage permission comes from and, when locally overridden, offers
                 a "reset to inherited" action. Reka Popover is click-triggered so
                 the inner button stays reachable. -->
                <Popover v-if="member.bound || member.manageInherited">
                  <PopoverTrigger as-child>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="text-muted-foreground"
                      :title="$t('bots.access.members.platformMember')"
                      :aria-label="$t('bots.access.members.platformMember')"
                    >
                      <Info class="size-3.5" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    align="end"
                    class="w-72 space-y-2 text-left"
                  >
                    <div class="flex items-center gap-1.5 text-xs font-medium text-foreground">
                      <Info class="size-3.5 text-muted-foreground" />
                      {{ $t('bots.access.members.platformMember') }}
                    </div>
                    <p class="text-xs leading-relaxed text-muted-foreground">
                      {{ member.manageInherited
                        ? (member.manageHasOverride
                          ? $t('bots.access.members.overrideActive')
                          : $t('bots.access.members.inheritedFollowing'))
                        : $t('bots.access.members.platformMemberHint') }}
                    </p>
                    <Button
                      v-if="member.manageInherited && member.manageHasOverride"
                      variant="outline"
                      size="sm"
                      class="w-full gap-1.5"
                      :disabled="isRowBusy(member)"
                      @click="recoverInherit(member)"
                    >
                      <RotateCcw class="size-3.5" />
                      {{ $t('bots.access.members.recoverInherit') }}
                    </Button>
                  </PopoverContent>
                </Popover>

                <!-- Only local-only visitors can be removed here. Platform members (bound)
                 come from Workspace Members, so they are managed via the checkboxes
                 (untick Manage to suppress) rather than deleted from this list. -->
                <ConfirmPopover
                  v-if="!member.bound && !member.manageInherited"
                  :message="$t('bots.access.members.removeConfirm')"
                  :confirm-text="$t('common.delete')"
                  @confirm="() => removeMember(member)"
                >
                  <template #trigger>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="text-muted-foreground"
                      :disabled="isRowBusy(member)"
                    >
                      <Trash2 class="size-3.5" />
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>
          </template>
        </SettingsSection>

        <SettingsSection>
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.access.rulesTitle') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.access.rulesEmptyDescription') }}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              class="shrink-0"
              @click="advancedOpen = !advancedOpen"
            >
              <ChevronRight
                class="size-4 transition-transform"
                :class="advancedOpen ? 'rotate-90' : ''"
              />
              {{ advancedOpen ? $t('bots.access.advanced.hide') : $t('bots.access.advanced.show') }}
            </Button>
          </div>

          <template v-if="advancedOpen">
            <div
              v-if="!formVisible"
              class="mx-4 flex min-h-[3.75rem] items-center justify-end border-b border-border py-3"
            >
              <Button
                size="sm"
                variant="outline"
                @click="openAddDialog"
              >
                <Plus class="size-4" />
                {{ addListEntryLabel }}
              </Button>
            </div>

            <template v-if="advancedRules.length">
              <div
                v-for="rule in advancedRules"
                :key="rule.id"
                class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 last:border-b-0"
              >
                <div
                  v-if="rule.subject_channel_type"
                  class="flex size-8 shrink-0 items-center justify-center text-muted-foreground"
                >
                  <ChannelIcon
                    :channel="rule.subject_channel_type"
                    size="1em"
                  />
                </div>
                <Avatar
                  v-else-if="rule.channel_identity_id"
                  class="size-8 shrink-0"
                >
                  <AvatarImage
                    :src="rule.channel_identity_avatar_url || ''"
                    :alt="describeRuleTarget(rule)"
                  />
                  <AvatarFallback class="text-caption">
                    {{ ruleTargetFallback(rule) }}
                  </AvatarFallback>
                </Avatar>
                <div
                  v-else
                  class="flex size-8 shrink-0 items-center justify-center text-muted-foreground"
                >
                  <Users class="size-4" />
                </div>

                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-center gap-2">
                    <p class="truncate text-sm font-medium text-foreground">
                      {{ describeRuleTarget(rule) }}
                    </p>
                    <Badge
                      :variant="rule.enabled ? 'secondary' : 'outline'"
                      size="sm"
                    >
                      {{ rule.enabled ? $t('bots.access.ruleEnabled') : $t('bots.access.ruleDisabled') }}
                    </Badge>
                  </div>
                  <div class="mt-0.5 flex min-w-0 items-center text-xs text-muted-foreground">
                    <span class="shrink-0">{{ ruleScopePrefix(rule) }}</span>
                    <template v-if="ruleScopeDetail(rule)">
                      <span class="mx-1 shrink-0">: </span>
                      <span class="truncate">{{ ruleScopeDetail(rule) }}</span>
                    </template>
                  </div>
                </div>

                <div class="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    class="text-muted-foreground"
                    :aria-label="rule.enabled ? $t('bots.access.disableRule') : $t('bots.access.enableRule')"
                    @click="handleToggleEnabled(rule, !(rule.enabled ?? false))"
                  >
                    <Power class="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    class="text-muted-foreground"
                    :aria-label="$t('common.edit')"
                    @click="openEditDialog(rule)"
                  >
                    <SquarePen class="size-3.5" />
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
                        class="text-muted-foreground"
                        :aria-label="$t('common.delete')"
                      >
                        <Trash2 class="size-3.5" />
                      </Button>
                    </template>
                  </ConfirmPopover>
                </div>
              </div>
            </template>

            <Empty
              v-else-if="!formVisible"
              class="py-12"
            >
              <EmptyHeader>
                <EmptyTitle>{{ $t('bots.access.rulesEmpty') }}</EmptyTitle>
                <EmptyDescription>{{ $t('bots.access.rulesEmptyDescription') }}</EmptyDescription>
              </EmptyHeader>
            </Empty>

            <section
              v-if="formVisible"
              class="mx-4 space-y-4 border-b border-border py-4 last:border-b-0"
            >
              <div class="flex items-center justify-between gap-4">
                <h3 class="text-sm font-medium text-foreground">
                  {{ editingRule ? $t('bots.access.editRule') : addListEntryLabel }}
                </h3>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  :aria-label="$t('common.cancel')"
                  @click="formVisible = false"
                >
                  <X class="size-4" />
                </Button>
              </div>

              <form
                class="space-y-4"
                @submit.prevent="handleSaveRule(false)"
              >
                <p class="font-mono text-xs leading-relaxed text-muted-foreground">
                  {{ rulePreviewText }}
                </p>

                <div class="grid gap-4 sm:grid-cols-2">
                  <div class="space-y-1.5">
                    <div class="flex items-center justify-between gap-2">
                      <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.platformQuestion') }}</Label>
                      <Button
                        v-if="ruleForm.subjectChannelType"
                        type="button"
                        variant="ghost"
                        size="text"
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
                    />
                  </div>

                  <div class="space-y-1.5">
                    <div class="flex items-center justify-between gap-2">
                      <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.userQuestion') }}</Label>
                      <Button
                        v-if="ruleForm.channelIdentityId"
                        type="button"
                        variant="ghost"
                        size="text"
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
                    />
                  </div>
                </div>

                <div class="space-y-2">
                  <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.scopeQuestion') }}</Label>
                  <SegmentedControl
                    :model-value="ruleForm.sourceConversationType"
                    :items="chatScopeOptions"
                    :aria-label="$t('bots.access.scopeQuestion')"
                    class="w-full sm:w-fit"
                    @update:model-value="(value) => setChatScope(String(value))"
                  />
                </div>

                <div
                  v-if="showSpecificConversationSection"
                  class="space-y-3"
                >
                  <div class="grid gap-3 sm:grid-cols-2">
                    <div class="space-y-1.5">
                      <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.conversationId') }}</Label>
                      <Input
                        v-model="ruleForm.sourceConversationId"
                        class="h-8"
                        :placeholder="$t('bots.access.conversationIdPlaceholder')"
                      />
                    </div>
                    <div
                      v-if="ruleForm.sourceConversationType === 'thread'"
                      class="space-y-1.5"
                    >
                      <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.threadId') }}</Label>
                      <Input
                        v-model="ruleForm.sourceThreadId"
                        class="h-8"
                        :placeholder="$t('bots.access.threadIdPlaceholder')"
                      />
                    </div>
                  </div>
                </div>

                <div class="space-y-1.5">
                  <Label class="text-xs font-medium text-muted-foreground">{{ $t('bots.access.description') }}</Label>
                  <Input
                    v-model="ruleForm.description"
                    class="h-8"
                    :placeholder="$t('bots.access.descriptionPlaceholder')"
                  />
                </div>

                <p
                  v-if="formError"
                  class="text-xs text-destructive"
                >
                  {{ formError }}
                </p>

                <div class="flex justify-end gap-2 border-t border-border pt-4">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    @click="formVisible = false"
                  >
                    {{ $t('common.cancel') }}
                  </Button>
                  <Button
                    type="submit"
                    size="sm"
                    :disabled="isSavingRule"
                  >
                    <Spinner
                      v-if="isSavingRule"
                      class="size-3"
                    />
                    {{ $t('bots.access.saveAndEnable') }}
                  </Button>
                </div>
              </form>
            </section>
          </template>
        </SettingsSection>
      </TabsContent>

      <TabsContent
        value="workspace"
        class="space-y-8"
      >
        <BotUserAccess :bot-id="botId" />
      </TabsContent>
    </Tabs>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Plus,
  SquarePen,
  Trash2,
  X,
  Users,
  Power,
  Info,
  RotateCcw,
  ChevronRight,
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
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
  Badge,
  Checkbox,
  SegmentedControl,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import BotUserAccess from './bot-user-access.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { channelTypeDisplayName } from '@/utils/channel-type-label'
import SettingsSection from '@/components/settings/section.vue'
import type { AclRule, AclSourceScope, ChannelaccessManager, HandlersChannelMeta } from '@memohai/sdk'
import {
  getChannels,
  getBotsByBotIdAclRules,
  getBotsByBotIdAclDefaultEffect,
  putBotsByBotIdAclDefaultEffect,
  postBotsByBotIdAclRules,
  putBotsByBotIdAclRulesByRuleId,
  deleteBotsByBotIdAclRulesByRuleId,
  getBotsByBotIdAclChannelIdentities,
  getBotsByBotIdChannelManagers,
  postBotsByBotIdChannelManagers,
  deleteBotsByBotIdChannelManagersByChannelIdentityId,
} from '@memohai/sdk'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

const activeTab = ref<'channel' | 'workspace'>('channel')

const accessModeItems = computed(() => [
  {
    value: 'allow',
    label: t('bots.access.blacklistMode'),
    disabled: isSavingDefaultEffect.value,
  },
  {
    value: 'deny',
    label: t('bots.access.whitelistMode'),
    disabled: isSavingDefaultEffect.value,
  },
])

const chatScopeOptions = computed(() => [
  { value: '', label: t('bots.access.chatScopeAny') },
  { value: 'private', label: t('bots.access.privateConversationGroup') },
  { value: 'group', label: t('bots.access.groupConversationGroup') },
  { value: 'thread', label: t('bots.access.threadConversationGroup') },
])

const aclExcludedChannelTypes = new Set(['web'])

interface IdentityOptionMeta {
  avatarUrl?: string
  channelLabel?: string
}

function optionMeta(meta: unknown): IdentityOptionMeta {
  return (meta ?? {}) as IdentityOptionMeta
}

// ---- queries ----

const { data: rulesData, isLoading: isLoadingRules } = useQuery({
  key: () => ['bot-acl-rules', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdAclRules({ path: { bot_id: props.botId }, throwOnError: true })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: managersData, isLoading: isLoadingManagers } = useQuery({
  key: () => ['bot-channel-managers', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdChannelManagers({ path: { bot_id: props.botId }, throwOnError: true })
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
    const { data } = await getBotsByBotIdAclDefaultEffect({ path: { bot_id: props.botId }, throwOnError: true })
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

// ---- derived: mode ----

const defaultEffectDraft = ref('allow')
const isSavingDefaultEffect = ref(false)

watch(defaultEffectData, (data) => {
  if (data?.default_effect) defaultEffectDraft.value = data.default_effect
}, { immediate: true })

const isBlacklistMode = computed(() => defaultEffectDraft.value === 'allow')
const listEntryEffect = computed(() => (isBlacklistMode.value ? 'deny' : 'allow'))

const rules = computed(() => rulesData.value?.items ?? [])
const managers = computed<ChannelaccessManager[]>(() => managersData.value?.items ?? [])

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
    .map(meta => ({ value: meta.type?.trim() ?? '', label: formatPlatformName(meta.type, meta.display_name) }))
    .filter(option => option.value && !aclExcludedChannelTypes.has(option.value))
    .sort((a, b) => a.label.localeCompare(b.label)),
)

// A pure-identity rule targets a single channel identity with no platform/scope.
function isPureIdentityRule(rule: AclRule): boolean {
  return !!rule.channel_identity_id && !rule.subject_channel_type && !rule.source_scope
}

// Identity-scoped chat rules for the current mode (deny in blacklist, allow in whitelist).
const identityChatRules = computed(() =>
  rules.value.filter(r => r.effect === listEntryEffect.value && isPureIdentityRule(r)),
)

// Advanced rules: platform-wide or conversation-scoped (everything not a pure-identity rule).
const advancedRules = computed(() =>
  rules.value.filter(r => r.effect === listEntryEffect.value && !isPureIdentityRule(r)),
)

// ---- members aggregation ----

interface MemberRow {
  channelIdentityId: string
  label: string
  avatarUrl: string
  channelType: string
  chat: boolean
  chatRuleId?: string
  manage: boolean
  manageInherited: boolean
  manageHasOverride: boolean
  // bound = linked to a workspace member of this bot (a "platform member"),
  // regardless of whether it carries Manage. Drives the info (ⓘ) marker.
  bound: boolean
}

const pendingIds = ref<Set<string>>(new Set())
const busyIds = ref<Set<string>>(new Set())

const identityInfoById = computed(() => {
  const map = new Map<string, { label: string, avatarUrl: string, channelType: string }>()
  for (const i of identityCandidates.value?.items ?? []) {
    if (!i.id) continue
    map.set(i.id, {
      label: i.display_name || i.channel_subject_id || i.id,
      avatarUrl: i.avatar_url ?? '',
      channelType: i.channel ?? '',
    })
  }
  return map
})

const members = computed<MemberRow[]>(() => {
  const byId = new Map<string, MemberRow>()
  const ensure = (id: string): MemberRow => {
    let row = byId.get(id)
    if (!row) {
      row = {
        channelIdentityId: id,
        label: id,
        avatarUrl: '',
        channelType: '',
        chat: isBlacklistMode.value, // default: blacklist allows, whitelist blocks
        manage: false,
        manageInherited: false,
        manageHasOverride: false,
        bound: false,
      }
      byId.set(id, row)
    }
    return row
  }

  // Managers (manage / inherited / override + display info).
  for (const m of managers.value) {
    const id = m.channel_identity_id
    if (!id) continue
    const row = ensure(id)
    row.manage = m.manage ?? false
    row.manageInherited = m.inherited ?? false
    row.manageHasOverride = m.has_override ?? false
    row.bound = m.bound ?? false
    if (m.channel_identity_display_name) row.label = m.channel_identity_display_name
    else if (m.channel_subject_id) row.label = m.channel_subject_id
    if (m.channel_identity_avatar_url) row.avatarUrl = m.channel_identity_avatar_url
    if (m.channel_type) row.channelType = m.channel_type
  }

  // Identity-scoped chat rules for current mode.
  for (const rule of identityChatRules.value) {
    const id = rule.channel_identity_id
    if (!id) continue
    const row = ensure(id)
    row.chatRuleId = rule.id
    // blacklist: a deny rule means chat OFF; whitelist: an allow rule means chat ON.
    row.chat = !isBlacklistMode.value
    if (rule.channel_identity_display_name) row.label = rule.channel_identity_display_name
    else if (rule.channel_subject_id) row.label = rule.channel_subject_id
    if (rule.channel_identity_avatar_url) row.avatarUrl = rule.channel_identity_avatar_url
    if (rule.channel_type) row.channelType = rule.channel_type
  }

  // Optimistic rows for an in-flight add, until the persisted record is refetched.
  for (const id of pendingIds.value) {
    ensure(id)
  }

  // Fill display info from candidate directory where missing.
  for (const row of byId.values()) {
    if (!row.label || row.label === row.channelIdentityId || !row.avatarUrl || !row.channelType) {
      const info = identityInfoById.value.get(row.channelIdentityId)
      if (info) {
        if (!row.label || row.label === row.channelIdentityId) row.label = info.label
        if (!row.avatarUrl) row.avatarUrl = info.avatarUrl
        if (!row.channelType) row.channelType = info.channelType
      }
    }
  }

  return [...byId.values()].sort((a, b) => a.label.localeCompare(b.label))
})

function isRowBusy(member: MemberRow): boolean {
  return busyIds.value.has(member.channelIdentityId)
}

// ---- member add form ----

const memberFormVisible = ref(false)
const memberFormIdentityId = ref('')

const memberAddLabel = computed(() =>
  isBlacklistMode.value ? t('bots.access.members.addBlocked') : t('bots.access.members.add'),
)
const memberAddConfirmLabel = computed(() =>
  isBlacklistMode.value ? t('bots.access.members.blockConfirm') : t('common.add'),
)
const memberEmptyDescription = computed(() =>
  isBlacklistMode.value ? t('bots.access.members.emptyBlacklist') : t('bots.access.members.emptyWhitelist'),
)
const memberAddedMessage = computed(() =>
  isBlacklistMode.value ? t('bots.access.members.blocked') : t('bots.access.members.added'),
)
const memberAddFailedMessage = computed(() =>
  isBlacklistMode.value ? t('bots.access.members.blockFailed') : t('bots.access.members.addFailed'),
)

const memberCandidateOptions = computed<SearchableSelectOption[]>(() => {
  const present = new Set(members.value.map(m => m.channelIdentityId))
  return (identityCandidates.value?.items ?? [])
    .filter(i => i.id && !present.has(i.id) && !aclExcludedChannelTypes.has(i.channel ?? ''))
    .map(i => ({
      value: i.id ?? '',
      label: i.display_name || i.channel_subject_id || i.id || '',
      keywords: [i.display_name ?? '', i.channel_subject_id ?? '', i.channel ?? ''],
      meta: { avatarUrl: i.avatar_url, channelLabel: channelTypeDisplayName(t, i.channel ?? '') },
    }))
})

function openMemberForm() {
  memberFormVisible.value = true
  memberFormIdentityId.value = ''
}

function closeMemberForm() {
  memberFormVisible.value = false
  memberFormIdentityId.value = ''
}

// Add persists immediately (no in-memory-only draft): adding a member writes the
// list entry for the active mode. Whitelist entries allow chat; blacklist entries
// deny chat. Manage is only written by the Manage checkbox so list membership
// cannot accidentally suppress inherited workspace permissions.
async function confirmAddMember() {
  const id = memberFormIdentityId.value.trim()
  if (!id) return
  busyIds.value.add(id)
  pendingIds.value.add(id) // optimistic row while the write lands
  closeMemberForm()
  try {
    if (isBlacklistMode.value) {
      await createIdentityRule(id, 'deny')
    }
    else {
      await createIdentityRule(id, 'allow')
    }
    await Promise.all([invalidateRules(), invalidateManagers()])
    toast.success(memberAddedMessage.value)
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, memberAddFailedMessage.value))
  }
  finally {
    pendingIds.value.delete(id)
    busyIds.value.delete(id)
  }
}

// ---- chat / manage toggles ----

async function createIdentityRule(channelIdentityId: string, effect: string) {
  await postBotsByBotIdAclRules({
    path: { bot_id: props.botId },
    body: { enabled: true, effect, channel_identity_id: channelIdentityId },
    throwOnError: true,
  })
}

async function deleteRule(ruleId: string) {
  await deleteBotsByBotIdAclRulesByRuleId({ path: { bot_id: props.botId, rule_id: ruleId }, throwOnError: true })
}

function invalidateRules() {
  return queryCache.invalidateQueries({ key: ['bot-acl-rules', props.botId] })
}

function invalidateManagers() {
  return queryCache.invalidateQueries({ key: ['bot-channel-managers', props.botId] })
}

async function toggleChat(member: MemberRow, next: boolean) {
  busyIds.value.add(member.channelIdentityId)
  try {
    if (isBlacklistMode.value) {
      // blacklist: chat ON = no deny rule; chat OFF = deny rule.
      if (next) {
        if (member.chatRuleId) await deleteRule(member.chatRuleId)
      }
      else if (!member.chatRuleId) {
        await createIdentityRule(member.channelIdentityId, 'deny')
      }
    }
    else {
      // whitelist: chat ON = allow rule; chat OFF = no allow rule.
      if (next) {
        if (!member.chatRuleId) await createIdentityRule(member.channelIdentityId, 'allow')
      }
      else if (member.chatRuleId) {
        await deleteRule(member.chatRuleId)
      }
    }
    await invalidateRules()
    toast.success(t('bots.access.members.updated'))
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.members.updateFailed')))
  }
  finally {
    busyIds.value.delete(member.channelIdentityId)
  }
}

async function toggleManage(member: MemberRow, next: boolean) {
  busyIds.value.add(member.channelIdentityId)
  try {
    // Toggling Manage always writes an explicit local override (granted = next);
    // it never deletes the membership. This keeps the row stable (final state =
    // local override ?? inherited). To stop overriding an inherited member, use
    // "Reset to inherited" in the info popover; to remove a local member entirely,
    // use the Trash action.
    await postBotsByBotIdChannelManagers({
      path: { bot_id: props.botId },
      body: { channel_identity_id: member.channelIdentityId, granted: next },
      throwOnError: true,
    })
    await invalidateManagers()
    toast.success(t('bots.access.members.updated'))
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.members.updateFailed')))
  }
  finally {
    busyIds.value.delete(member.channelIdentityId)
  }
}

async function recoverInherit(member: MemberRow) {
  busyIds.value.add(member.channelIdentityId)
  try {
    await deleteBotsByBotIdChannelManagersByChannelIdentityId({
      path: { bot_id: props.botId, channel_identity_id: member.channelIdentityId },
      throwOnError: true,
    })
    await invalidateManagers()
    toast.success(t('bots.access.members.updated'))
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.members.updateFailed')))
  }
  finally {
    busyIds.value.delete(member.channelIdentityId)
  }
}

async function removeMember(member: MemberRow) {
  busyIds.value.add(member.channelIdentityId)
  try {
    if (member.chatRuleId) await deleteRule(member.chatRuleId)
    if (member.manageHasOverride || (member.manage && !member.manageInherited)) {
      await deleteBotsByBotIdChannelManagersByChannelIdentityId({
        path: { bot_id: props.botId, channel_identity_id: member.channelIdentityId },
        throwOnError: true,
      }).catch(() => undefined)
    }
    pendingIds.value.delete(member.channelIdentityId)
    await Promise.all([invalidateRules(), invalidateManagers()])
    toast.success(t('bots.access.members.removed'))
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.members.removeFailed')))
  }
  finally {
    busyIds.value.delete(member.channelIdentityId)
  }
}

// ---- default effect ----

async function handleSetDefaultEffect(effect: string) {
  const previousEffect = defaultEffectDraft.value
  if (effect === previousEffect || isSavingDefaultEffect.value) return
  defaultEffectDraft.value = effect
  isSavingDefaultEffect.value = true
  try {
    await putBotsByBotIdAclDefaultEffect({ path: { bot_id: props.botId }, body: { default_effect: effect }, throwOnError: true })
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

// ---- advanced rule form ----

const advancedOpen = ref(false)

interface RuleForm {
  effect: string
  subjectChannelType: string
  channelIdentityId: string
  sourceConversationType: string
  sourceConversationId: string
  sourceThreadId: string
  description: string
}

function createRuleForm(effect = 'deny'): RuleForm {
  return {
    effect,
    subjectChannelType: '',
    channelIdentityId: '',
    sourceConversationType: '',
    sourceConversationId: '',
    sourceThreadId: '',
    description: '',
  }
}

const ruleForm = reactive(createRuleForm())
const formVisible = ref(false)
const editingRule = ref<AclRule | null>(null)
const formError = ref('')
const savingRuleAction = ref(false)
const isSavingRule = computed(() => savingRuleAction.value)

const identityOptions = computed(() =>
  (identityCandidates.value?.items ?? [])
    .filter(i => !aclExcludedChannelTypes.has(i.channel ?? ''))
    .map(i => ({
      value: i.id ?? '',
      label: i.display_name || i.channel_subject_id || i.id || '',
      meta: { avatarUrl: i.avatar_url, channel: i.channel, channelLabel: formatPlatformName(i.channel) },
    })),
)

const filteredIdentityOptions = computed(() => {
  const platform = ruleForm.subjectChannelType.trim()
  if (!platform) return identityOptions.value
  return identityOptions.value.filter(option => option.meta.channel === platform)
})

const addListEntryLabel = computed(() =>
  isBlacklistMode.value ? t('bots.access.addBlacklistEntry') : t('bots.access.addWhitelistEntry'),
)
const showSpecificConversationSection = computed(() =>
  ruleForm.sourceConversationType === 'group'
  || ruleForm.sourceConversationType === 'thread'
  || !!ruleForm.sourceConversationId
  || !!ruleForm.sourceThreadId,
)
const rulePreviewText = computed(() => {
  const target = ruleForm.subjectChannelType
    ? formatPlatformName(ruleForm.subjectChannelType)
    : (ruleForm.channelIdentityId ? t('bots.access.userTargetPreview', { user: selectedIdentityLabel.value || '?' }) : t('bots.access.subjectAllLabel'))
  return isBlacklistMode.value
    ? t('bots.access.blacklistPreview', { target })
    : t('bots.access.whitelistPreview', { target })
})
const selectedIdentityLabel = computed(() =>
  identityOptions.value.find(o => o.value === ruleForm.channelIdentityId)?.label ?? '',
)

watch(listEntryEffect, (effect) => {
  if (formVisible.value && !editingRule.value) ruleForm.effect = effect
})

function openAddDialog() {
  editingRule.value = null
  Object.assign(ruleForm, createRuleForm(listEntryEffect.value))
  formError.value = ''
  formVisible.value = true
}

function openEditDialog(rule: AclRule) {
  editingRule.value = rule
  ruleForm.effect = rule.effect ?? 'deny'
  ruleForm.subjectChannelType = rule.subject_channel_type ?? ''
  ruleForm.channelIdentityId = rule.channel_identity_id ?? ''
  ruleForm.sourceConversationType = rule.source_scope?.conversation_type ?? ''
  ruleForm.sourceConversationId = rule.source_scope?.conversation_id ?? ''
  ruleForm.sourceThreadId = rule.source_scope?.thread_id ?? ''
  ruleForm.description = rule.description ?? ''
  formError.value = ''
  formVisible.value = true
}

function setChatScope(scope: string) {
  if (scope === '' || scope === 'private' || scope !== ruleForm.sourceConversationType) {
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
}

function buildSourceScope(): AclSourceScope | undefined {
  const scope: AclSourceScope = {}
  if (ruleForm.sourceConversationType) scope.conversation_type = ruleForm.sourceConversationType
  if (ruleForm.sourceConversationId) scope.conversation_id = ruleForm.sourceConversationId
  if (ruleForm.sourceThreadId) scope.thread_id = ruleForm.sourceThreadId
  if (!scope.conversation_type && !scope.conversation_id && !scope.thread_id) return undefined
  return scope
}

async function handleSaveRule(_enable: boolean) {
  formError.value = ''
  savingRuleAction.value = true
  try {
    const body = {
      enabled: true,
      effect: ruleForm.effect,
      channel_identity_id: ruleForm.channelIdentityId || undefined,
      subject_channel_type: ruleForm.subjectChannelType || undefined,
      source_scope: buildSourceScope(),
      description: ruleForm.description || undefined,
    }
    if (editingRule.value?.id) {
      await putBotsByBotIdAclRulesByRuleId({ path: { bot_id: props.botId, rule_id: editingRule.value.id }, body, throwOnError: true })
    }
    else {
      await postBotsByBotIdAclRules({ path: { bot_id: props.botId }, body, throwOnError: true })
    }
    await invalidateRules()
    toast.success(t('bots.access.ruleSaved'))
    formVisible.value = false
  }
  catch (e) {
    formError.value = resolveApiErrorMessage(e, t('bots.access.saveFailed'))
  }
  finally {
    savingRuleAction.value = false
  }
}

async function handleDeleteRule(ruleId: string) {
  try {
    await deleteRule(ruleId)
    await invalidateRules()
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
    await invalidateRules()
  }
  catch (e) {
    toast.error(resolveApiErrorMessage(e, t('bots.access.saveFailed')))
  }
}

// ---- display helpers ----

function describeRuleTarget(rule: AclRule): string {
  const platformType = rule.subject_channel_type || rule.channel_type
  const platform = platformType ? formatPlatformName(platformType) : ''
  const user = rule.channel_identity_display_name || rule.channel_subject_id || rule.channel_identity_id || ''
  if (rule.subject_channel_type && rule.channel_identity_id) return t('bots.access.platformUserTargetPreview', { platform, user: user || '?' })
  if (rule.subject_channel_type) return t('bots.access.platformTargetPreview', { platform })
  if (rule.channel_identity_id) return t('bots.access.userTargetPreview', { user: user || '?' })
  return t('bots.access.subjectAllLabel')
}

function formatPlatformName(type?: string | null, displayName?: string | null): string {
  const raw = type?.trim() ?? ''
  const meta = raw ? platformMetaByType.value.get(raw) : undefined
  return channelTypeDisplayName(t, raw, displayName ?? meta?.display_name)
}

function ruleTargetFallback(rule: AclRule): string {
  const label = describeRuleTarget(rule).trim()
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function ruleScopePrefix(rule: AclRule): string {
  const scope = rule.source_scope
  if (!scope?.conversation_type) return t('bots.access.chatScopeAny')
  switch (scope.conversation_type) {
    case 'private': return t('bots.access.privateConversationGroup')
    case 'group': return t('bots.access.groupConversationGroup')
    case 'thread': return t('bots.access.threadConversationGroup')
    default: return scope.conversation_type
  }
}

function ruleScopeDetail(rule: AclRule): string {
  const scope = rule.source_scope
  const conversationID = scope?.conversation_id?.trim()
  if (!conversationID) return ''
  const name = rule.source_conversation_name?.trim()
  const displayName = name ? `${name} (${conversationID})` : conversationID
  const thread = scope?.thread_id ? ` · thread:${scope.thread_id}` : ''
  return `${displayName}${thread}`
}
</script>
