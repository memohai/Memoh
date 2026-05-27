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
          :aria-label="t('bots.createBot')"
          @click="router.push('/settings/bots')"
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
            aria-label="Toggle Sidebar"
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
              :aria-label="t('bots.createBot')"
              @click="router.push({ name: 'bots' })"
            >
              <Plus class="size-3.5" />
            </Button>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent class="@container/bots">
        <SidebarGroup class="px-2 py-0">
          <SidebarGroupLabel class="text-[10px] uppercase text-muted-foreground tracking-wide">
            {{ t('sidebar.bots') }}
          </SidebarGroupLabel>
          <SidebarGroupContent>
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
              <LoaderCircle
                class="size-4 animate-spin text-muted-foreground"
              />
            </div>
            <div
              v-if="!isLoading && bots.length === 0"
              class="px-3 py-6 text-center text-xs text-muted-foreground @max-[50px]/bots:hidden"
            >
              {{ t('bots.emptyTitle') }}
            </div>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup
          v-if="teams.length > 0"
          class="px-2 py-0 group-data-[collapsible=icon]:mt-3"
        >
          <SidebarGroupLabel class="text-[10px] uppercase text-muted-foreground tracking-wide">
            {{ t('sidebar.teams') }}
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="team in teams"
                :key="team.id"
              >
                <SidebarMenuButton
                  :tooltip="team.name"
                  as-child
                >
                  <button
                    :class="[
                      'group/team flex items-center gap-2.5 w-full h-9.5 px-2.5 rounded-lg transition-colors',
                      'group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0',
                      isTeamActive(team.id)
                        ? 'bg-sidebar-accent'
                        : 'hover:bg-sidebar-accent/60',
                    ]"
                    @click="goToTeam(team.id)"
                  >
                    <div class="size-6.5 shrink-0 rounded-full border border-border bg-accent overflow-hidden p-px group-data-[collapsible=icon]:mx-auto">
                      <img
                        v-if="team.avatar_url && !teamImageError[team.id ?? '']"
                        :src="team.avatar_url"
                        :alt="team.name"
                        class="size-full rounded-full object-cover"
                        @error="() => { if (team.id) teamImageError[team.id] = true }"
                      >
                      <span
                        v-else
                        class="size-full flex items-center justify-center text-[8px] font-medium text-muted-foreground"
                      >
                        <Users
                          v-if="!teamInitials(team)"
                          class="size-3"
                        />
                        <template v-else>
                          {{ teamInitials(team) }}
                        </template>
                      </span>
                    </div>
                    <span class="text-xs font-medium text-foreground truncate flex-1 text-left group-data-[collapsible=icon]:hidden">{{ team.name }}</span>
                  </button>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
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
              <Settings
                class="size-3.5"
              />
              <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('sidebar.settings') }}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>

      <SidebarRail v-if="!desktopShell" />
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import { computed, inject, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import {
  Button,
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@memohai/ui'
import { Plus, LoaderCircle, Settings, PanelLeftClose, PanelLeftOpen, Users } from 'lucide-vue-next'
import { getTeams } from '@memohai/sdk'
import BotItem from './bot-item.vue'
import { usePinnedBots } from '@/composables/usePinnedBots'
import { DesktopShellKey } from '@/lib/desktop-shell'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const { toggleSidebar } = useSidebar()
const desktopShell = inject(DesktopShellKey, false)
const macTopInset = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)
const { sortBots } = usePinnedBots()

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => sortBots(botData.value?.items ?? []))

const { data: teamsData } = useQuery({
  key: () => ['teams'],
  query: async () => {
    const { data, error } = await getTeams()
    if (error) throw error
    return data ?? []
  },
})
const teams = computed(() => teamsData.value ?? [])
// 头像加载失败的 team id 集合：渲染时降级到字母 / Users 图标。
const teamImageError = reactive<Record<string, boolean>>({})

const isSettingsActive = computed(() => route.path.startsWith('/settings'))

function isTeamActive(teamId: string | undefined): boolean {
  if (!teamId) return false
  return route.path.startsWith(`/teams/${teamId}`)
}

function goToTeam(teamId: string | undefined) {
  if (!teamId) return
  router.push({ name: 'team-workspace', params: { teamId } })
}

function teamInitials(team: { name?: string }): string {
  const label = (team.name ?? '').trim()
  if (!label) return ''
  return label.slice(0, 2).toUpperCase()
}
</script>
