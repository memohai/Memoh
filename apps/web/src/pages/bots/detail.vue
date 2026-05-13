<template>
  <section class="mx-auto absolute inset-0 flex flex-col bg-background">
    <div class="flex-1 relative">
      <MasterDetailSidebarLayout
        class="[&_td:last-child]:w-45"
      >
        <template #sidebar-header>
          <!-- Bot Identity Header -->
          <div class="p-4 pb-3 flex flex-col">
            <div class="flex items-center gap-3">
              <!-- Avatar -->
              <div class="group/avatar relative size-12 shrink-0 rounded-full overflow-hidden bg-muted">
                <Avatar class="size-12 rounded-full">
                  <AvatarImage
                    v-if="bot?.avatar_url"
                    :src="bot.avatar_url"
                    :alt="bot.display_name"
                  />
                  <AvatarFallback class="text-lg">
                    {{ avatarFallback }}
                  </AvatarFallback>
                </Avatar>
                <!-- Edit Overlay -->
                <button
                  type="button"
                  class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
                  :title="$t('common.edit')"
                  :aria-label="$t('common.edit')"
                  :disabled="!bot || botLifecyclePending"
                  @click="handleEditAvatar"
                >
                  <SquarePen class="size-4 text-white" />
                </button>
              </div>
              
              <!-- Info Block -->
              <div class="min-w-0 flex-1 flex flex-col justify-center">
                <div class="group/name flex items-center gap-1 relative min-w-0">
                  <template v-if="isEditingBotName && bot">
                    <Input
                      ref="editNameInputRef"
                      v-model="botNameDraft"
                      class="h-7 w-full text-xs px-2 pr-6 shadow-none"
                      :placeholder="$t('bots.displayNamePlaceholder')"
                      :disabled="isSavingBotName"
                      @keydown.enter.prevent="handleConfirmBotName"
                      @keydown.esc.prevent="handleCancelBotName"
                      @blur="handleConfirmBotName"
                    />
                    <div class="absolute right-1.5 top-1/2 -translate-y-1/2 opacity-50 pointer-events-none">
                      <Check class="size-3" />
                    </div>
                  </template>
                  <template v-else>
                    <h2 class="truncate text-sm font-semibold text-foreground">
                      {{ botNameDraft.trim() || bot?.display_name || botId }}
                    </h2>
                    <button
                      v-if="bot"
                      type="button"
                      class="opacity-0 group-hover/name:opacity-100 p-1 shrink-0"
                      :disabled="botLifecyclePending"
                      @click="handleStartEditBotName"
                    >
                      <SquarePen class="size-3 text-muted-foreground" />
                    </button>
                  </template>
                </div>
                
                <!-- Status Badge (Flat Industrial Style) -->
                <div class="mt-0.5">
                  <div
                    v-if="bot"
                    class="inline-flex items-center h-5 bg-[#27272a] rounded-full px-2 gap-1.5"
                    :title="hasIssue ? issueTitle : undefined"
                  >
                    <LoaderCircle
                      v-if="bot.status === 'creating' || bot.status === 'deleting'"
                      class="size-2.5 animate-spin text-[#d0d0d4]"
                    />
                    <div
                      v-else
                      class="size-1.5 rounded-full"
                      :class="statusVariant === 'destructive' ? 'bg-destructive' : 'bg-success'"
                    />
                    <span class="text-[10px] font-medium text-[#d0d0d4]">{{ statusLabel }}</span>
                  </div>
                  <span
                    v-if="bot?.type"
                    class="text-[10px] text-muted-foreground ml-1.5"
                  >{{ botTypeLabel }}</span>
                </div>
              </div>
            </div>
            
            <!-- Search Input -->
            <div class="mt-4 relative">
              <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground" />
              <Input
                v-model="searchQuery"
                type="text"
                class="pl-8 h-8 text-xs bg-transparent shadow-none focus-visible:ring-0"
                :placeholder="$t('common.search')"
              />
              <button
                v-if="searchQuery"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 size-4 flex items-center justify-center rounded-full hover:bg-muted text-muted-foreground shrink-0"
                @click="searchQuery = ''"
              >
                <X class="size-2.5" />
              </button>
            </div>
          </div>
        </template>

        <template #sidebar-content>
          <!-- Search Results View -->
          <div
            v-if="searchQuery"
            class="flex flex-col gap-1"
          >
            <div
              v-if="searchResults.length === 0"
              class="px-3 py-4 text-xs text-muted-foreground text-center"
            >
              {{ $t('common.noData') }}
            </div>
            <SidebarMenu
              v-else
              class="m-0 p-0 gap-1"
            >
              <SidebarMenuItem
                v-for="(result, idx) in searchResults"
                :key="idx"
              >
                <SidebarMenuButton
                  as-child
                  class="justify-start py-0! px-0 h-11 before:hidden"
                >
                  <button
                    class="w-full flex flex-col items-start justify-center py-2 px-3 text-left border border-transparent transition-colors hover:bg-accent hover:text-accent-foreground rounded-md group/result"
                    @click="() => { activeTab = result.tab; searchQuery = '' }"
                  >
                    <span class="text-xs font-medium text-foreground group-hover/result:text-accent-foreground">{{ result.translatedTitle }}</span>
                    <span class="text-[10px] text-muted-foreground mt-1 flex items-center gap-1 group-hover/result:text-accent-foreground/70">
                      <component
                        :is="tabList.find(t => t.value === result.tab)?.icon"
                        class="size-3 opacity-70"
                      />
                      {{ $t(`bots.tabs.${result.tab}`) }}
                    </span>
                  </button>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </div>

          <!-- Normal Grouped View -->
          <template v-else>
            <div
              v-for="(group, idx) in groupedTabs"
              :key="group.key"
              :class="idx > 0 ? 'mt-4' : ''"
              class="flex flex-col gap-0.5"
            >
              <SidebarMenu
                v-for="tab in group.items"
                :key="tab.value"
                class="m-0 p-0"
              >
                <SidebarMenuItem>
                  <SidebarMenuButton
                    as-child
                    :is-active="activeTab === tab.value"
                    class="justify-start py-0! px-0 h-10 before:hidden"
                  >
                    <Toggle
                      class="w-full justify-start h-10 px-3 text-xs font-medium border-0 bg-transparent! transition-colors gap-3"
                      :model-value="isActive(tab.value).value"
                      @update:model-value="(isSelect: boolean) => {
                        if (isSelect) {
                          activeTab = tab.value
                        }
                      }"
                    >
                      <component
                        :is="tab.icon"
                        v-if="tab.icon"
                        class="size-4 shrink-0"
                      />
                      <span class="truncate">{{ $t(tab.label) }}</span>
                    </Toggle>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </div>
          </template>
        </template>

        <template #sidebar-footer />

        <template #detail>
          <div class="absolute inset-0 overflow-y-auto bg-background">
            <!-- Ensure consistent padding matching Box-in-Box bento architecture -->
            <div class="px-6 pt-4 pb-12 max-w-4xl mx-auto">
              <KeepAlive>
                <component
                  :is="activeComponent?.component"
                  v-bind="activeComponent?.params"
                />
              </KeepAlive>
            </div>
          </div>
        </template>
      </MasterDetailSidebarLayout>
    </div>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="avatarUrlModel"
      :fallback-text="avatarFallback"
    />
  </section>
</template>

<script setup lang="ts">
import {
  Avatar, AvatarImage, AvatarFallback, Input,
  SidebarMenu, SidebarMenuButton, SidebarMenuItem, Toggle
} from '@memohai/ui'
import {
  SquarePen, LoaderCircle, Check, Search, X, LayoutDashboard, Settings, MessageSquare,
  BrainCircuit, ShieldAlert, HeartPulse, Database, Mail, Link, Clock, Server, FileBox, Zap,
  Monitor, Globe
} from 'lucide-vue-next'
import { computed, ref, watch, onMounted, toValue, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import {
  getBotsById, putBotsById,
  getBotsByIdChecks,
  getBotsByBotIdContainer,
  getBotsByBotIdContainerSnapshots,
} from '@memohai/sdk'
import { getBotsQueryKey } from '@memohai/sdk/colada'
import type {
  BotsBotCheck, HandlersGetContainerResponse,
  HandlersListSnapshotsResponse,
} from '@memohai/sdk'
import { useCapabilitiesStore } from '@/store/capabilities'

import BotSettings from './components/bot-settings.vue'
import BotToolApproval from './components/bot-tool-approval.vue'
import BotDesktop from './components/bot-desktop.vue'
import BotNetwork from './components/bot-network.vue'
import BotChannels from './components/bot-channels.vue'
import BotMcp from './components/bot-mcp.vue'
import BotMemory from './components/bot-memory.vue'
import BotSkills from './components/bot-skills.vue'
import BotHeartbeat from './components/bot-heartbeat.vue'
import BotCompaction from './components/bot-compaction.vue'
import BotEmail from './components/bot-email.vue'
import BotOverview from './components/bot-overview.vue'
import BotSchedule from './components/bot-schedule.vue'
import BotContainer from './components/bot-container.vue'
import BotAccess from './components/bot-access.vue'
import AvatarEditDialog from './components/avatar-edit-dialog.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import { isLocalWorkspaceBot } from '@/utils/bot-workspace'
type BotCheck = BotsBotCheck
type BotContainerInfo = HandlersGetContainerResponse
type BotContainerSnapshot = HandlersListSnapshotsResponse extends { snapshots?: (infer T)[] } ? T : never

const route = useRoute()
const { t } = useI18n()
const botId = computed(() => {
  const id = route.params.botId
  return typeof id === 'string' ? id : ''
})

const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

const containerInfo = ref<BotContainerInfo | null>(null)

const isLocalWorkspace = computed(() =>
  isLocalWorkspaceBot(bot.value?.metadata, containerInfo.value?.workspace_backend),
)

const tabList = computed(() => {
  const bot_id = toValue(botId)
  const tabs = [
    { value: 'overview', label: 'bots.tabs.overview', icon: LayoutDashboard, component: BotOverview, params: {} },
    { value: 'general', label: 'bots.tabs.general', icon: Settings, component: BotSettings, params: { 'bot-id': bot_id, 'bot-type': bot.value?.type } },
    { value: 'desktop', label: 'bots.tabs.desktop', icon: Monitor, component: BotDesktop, params: { 'bot-id': bot_id } },
    { value: 'container', label: 'bots.tabs.container', icon: Server, component: BotContainer, params: {} },
    { value: 'network', label: 'bots.tabs.network', icon: Globe, component: BotNetwork, params: { 'bot-id': bot_id } },
    { value: 'memory', label: 'bots.tabs.memory', icon: Database, component: BotMemory, params: { 'bot-id': bot_id } },
    { value: 'channels', label: 'bots.tabs.channels', icon: MessageSquare, component: BotChannels, params: { 'bot-id': bot_id } },
    { value: 'access', label: 'bots.tabs.access', icon: ShieldAlert, component: BotAccess, params: { 'bot-id': bot_id, 'bot-type': bot.value?.type } },
    { value: 'tool-approval', label: 'bots.tabs.toolApproval', icon: Zap, component: BotToolApproval, params: { 'bot-id': bot_id } },
    { value: 'email', label: 'bots.tabs.email', icon: Mail, component: BotEmail, params: { 'bot-id': bot_id } },
    { value: 'mcp', label: 'bots.tabs.mcp', icon: Link, component: BotMcp, params: { 'bot-id': bot_id } },
    { value: 'heartbeat', label: 'bots.tabs.heartbeat', icon: HeartPulse, component: BotHeartbeat, params: { 'bot-id': bot_id } },
    { value: 'compaction', label: 'bots.tabs.compaction', icon: FileBox, component: BotCompaction, params: { 'bot-id': bot_id } },
    { value: 'schedule', label: 'bots.tabs.schedule', icon: Clock, component: BotSchedule, params: { 'bot-id': bot_id } },
    { value: 'skills', label: 'bots.tabs.skills', icon: BrainCircuit, component: BotSkills, params: { 'bot-id': bot_id } },
  ]
  if (isLocalWorkspace.value) {
    return tabs.filter(tab => tab.value !== 'container' && tab.value !== 'network' && tab.value !== 'desktop')
  }
  return tabs
})

const searchQuery = ref('')

const searchIndex = computed(() => {
  return [
    { tab: 'general', key: 'bots.settings.blocks.global', keywords: ['name', 'avatar', 'description', 'timezone'] },
    { tab: 'general', key: 'bots.settings.blocks.interaction', keywords: ['language', 'chat model', 'reasoning'] },
    { tab: 'general', key: 'bots.settings.blocks.context', keywords: ['browser', 'search', 'provider'] },
    { tab: 'general', key: 'bots.settings.blocks.multimedia', keywords: ['image', 'tts', 'transcription'] },
    { tab: 'general', key: 'bots.settings.dangerZone', keywords: ['delete', 'remove'] },
    { tab: 'container', key: 'bots.container.dataTitle', keywords: ['docker', 'image', 'gpu', 'volume'] },
    { tab: 'container', key: 'bots.container.metricsTitle', keywords: ['cpu', 'ram', 'storage'] },
    { tab: 'memory', key: 'bots.memory.title', keywords: ['vector', 'database', 'qdrant', 'embed'] },
    { tab: 'channels', key: 'bots.channels.configured', keywords: ['telegram', 'discord', 'wechat', 'slack'] },
    { tab: 'access', key: 'bots.access.title', keywords: ['permissions', 'acl', 'rules', 'allow', 'deny'] },
    { tab: 'tool-approval', key: 'bots.toolApproval.title', keywords: ['mcp', 'tools', 'review', 'bypass', 'approval'] },
    { tab: 'email', key: 'bots.email.title', keywords: ['smtp', 'imap', 'mailbox', 'bindings'] },
    { tab: 'mcp', key: 'bots.tabs.mcp', keywords: ['servers', 'connect', 'plugins'] },
    { tab: 'heartbeat', key: 'bots.heartbeat.title', keywords: ['cron', 'ping', 'alive'] },
    { tab: 'compaction', key: 'bots.compaction.title', keywords: ['compress', 'summarize', 'context window'] },
    { tab: 'schedule', key: 'bots.schedule.title', keywords: ['cron', 'jobs', 'tasks', 'automation'] },
    { tab: 'skills', key: 'bots.skills.title', keywords: ['prompts', 'instructions', 'system prompt'] },
  ].map(item => ({
    ...item,
    translatedTitle: t(item.key)
  }))
})

const searchResults = computed(() => {
  const query = searchQuery.value.toLowerCase().trim()
  if (!query) return []
  
  return searchIndex.value.filter(item => {
    return item.translatedTitle.toLowerCase().includes(query) || 
           item.keywords.some(k => k.toLowerCase().includes(query)) ||
           t(`bots.tabs.${item.tab}`).toLowerCase().includes(query) ||
           item.tab.toLowerCase().includes(query)
  })
})

const groupedTabs = computed(() => {
  const coreKeys = ['overview', 'general', 'channels']
  const capabilityKeys = ['skills', 'tool-approval', 'mcp', 'memory']
  const runtimeKeys = ['desktop', 'container', 'network', 'schedule', 'compaction', 'heartbeat']
  const securityKeys = ['access', 'email']

  return [
    { key: 'core', items: tabList.value.filter(t => coreKeys.includes(t.value)) },
    { key: 'capabilities', items: tabList.value.filter(t => capabilityKeys.includes(t.value)) },
    { key: 'runtime', items: tabList.value.filter(t => runtimeKeys.includes(t.value)) },
    { key: 'security', items: tabList.value.filter(t => securityKeys.includes(t.value)) },
  ].filter(g => g.items.length > 0)
})

const isActive = (name: string) => computed(() => {
  return activeTab.value === name
})

const activeComponent = computed(() => {
  return tabList.value.find(tab => tab.value === activeTab.value)
})

const capabilitiesStore = useCapabilitiesStore()
onMounted(() => {
  void capabilitiesStore.load()
})

const queryCache = useQueryCache()
const { mutateAsync: updateBot, isLoading: updateBotLoading } = useMutation({
  mutation: async ({ id, ...body }: Record<string, unknown> & { id: string }) => {
    const { data } = await putBotsById({ path: { id }, body, throwOnError: true })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: getBotsQueryKey() })
    queryCache.invalidateQueries({ key: ['bot'] })
  },
})

async function fetchChecks(id: string): Promise<BotCheck[]> {
  const { data } = await getBotsByIdChecks({ path: { id }, throwOnError: true })
  return data?.items ?? []
}

const isEditingBotName = ref(false)
const botNameDraft = ref('')
const editNameInputRef = ref<InstanceType<typeof Input> | null>(null)

// Replace breadcrumb bot id with display name when available.
watch(bot, (val) => {
  if (!val) return
  const currentName = (val.display_name || '').trim()
  if (currentName) {
    route.meta.breadcrumb = () => currentName
  }
  if (!isEditingBotName.value) {
    botNameDraft.value = val.display_name || ''
  }
}, { immediate: true })

const activeTab = useSyncedQueryParam('tab', 'overview')
watch([tabList, activeTab], ([tabs, tab]) => {
  if (!tabs.some(item => item.value === tab)) {
    activeTab.value = 'overview'
  }
}, { immediate: true })
const avatarDialogOpen = ref(false)
const avatarUrlModel = ref('')
const avatarFallback = useAvatarInitials(() => bot.value?.display_name || botId.value || '')
const isSavingBotName = computed(() => updateBotLoading.value)

watch(() => bot.value?.avatar_url, (url) => {
  avatarUrlModel.value = url || ''
}, { immediate: true })

watch(avatarUrlModel, async (nextUrl) => {
  if (!bot.value) return
  const current = (bot.value.avatar_url || '').trim()
  if (nextUrl.trim() === current) return
  try {
    await updateBot({
      id: bot.value.id as string,
      avatar_url: nextUrl || undefined,
    })
    toast.success(t('bots.avatarUpdateSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.avatarUpdateFailed')))
  }
})
const canConfirmBotName = computed(() => {
  if (!bot.value) return false
  const nextName = botNameDraft.value.trim()
  if (!nextName) return false
  return nextName !== (bot.value.display_name || '').trim()
})
const {
  hasIssue,
  isPending: botLifecyclePending,
  issueTitle,
  statusLabel,
  statusVariant,
} = useBotStatusMeta(bot, t)

const botTypeLabel = computed(() => {
  const type = bot.value?.type
  if (type === 'personal' || type === 'public') return t('bots.types.' + type)
  return type ?? ''
})

const checks = ref<BotCheck[]>([])
const checksLoading = ref(false)

const containerMissing = ref(false)
const containerLoading = ref(false)
const snapshotsLoading = ref(false)
const snapshots = ref<BotContainerSnapshot[]>([])

watch(botId, () => {
  isEditingBotName.value = false
  botNameDraft.value = ''
})

watch([activeTab, botId], ([tab]) => {
  if (!botId.value) {
    return
  }
  if (tab === 'container') {
    void loadContainerData(true)
    return
  }
  if (tab === 'overview') {
    void loadChecks(true)
  }
}, { immediate: true })

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

function handleEditAvatar() {
  if (!bot.value || botLifecyclePending.value) return
  avatarDialogOpen.value = true
}

function handleStartEditBotName() {
  if (!bot.value) return
  isEditingBotName.value = true
  botNameDraft.value = bot.value.display_name || ''
  nextTick(() => {
    const el = editNameInputRef.value?.$el
    if (el) {
      const input = el instanceof HTMLInputElement ? el : el.querySelector('input')
      if (input) input.focus()
    }
  })
}

function handleCancelBotName() {
  isEditingBotName.value = false
  botNameDraft.value = bot.value?.display_name || ''
}

async function handleConfirmBotName() {
  if (!bot.value || !canConfirmBotName.value) {
    handleCancelBotName()
    return
  }
  const nextName = botNameDraft.value.trim()
  try {
    await updateBot({
      id: bot.value.id as string,
      display_name: nextName,
    })
    route.meta.breadcrumb = () => nextName
    isEditingBotName.value = false
    toast.success(t('bots.renameSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.renameFailed')))
  }
}

async function loadChecks(showToast: boolean) {
  checksLoading.value = true
  checks.value = []
  try {
    checks.value = await fetchChecks(botId.value)
  } catch (error) {
    if (showToast) {
      toast.error(resolveErrorMessage(error, t('bots.checks.loadFailed')))
    }
  } finally {
    checksLoading.value = false
  }
}

async function loadContainerData(showLoadingToast: boolean) {
  await capabilitiesStore.load()
  containerLoading.value = true
  try {
    const result = await getBotsByBotIdContainer({ path: { bot_id: botId.value } })
    if (result.error !== undefined) {
      if (result.response.status === 404) {
        containerInfo.value = null
        containerMissing.value = true
        snapshots.value = []
        return
      }
      throw result.error
    }
    containerInfo.value = result.data
    containerMissing.value = false
    if (capabilitiesStore.snapshotSupported) {
      await loadSnapshots()
    }
  } catch (error) {
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t('bots.container.loadFailed')))
    }
  } finally {
    containerLoading.value = false
  }
}

async function loadSnapshots() {
  if (!containerInfo.value || !capabilitiesStore.snapshotSupported) {
    snapshots.value = []
    return
  }
  snapshotsLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerSnapshots({ path: { bot_id: botId.value }, throwOnError: true })
    snapshots.value = data.snapshots ?? []
  } catch (error) {
    snapshots.value = []
    toast.error(resolveErrorMessage(error, t('bots.container.snapshotLoadFailed')))
  } finally {
    snapshotsLoading.value = false
  }
}
</script>
