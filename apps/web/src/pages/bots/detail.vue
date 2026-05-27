<template>
  <section class="absolute inset-0 flex flex-col bg-background">
    <div class="flex-1 relative">
      <BotDetailSidebar
        v-model:active-tab="activeTab"
        v-model:bot-name-draft="botNameDraft"
        v-model:search-query="searchQuery"
        :avatar-fallback="avatarFallback"
        :bot="bot"
        :bot-id="botId"
        :bot-lifecycle-pending="botLifecyclePending"
        :bot-type-label="botTypeLabel"
        :grouped-tabs="groupedTabs"
        :has-issue="hasIssue"
        :is-editing-bot-name="isEditingBotName"
        :is-saving-bot-name="isSavingBotName"
        :issue-title="issueTitle"
        :search-results="searchResults"
        :status-label="statusLabel"
        :status-variant="statusVariant"
        :tab-list="tabList"
        @cancel-bot-name="handleCancelBotName"
        @confirm-bot-name="handleConfirmBotName"
        @edit-avatar="handleEditAvatar"
        @start-edit-bot-name="handleStartEditBotName"
      >
        <template #detail>
          <div class="absolute inset-0 overflow-y-auto bg-background">
            <div class="px-6 pt-4 pb-4">
              <KeepAlive>
                <component
                  :is="activeComponent?.component"
                  v-bind="activeComponent?.params"
                />
              </KeepAlive>
            </div>
          </div>
        </template>
      </BotDetailSidebar>
    </div>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="avatarUrlModel"
      :fallback-text="avatarFallback"
      :title="t('bots.editAvatar')"
      :description="t('bots.editAvatarDescription')"
      :placeholder="t('bots.avatarUrlPlaceholder')"
    />
  </section>
</template>

<script setup lang="ts">
import {
  LayoutDashboard, Settings, MessageSquare,
  BrainCircuit, ShieldAlert, HeartPulse, Database, Mail, Link, Clock, Server, FileBox, Zap,
  Monitor, Globe
} from 'lucide-vue-next'
import { computed, ref, watch, onMounted, toValue } from 'vue'
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
import AvatarEditDialog from '@/components/avatar-edit-dialog/index.vue'
import BotDetailSidebar from './components/bot-detail-sidebar.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
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
