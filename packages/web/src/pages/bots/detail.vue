<template>
  <section class=" mx-auto absolute inset-0  flex flex-col">
    <!-- Header -->
    <div class="flex p-4 items-center gap-4">
      <div class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden">
        <Avatar class="size-16 rounded-full">
          <AvatarImage
            v-if="bot?.avatar_url"
            :src="bot.avatar_url"
            :alt="bot.display_name"
          />
          <AvatarFallback class="text-xl">
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
        <button
          type="button"
          class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
          :title="$t('common.edit')"
          :aria-label="$t('common.edit')"
          :disabled="!bot || botLifecyclePending"
          @click="handleEditAvatar"
        >
          <FontAwesomeIcon
            :icon="['fas', 'pen-to-square']"
            class="size-6 text-white"
          />
        </button>
      </div>
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <template v-if="isEditingBotName && bot">
            <Input
              v-model="botNameDraft"
              class="h-9 max-w-xl"
              :placeholder="$t('bots.displayNamePlaceholder')"
              :disabled="isSavingBotName"
              @keydown.enter.prevent="handleConfirmBotName"
              @keydown.esc.prevent="handleCancelBotName"
            />
            <LoadingButton
              size="sm"
              :loading="isSavingBotName"
              :disabled="!canConfirmBotName"
              @click="handleConfirmBotName"
            >
              {{ $t('common.confirm') }}
            </LoadingButton>
            <Button
              variant="ghost"
              size="sm"
              :disabled="isSavingBotName"
              @click="handleCancelBotName"
            >
              {{ $t('common.cancel') }}
            </Button>
          </template>
          <template v-else>
            <h2 class="truncate text-2xl font-semibold tracking-tight">
              {{ botNameDraft.trim() || bot?.display_name || botId }}
            </h2>
            <Button
              v-if="bot"
              type="button"
              variant="ghost"
              size="sm"
              class="size-7 p-0"
              :disabled="botLifecyclePending"
              :title="$t('common.edit')"
              :aria-label="$t('common.edit')"
              @click="handleStartEditBotName"
            >
              <FontAwesomeIcon
                :icon="['fas', 'pen-to-square']"
                class="size-3.5"
              />
            </Button>
          </template>
        </div>
        <div class="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Badge
            v-if="bot"
            :variant="statusVariant"
            class="text-xs"
            :title="hasIssue ? issueTitle : undefined"
          >
            <FontAwesomeIcon
              v-if="bot.status === 'creating' || bot.status === 'deleting'"
              :icon="['fas', 'spinner']"
              class="mr-1 size-3 animate-spin"
            />
            {{ statusLabel }}
          </Badge>
          <span v-if="bot?.type">{{ botTypeLabel }}</span>
        </div>
      </div>
    </div>
    <Separator />
    <div class="flex-1  relative">
      <MasterDetailSidebarLayout class="[&_td:last-child]:w-45">
        <template #sidebar-content>
          <SidebarMenu
            v-for="tab in tabList"
            :key="tab.value"
          >
            <SidebarMenuItem>
              <SidebarMenuButton
                as-child
                class="justify-start py-5! px-4"
              >
                <Toggle
                  :class="`py-4 border border-transparent ${activeTab === tab.value ? 'border-inherit' : ''}`"
                  :model-value="isActive(tab.value as string).value"
                  @update:model-value="(isSelect:boolean) => {
                    if (isSelect) {
                      activeTab=tab.value
                    }
                  }"
                >
                  {{ $t(tab.label) }}
                </Toggle>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </template>

        <template #sidebar-footer>
          <!-- <AddProvider v-model:open="openStatus.provideOpen" /> -->
        </template>

        <template #detail>
          <ScrollArea class="max-h-full h-full">
            <section class="p-4">
              <KeepAlive>
                <component
                  :is="activeComponent?.component"                 
                  :bot-id="botId"
                  :bot-type="bot?.type"
                />
              </KeepAlive>
            </section>
          </ScrollArea>
          <!-- <ScrollArea
            v-if="curProvider?.id"
            class="max-h-full h-full"
          >
            <model-setting />
          </ScrollArea>
          <Empty
            v-else
            class="h-full flex justify-center items-center"
          >
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FontAwesomeIcon :icon="['far', 'rectangle-list']" />
              </EmptyMedia>
            </EmptyHeader>
            <EmptyTitle>{{ $t('provider.emptyTitle') }}</EmptyTitle>
            <EmptyDescription>{{ $t('provider.emptyDescription') }}</EmptyDescription>
            <EmptyContent>
              <Button
                variant="outline"
                @click="openStatus.provideOpen = true"
              >
                {{ $t('provider.addBtn') }}
              </Button>
            </EmptyContent>
          </Empty> -->
        </template>
      </MasterDetailSidebarLayout>
    </div>

    <!-- Edit avatar dialog -->
    <Dialog v-model:open="avatarDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.editAvatar') }}</DialogTitle>
          <DialogDescription>
            {{ $t('bots.editAvatarDescription') }}
          </DialogDescription>
        </DialogHeader>
        <div class="mt-4 flex flex-col items-center gap-4">
          <Avatar class="size-20 shrink-0 rounded-full">
            <AvatarImage
              v-if="avatarUrlDraft.trim()"
              :src="avatarUrlDraft.trim()"
              :alt="bot?.display_name"
            />
            <AvatarFallback class="text-xl">
              {{ avatarFallback }}
            </AvatarFallback>
          </Avatar>
          <Input
            v-model="avatarUrlDraft"
            type="url"
            class="w-full"
            :placeholder="$t('bots.avatarUrlPlaceholder')"
            :disabled="avatarSaving"
          />
        </div>
        <DialogFooter class="mt-6">
          <DialogClose as-child>
            <Button
              variant="outline"
              :disabled="avatarSaving"
            >
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            :disabled="avatarSaving || !canConfirmAvatar"
            @click="handleConfirmAvatar"
          >
            <Spinner
              v-if="avatarSaving"
              class="mr-1.5"
            />
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Separator,
  Spinner,
  ScrollArea,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Toggle
} from '@memoh/ui'
import { computed, ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import {
  getBotsById, putBotsById,
  getBotsByIdChecks,
  getBotsByBotIdContainer,
  getBotsByBotIdContainerSnapshots, 
} from '@memoh/sdk'
import type {
  BotsBotCheck, HandlersGetContainerResponse,
  HandlersListSnapshotsResponse,
} from '@memoh/sdk'
import { useCapabilitiesStore } from '@/store/capabilities'
import LoadingButton from '@/components/loading-button/index.vue'
import BotSettings from './components/bot-settings.vue'
import BotChannels from './components/bot-channels.vue'
import BotMcp from './components/bot-mcp.vue'
import BotMemory from './components/bot-memory.vue'
import BotSkills from './components/bot-skills.vue'
import BotHistory from './components/bot-history.vue'
import BotHeartbeat from './components/bot-heartbeat.vue'
import BotSubagents from './components/bot-subagents.vue'
import BotOverview from './components/bot-overview.vue'
import BotContainer from './components/bot-container.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

type BotCheck = BotsBotCheck
type BotContainerInfo = HandlersGetContainerResponse
type BotContainerSnapshot = HandlersListSnapshotsResponse extends { snapshots?: (infer T)[] } ? T : never

const route = useRoute()
const { t } = useI18n()
const botId = computed(() => route.params.botId as string)

const tabList = [
  { value: 'overview', label: 'bots.tabs.overview',component: BotOverview },
  { value: 'memory', label: 'bots.tabs.memory',component:BotMemory },
  { value: 'channels', label: 'bots.tabs.channels',component: BotChannels },
  { value: 'container', label: 'bots.tabs.container' , component: BotContainer },
  { value: 'mcp', label: 'bots.tabs.mcp' ,component: BotMcp },
  { value: 'subagents', label: 'bots.tabs.subagents',component: BotSubagents },
  { value: 'heartbeat', label: 'bots.tabs.heartbeat',component: BotHeartbeat },
  { value: 'history', label: 'bots.tabs.history',component: BotHistory },
  { value: 'skills', label: 'bots.tabs.skills',component: BotSkills },
  { value: 'settings', label: 'bots.tabs.settings',component: BotSettings }
]

const isActive = (name: string) => computed(() => {
  return activeTab.value===name
})

const activeComponent = computed(() => {
  return tabList.find(tab=>tab.value===activeTab.value)
})

const capabilitiesStore = useCapabilitiesStore()
onMounted(() => {
  void capabilitiesStore.load()
})

const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

const queryCache = useQueryCache()
const { mutateAsync: updateBot, isLoading: updateBotLoading } = useMutation({
  mutation: async ({ id, ...body }: Record<string, unknown> & { id: string }) => {
    const { data } = await putBotsById({ path: { id }, body, throwOnError: true })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bots'] })
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
const avatarDialogOpen = ref(false)
const avatarUrlDraft = ref('')
const avatarFallback = useAvatarInitials(() => bot.value?.display_name || botId.value || '')
const isSavingBotName = computed(() => updateBotLoading.value)
const avatarSaving = computed(() => updateBotLoading.value)
const canConfirmAvatar = computed(() => {
  if (!bot.value) return false
  const next = avatarUrlDraft.value.trim()
  const current = (bot.value.avatar_url || '').trim()
  return next !== current
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

const containerInfo = ref<BotContainerInfo | null>(null)
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
  avatarUrlDraft.value = bot.value.avatar_url || ''
  avatarDialogOpen.value = true
}

async function handleConfirmAvatar() {
  if (!bot.value || !canConfirmAvatar.value || avatarSaving.value) return
  const nextUrl = avatarUrlDraft.value.trim()
  try {
    await updateBot({
      id: bot.value.id as string,
      avatar_url: nextUrl || undefined,
    })
    avatarDialogOpen.value = false
    toast.success(t('bots.avatarUpdateSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.avatarUpdateFailed')))
  }
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