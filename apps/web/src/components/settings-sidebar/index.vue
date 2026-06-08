<template>
  <aside
    class="relative h-full"
    style="--sidebar-width: 15rem"
  >
    <Sidebar
      :collapsible="desktopShell ? 'none' : 'icon'"
      :class="desktopShell ? 'h-dvh border-r border-sidebar-border' : ''"
    >
      <SidebarHeader
        v-if="!hideHeader"
        class="px-[16px] pt-[18px] pb-3 border-0"
      >
        <NavItem @click="router.push(_backToChatRoute).catch(() => {})">
          <ChevronLeft class="size-3.5 shrink-0" />
          <span class="group-data-[collapsible=icon]:hidden">{{ t('sidebar.settings') }}</span>
        </NavItem>
      </SidebarHeader>

      <SidebarContent>
        <!-- Core group: no label -->
        <SidebarGroup class="px-[16px] pt-1 pb-0">
          <SidebarGroupContent>
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="item in coreNavItems"
                :key="item.name"
              >
                <NavItem
                  :active="isItemActive(item.name)"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="navigate(item.name)"
                >
                  <component
                    :is="item.icon"
                    :stroke-width="1.75"
                    class="size-4 shrink-0"
                    :class="item.flipX && '-scale-x-100'"
                  />
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </NavItem>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <!-- Integrations group -->
        <SidebarGroup
          v-if="integrationsNavItems.length"
          class="px-[16px] pt-4 pb-0"
        >
          <SidebarGroupLabel class="h-6! pl-[14px]! pr-3! font-[475] text-muted-foreground group-data-[collapsible=icon]:hidden">
            {{ t('sidebar.group.integrations') }}
          </SidebarGroupLabel>
          <SidebarGroupContent class="pt-0">
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="item in integrationsNavItems"
                :key="item.name"
              >
                <NavItem
                  :active="isItemActive(item.name)"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="navigate(item.name)"
                >
                  <component
                    :is="item.icon"
                    :stroke-width="1.75"
                    class="size-4 shrink-0"
                    :class="item.flipX && '-scale-x-100'"
                  />
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </NavItem>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <!-- Account group -->
        <SidebarGroup
          v-if="accountNavItems.length"
          class="px-[16px] pt-4 pb-0"
        >
          <SidebarGroupLabel class="h-6! pl-[14px]! pr-3! font-[475] text-muted-foreground group-data-[collapsible=icon]:hidden">
            {{ t('sidebar.group.account') }}
          </SidebarGroupLabel>
          <SidebarGroupContent class="pt-0">
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="item in accountNavItems"
                :key="item.name"
              >
                <NavItem
                  :active="isItemActive(item.name)"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  @click="navigate(item.name)"
                >
                  <component
                    :is="item.icon"
                    :stroke-width="1.75"
                    class="size-4 shrink-0"
                    :class="item.flipX && '-scale-x-100'"
                  />
                  <span class="group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </NavItem>
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
import { computed, inject, type Component } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  Box,
  Captions,
  ChartNoAxesColumn,
  ChevronLeft,
  CircleUserRound,
  Database,
  Globe,
  Info,
  Mail,
  MousePointer2,
  Store,
  Users,
  Volume2,
} from 'lucide-vue-next'
import AppearanceIcon from './appearance-icon.vue'
import { useChatSelectionStore } from '@/store/chat-selection'
import { useChatStore } from '@/store/chat-list'
import { useUserStore } from '@/store/user'
import {
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
import NavItem from './nav-item.vue'

const props = withDefaults(defineProps<{
  hideHeader?: boolean
  excludeItems?: string[]
}>(), {
  hideHeader: false,
  excludeItems: () => [],
})

defineEmits<{ back: [] }>()

const desktopShell = inject(DesktopShellKey, false)

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const selectionStore = useChatSelectionStore()
const { currentBotId } = storeToRefs(selectionStore)
const chatStore = useChatStore()
const { bots } = storeToRefs(chatStore)
const userStore = useUserStore()
const { userInfo } = storeToRefs(userStore)

const _backToChatRoute = computed(() => {
  const botId = (currentBotId.value ?? '').trim()
  if (!botId) return { name: 'home' as const }
  const botName = bots.value.find((b) => b.id === botId)?.name ?? botId
  return { name: 'bot' as const, params: { botName } }
})

function navigate(name: string): void {
  router.push({ name } as Parameters<typeof router.push>[0]).catch(() => {})
}

function isItemActive(name: string): boolean {
  if (name === 'bots') {
    return route.path.startsWith('/settings/bots')
  }
  if (name === 'supermarket') {
    return route.path.startsWith('/settings/supermarket')
  }
  return route.name === name
}

type NavItem = { title: string; name: string; icon: Component; flipX?: boolean; adminOnly?: boolean }

function filterItems(items: NavItem[]): NavItem[] {
  return items.filter((item) => {
    if (item.adminOnly && userInfo.value.role !== 'admin') return false
    return props.excludeItems.length === 0 || !props.excludeItems.includes(item.name)
  })
}

const coreNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.bots'), name: 'bots', icon: MousePointer2, flipX: true },
  { title: t('sidebar.providers'), name: 'providers', icon: Box },
  { title: t('sidebar.memory'), name: 'memory', icon: Database },
  { title: t('sidebar.webSearch'), name: 'web-search', icon: Globe },
  { title: t('sidebar.speech'), name: 'speech', icon: Volume2 },
  { title: t('sidebar.transcription'), name: 'transcription', icon: Captions },
]))

const integrationsNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.email'), name: 'email', icon: Mail },
  { title: t('sidebar.supermarket'), name: 'supermarket', icon: Store },
  { title: t('sidebar.usage'), name: 'usage', icon: ChartNoAxesColumn },
  { title: t('sidebar.people'), name: 'people', icon: Users, adminOnly: true },
]))

const accountNavItems = computed<NavItem[]>(() => filterItems([
  { title: t('sidebar.appearance'), name: 'appearance', icon: AppearanceIcon },
  { title: t('sidebar.profile'), name: 'profile', icon: CircleUserRound },
  { title: t('sidebar.about'), name: 'about', icon: Info },
]))
</script>
