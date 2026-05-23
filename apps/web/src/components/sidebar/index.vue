<template>
  <aside class="relative h-full">
    <header
      v-if="macTopInset"
      class="fixed top-0 left-0 z-20 h-9 w-(--sidebar-width) flex items-center pl-[78px] pr-2 gap-1 bg-sidebar border-r border-sidebar-border [-webkit-app-region:drag]"
    >
      <div class="ml-auto flex items-center gap-1 [-webkit-app-region:no-drag]">
        <Button
          variant="ghost"
          size="icon"
          class="size-6 text-muted-foreground hover:text-foreground shrink-0"
          :aria-label="t('projects.createProject')"
          @click="openProjectDialog"
        >
          <Plus class="size-3.5" />
        </Button>
      </div>
    </header>

    <Sidebar
      :collapsible="desktopShell ? 'none' : 'icon'"
      :class="macTopInset ? 'pt-9 h-dvh border-r border-sidebar-border' : desktopShell ? 'h-dvh border-r border-sidebar-border' : ''"
    >
      <SidebarHeader
        v-if="!desktopShell"
        class="p-0 border-0"
      >
        <div class="h-10 flex items-center pl-2 group-data-[collapsible=icon]:pl-3 transition-[padding] duration-200 ease-linear">
          <Button
            variant="ghost"
            size="icon"
            class="size-6 text-muted-foreground hover:text-foreground shrink-0"
            :aria-label="t('projects.toggleMainSidebar')"
            @click="toggleSidebar"
          >
            <PanelLeftClose class="size-3.5 group-data-[collapsible=icon]:hidden" />
            <PanelLeftOpen class="size-3.5 hidden group-data-[collapsible=icon]:block" />
          </Button>

          <div class="ml-auto mr-1.5 group-data-[collapsible=icon]:hidden">
            <Button
              variant="ghost"
              size="icon"
              class="size-6 text-muted-foreground hover:text-foreground shrink-0"
              :aria-label="t('projects.createProject')"
              @click="openProjectDialog"
            >
              <Plus class="size-3.5" />
            </Button>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent class="@container/projects">
        <SidebarGroup class="px-2 py-1.5">
          <SidebarGroupContent>
            <SidebarMenu class="gap-1">
              <SidebarMenuItem>
                <SidebarMenuButton
                  :tooltip="t('projects.newConversation')"
                  class="h-8.5 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  @click="handleNewConversation"
                >
                  <SquarePen class="size-3.5" />
                  <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('projects.newConversation') }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  :tooltip="t('projects.search')"
                  class="h-8.5 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  @click="openSearchDialog"
                >
                  <Search class="size-3.5" />
                  <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('projects.search') }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  :tooltip="t('projects.plugins')"
                  class="h-8.5 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  @click="openActivity('mcp')"
                >
                  <Plug class="size-3.5" />
                  <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('projects.plugins') }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  :tooltip="t('projects.automation')"
                  class="h-8.5 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  @click="openActivity('schedule')"
                >
                  <CalendarClock class="size-3.5" />
                  <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('projects.automation') }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup class="px-2 py-2">
          <div class="mb-1 flex h-7 items-center gap-1 group-data-[collapsible=icon]:justify-center">
            <Button
              variant="ghost"
              size="icon"
              class="size-6 text-muted-foreground hover:text-foreground"
              :aria-label="projectsExpanded ? t('projects.collapseProjects') : t('projects.expandProjects')"
              @click="projectsExpanded = !projectsExpanded"
            >
              <ChevronDown
                class="size-3.5 transition-transform group-data-[collapsible=icon]:hidden"
                :class="{ '-rotate-90': !projectsExpanded }"
              />
              <FolderOpen class="size-3.5 hidden group-data-[collapsible=icon]:block" />
            </Button>
            <span class="min-w-0 flex-1 text-[10px] font-semibold uppercase text-muted-foreground group-data-[collapsible=icon]:hidden">
              {{ t('projects.title') }}
            </span>
            <DropdownMenu>
              <DropdownMenuTrigger
                as-child
                class="group-data-[collapsible=icon]:hidden"
              >
                <Button
                  variant="ghost"
                  size="icon"
                  class="size-6 text-muted-foreground hover:text-foreground"
                  :aria-label="t('projects.more')"
                >
                  <Ellipsis class="size-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="start"
                side="bottom"
              >
                <DropdownMenuItem @click="router.push({ name: 'bots' })">
                  <Settings class="mr-2 size-3.5" />
                  {{ t('projects.manageProjects') }}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              variant="ghost"
              size="icon"
              class="size-6 text-muted-foreground hover:text-foreground group-data-[collapsible=icon]:hidden"
              :aria-label="t('projects.createProject')"
              @click="openProjectDialog"
            >
              <FolderPlus class="size-3.5" />
            </Button>
          </div>

          <SidebarGroupContent v-show="projectsExpanded">
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="bot in bots"
                :key="bot.id"
              >
                <BotItem :bot="bot" />
              </SidebarMenuItem>
            </SidebarMenu>

            <div
              v-if="isLoading"
              class="flex justify-center py-4"
            >
              <LoaderCircle class="size-4 animate-spin text-muted-foreground" />
            </div>
            <div
              v-if="!isLoading && bots.length === 0"
              class="px-2 py-5 text-center text-xs text-muted-foreground @max-[50px]/projects:hidden"
            >
              <p>{{ t('projects.emptyTitle') }}</p>
              <Button
                variant="outline"
                size="sm"
                class="mt-3 h-7 text-xs"
                @click="openProjectDialog"
              >
                <Plus class="mr-1 size-3" />
                {{ t('projects.createProject') }}
              </Button>
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter class="relative border-0 px-2 pb-3.5 pt-2.5">
        <div class="pointer-events-none absolute -top-30 left-0 h-38.25 w-full bg-linear-to-t from-(--sidebar-background) from-18% to-transparent z-10 group-data-[collapsible=icon]:hidden" />
        <SidebarMenu class="gap-2.5">
          <SidebarMenuItem>
            <SidebarMenuButton
              :tooltip="t('sidebar.settings')"
              class="h-9 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
              :is-active="isSettingsActive"
              @click="router.push('/settings')"
            >
              <Settings class="size-3.5" />
              <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('sidebar.settings') }}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>

      <SidebarRail v-if="!desktopShell" />
    </Sidebar>

    <Dialog v-model:open="projectDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('projects.createProject') }}</DialogTitle>
          <DialogDescription>{{ t('projects.createProjectDescription') }}</DialogDescription>
        </DialogHeader>
        <form
          class="space-y-4"
          @submit.prevent="handleCreateProject"
        >
          <div class="space-y-2">
            <Label>
              {{ t('projects.projectName') }}
              <span class="text-destructive">*</span>
            </Label>
            <Input
              v-model="projectForm.displayName"
              :placeholder="t('projects.projectNamePlaceholder')"
              :disabled="createProjectLoading"
              autofocus
            />
          </div>
          <div
            v-if="localWorkspaceEnabled"
            class="space-y-2"
          >
            <Label>{{ t('projects.workspacePath') }}</Label>
            <Input
              v-model="projectForm.localWorkspacePath"
              :placeholder="t('projects.workspacePathPlaceholder')"
              :disabled="createProjectLoading"
            />
            <p class="text-xs text-muted-foreground">
              {{ t('projects.workspacePathHint') }}
            </p>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="createProjectLoading"
              @click="projectDialogOpen = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              :disabled="!canCreateProject || createProjectLoading"
            >
              <LoaderCircle
                v-if="createProjectLoading"
                class="mr-1 size-3 animate-spin"
              />
              {{ t('common.create') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="searchDialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ t('projects.search') }}</DialogTitle>
          <DialogDescription>{{ t('projects.searchDescription') }}</DialogDescription>
        </DialogHeader>
        <InputGroup class="h-9">
          <InputGroupAddon class="pl-2.5">
            <Search class="size-3.5 text-muted-foreground" />
          </InputGroupAddon>
          <InputGroupInput
            v-model="searchQuery"
            :placeholder="t('projects.searchPlaceholder')"
            class="text-sm"
            autofocus
          />
        </InputGroup>
        <div class="max-h-80 overflow-y-auto">
          <div class="mb-4">
            <p class="mb-1.5 text-[10px] font-semibold uppercase text-muted-foreground">
              {{ t('projects.title') }}
            </p>
            <button
              v-for="bot in filteredBots"
              :key="bot.id"
              type="button"
              class="flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent"
              @click="selectProject(bot)"
            >
              <FolderOpen class="size-3.5 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate">{{ bot.display_name || bot.id }}</span>
            </button>
            <p
              v-if="filteredBots.length === 0"
              class="px-2 py-3 text-xs text-muted-foreground"
            >
              {{ t('projects.noProjectResults') }}
            </p>
          </div>
          <div>
            <p class="mb-1.5 text-[10px] font-semibold uppercase text-muted-foreground">
              {{ t('chat.sessions') }}
            </p>
            <button
              v-for="session in filteredSessions"
              :key="session.id"
              type="button"
              class="flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent"
              @click="selectSession(session.id)"
            >
              <MessageSquare class="size-3.5 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate">{{ session.title || t('chat.untitledSession') }}</span>
            </button>
            <p
              v-if="filteredSessions.length === 0"
              class="px-2 py-3 text-xs text-muted-foreground"
            >
              {{ t('projects.noSessionResults') }}
            </p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  </aside>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useLocalStorage } from '@vueuse/core'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsQuery, getBotsQueryKey, postBotsMutation } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Input,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  Label,
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@memohai/ui'
import {
  CalendarClock,
  ChevronDown,
  Ellipsis,
  FolderOpen,
  FolderPlus,
  LoaderCircle,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  Plug,
  Plus,
  Search,
  Settings,
  SquarePen,
} from 'lucide-vue-next'
import BotItem from './bot-item.vue'
import { usePinnedBots } from '@/composables/usePinnedBots'
import { DesktopShellKey } from '@/lib/desktop-shell'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useCapabilitiesStore } from '@/store/capabilities'
import { defaultAclPreset } from '@/constants/acl-presets'
import { resolveApiErrorMessage } from '@/utils/api-error'

type ActivityTabId = 'sessions' | 'files' | 'skills' | 'mcp' | 'schedule'

const CHAT_SIDEBAR_TAB_EVENT = 'memoh:chat-sidebar-tab'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const { toggleSidebar } = useSidebar()
const queryCache = useQueryCache()
const desktopShell = inject(DesktopShellKey, false)
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const capabilities = useCapabilitiesStore()
const { currentBotId, sessions } = storeToRefs(chatStore)
const macTopInset = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)
const { sortBots } = usePinnedBots()

void capabilities.load(true)

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => sortBots(botData.value?.items ?? []))

const projectsExpanded = useLocalStorage('projects-sidebar-expanded', true)
const projectDialogOpen = ref(false)
const searchDialogOpen = ref(false)
const searchQuery = ref('')
const localPathTouched = ref(false)

const projectForm = reactive({
  displayName: '',
  localWorkspacePath: '',
})

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)
const canCreateProject = computed(() => {
  if (!projectForm.displayName.trim()) return false
  return true
})

const isSettingsActive = computed(() => route.path.startsWith('/settings'))
const readyBots = computed(() => bots.value.filter((bot) => bot.status !== 'error' && bot.status !== 'deleting'))

const filteredBots = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return bots.value
  return bots.value.filter((bot) =>
    (bot.display_name ?? '').toLowerCase().includes(q)
    || (bot.id ?? '').toLowerCase().includes(q),
  )
})

const filteredSessions = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const list = sessions.value
  if (!q) return list
  return list.filter((session) =>
    (session.title ?? '').toLowerCase().includes(q)
    || (session.id ?? '').toLowerCase().includes(q),
  )
})

watch(projectDialogOpen, (open) => {
  if (!open) return
  if (!projectForm.displayName.trim()) {
    projectForm.displayName = t('projects.defaultProjectName')
  }
})

watch([() => projectForm.displayName, () => capabilities.localWorkspaceParent], async ([displayName]) => {
  if (!localWorkspaceEnabled.value || localPathTouched.value || !displayName?.trim()) return
  try {
    const api = (window as Record<string, unknown>).api as
      | { desktop?: { defaultWorkspacePath?: (name: string) => Promise<string> } }
      | undefined
    const path = await api?.desktop?.defaultWorkspacePath?.(displayName.trim())
    if (path && !localPathTouched.value) {
      projectForm.localWorkspacePath = path
      return
    }
  } catch {
    // Desktop IPC is optional; web builds fall back to the configured parent path.
  }
  if (!localPathTouched.value) {
    projectForm.localWorkspacePath = defaultLocalWorkspacePath(displayName.trim(), capabilities.localWorkspaceParent)
  }
}, { immediate: true })

watch(() => projectForm.localWorkspacePath, () => {
  localPathTouched.value = true
})

const { mutateAsync: createProject, isLoading: createProjectLoading } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

function defaultLocalWorkspacePath(displayName: string, parent: string) {
  const safeName = displayName
    .trim()
    .replace(/[<>:"/\\|?*\x00-\x1F]/g, '-')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  if (!safeName || !parent.trim()) return ''
  const separator = parent.includes('\\') ? '\\' : '/'
  return `${parent.replace(/[\\/]+$/, '')}${separator}${safeName}`
}

function openProjectDialog() {
  projectDialogOpen.value = true
}

async function ensureProject(): Promise<string | null> {
  const activeId = (currentBotId.value ?? '').trim()
  if (activeId) return activeId
  const first = readyBots.value[0]
  if (first?.id) {
    await selectProject(first, false)
    return first.id
  }
  openProjectDialog()
  return null
}

async function selectProject(bot: BotsBot, closeSearch = true) {
  if (bot.status === 'error' || !bot.id) return
  await chatStore.selectBot(bot.id)
  await router.push({ name: 'chat', params: { botId: bot.id } })
  if (closeSearch) searchDialogOpen.value = false
}

async function handleNewConversation() {
  const botId = await ensureProject()
  if (!botId) return
  await router.push({ name: 'chat', params: { botId } })
  dispatchChatSidebarTab('sessions')
  workspaceTabs.openDraft()
}

async function openActivity(tab: ActivityTabId) {
  const botId = await ensureProject()
  if (!botId) return
  await router.push({ name: 'chat', params: { botId } })
  dispatchChatSidebarTab(tab)
}

function openSearchDialog() {
  searchDialogOpen.value = true
}

async function selectSession(sessionId: string) {
  const botId = await ensureProject()
  if (!botId) return
  await router.push({ name: 'chat', params: { botId } })
  workspaceTabs.openChat(sessionId, sessions.value.find((s) => s.id === sessionId)?.title ?? '')
  dispatchChatSidebarTab('sessions')
  searchDialogOpen.value = false
}

function dispatchChatSidebarTab(tab: ActivityTabId) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(CHAT_SIDEBAR_TAB_EVENT, { detail: { tab } }))
}

async function handleCreateProject() {
  if (!canCreateProject.value || createProjectLoading.value) return
  const displayName = projectForm.displayName.trim()
  const metadata = localWorkspaceEnabled.value && projectForm.localWorkspacePath.trim()
    ? {
        workspace: {
          backend: 'local',
          local_workspace_path: projectForm.localWorkspacePath.trim(),
        },
      }
    : undefined

  try {
    const bot = await createProject({
      body: {
        display_name: displayName,
        is_active: true,
        acl_preset: defaultAclPreset,
        metadata,
        wait_for_ready: true,
      },
    })
    const botId = bot?.id
    projectDialogOpen.value = false
    projectForm.displayName = ''
    projectForm.localWorkspacePath = ''
    localPathTouched.value = false
    if (botId) {
      await chatStore.selectBot(botId)
      await router.push({ name: 'chat', params: { botId } })
      workspaceTabs.openDraft()
      toast.success(t('projects.createProjectSuccess'))
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('projects.createProjectFailed')))
  }
}
</script>
