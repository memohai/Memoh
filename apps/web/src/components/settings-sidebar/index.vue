<template>
  <aside class="relative h-full">
    <header
      v-if="macTopInset"
      class="fixed top-0 left-0 z-20 h-9 w-(--sidebar-width) bg-sidebar border-r border-sidebar-border [-webkit-app-region:drag]"
    />

    <Sidebar
      :collapsible="desktopShell ? 'none' : 'icon'"
      :class="macTopInset ? 'pt-9 h-dvh border-r border-sidebar-border' : desktopShell ? 'h-dvh border-r border-sidebar-border' : ''"
    >
      <SidebarHeader
        v-if="!hideHeader"
        class="px-4 pt-2 pb-6 border-0"
      >
        <SidebarMenu>
          <SidebarMenuItem>
            <Button
              variant="ghost"
              block
              class="justify-start font-semibold group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:px-0!"
              @click="router.push(backToChatRoute)"
            >
              <ChevronLeft class="size-3.5 shrink-0" />
              <span class="group-data-[collapsible=icon]:hidden">{{ t('sidebar.settings') }}</span>
            </Button>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup class="px-4 py-0">
          <SidebarGroupContent>
            <SidebarMenu class="gap-2">
              <SidebarMenuItem
                v-for="item in coreNavItems"
                :key="item.name"
              >
                <Button
                  variant="ghost"
                  block
                  class="justify-start group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:px-0!"
                  :class="isItemActive(item.name) && 'bg-sidebar-accent'"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="router.push({ name: item.name })"
                >
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </Button>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup
          v-if="integrationsNavItems.length"
          class="px-4 pt-4 pb-0"
        >
          <SidebarGroupLabel class="group-data-[collapsible=icon]:hidden">
            {{ t('sidebar.group.integrations') }}
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu class="gap-2">
              <SidebarMenuItem
                v-for="item in integrationsNavItems"
                :key="item.name"
              >
                <Button
                  variant="ghost"
                  block
                  class="justify-start group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:px-0!"
                  :class="isItemActive(item.name) && 'bg-sidebar-accent'"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="router.push({ name: item.name })"
                >
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </Button>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup
          v-if="accountNavItems.length"
          class="px-4 pt-4 pb-0"
        >
          <SidebarGroupLabel class="group-data-[collapsible=icon]:hidden">
            {{ t('sidebar.group.account') }}
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu class="gap-2">
              <SidebarMenuItem
                v-for="item in accountNavItems"
                :key="item.name"
              >
                <Button
                  variant="ghost"
                  block
                  class="justify-start group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:px-0!"
                  :class="isItemActive(item.name) && 'bg-sidebar-accent'"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="router.push({ name: item.name })"
                >
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </Button>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarRail v-if="!desktopShell" />
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft } from 'lucide-vue-next'
import { useChatSelectionStore } from '@/store/chat-selection'
import { useChatStore } from '@/store/chat-list'
import { useUserStore } from '@/store/user'
import {
  Button,
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarRail,
} from '@memohai/ui'
import { DesktopShellKey } from '@/lib/desktop-shell'

const props = withDefaults(defineProps<{
  hideHeader?: boolean
  excludeItems?: string[]
}>(), {
  hideHeader: false,
  excludeItems: () => [],
})

const desktopShell = inject(DesktopShellKey, false)
const macTopInset = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const selectionStore = useChatSelectionStore()
const { currentBotId } = storeToRefs(selectionStore)
const chatStore = useChatStore()
const { bots } = storeToRefs(chatStore)
const userStore = useUserStore()
const { userInfo } = storeToRefs(userStore)

const backToChatRoute = computed(() => {
  const botId = (currentBotId.value ?? '').trim()
  if (!botId) return { name: 'home' as const }
  const botName = bots.value.find((b) => b.id === botId)?.name ?? botId
  return {
    name: 'bot' as const,
    params: { botName },
  }
})

function isItemActive(name: string): boolean {
  if (name === 'bots') {
    return route.path.startsWith('/settings/bots')
  }
  if (name === 'supermarket') {
    return route.path.startsWith('/settings/supermarket')
  }
  return route.name === name
}

type NavItem = { title: string; name: string; adminOnly?: boolean }

function filterItems(items: NavItem[]): NavItem[] {
  return items.filter((item) => {
    if (item.adminOnly && userInfo.value.role !== 'admin') return false
    return props.excludeItems.length === 0 || !props.excludeItems.includes(item.name)
  })
}

const coreNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.bots'), name: 'bots' },
  { title: t('sidebar.providers'), name: 'providers' },
  { title: t('sidebar.memory'), name: 'memory' },
  { title: t('sidebar.webSearch'), name: 'web-search' },
  { title: t('sidebar.speech'), name: 'speech' },
  { title: t('sidebar.transcription'), name: 'transcription' },
]))

const integrationsNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.email'), name: 'email' },
  { title: t('sidebar.supermarket'), name: 'supermarket' },
  { title: t('sidebar.usage'), name: 'usage' },
  { title: t('sidebar.people'), name: 'people', adminOnly: true },
]))

const accountNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.appearance'), name: 'appearance' },
  { title: t('sidebar.profile'), name: 'profile' },
  { title: t('sidebar.about'), name: 'about' },
]))
</script>
