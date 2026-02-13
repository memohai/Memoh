<template>
  <aside class="[&_[data-state=collapsed]_:is(.title-container,.exist-btn)]:hidden">
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <button
          class="flex w-full min-w-0 items-center gap-2 px-3 py-2 text-left group-data-[state=collapsed]:w-8 group-data-[state=collapsed]:min-w-8 group-data-[state=collapsed]:max-w-8 group-data-[state=collapsed]:justify-center group-data-[state=collapsed]:p-2"
          @click="onLogoClick"
        >
          <img
            src="/logo.png"
            class="size-8 shrink-0 object-contain"
            alt="logo"
          >
          <span class="text-xl font-bold text-gray-500 dark:text-gray-400 group-data-[state=collapsed]:hidden">
            Memoh
          </span>
        </button>
      </SidebarHeader>

      <SidebarContent>
        <ChatListMenu :collapsible="true" />
        <SettingsListMenu :collapsible="true" />
      </SidebarContent>
      <SidebarFooter class="border-t p-2">
        <div class="flex items-center gap-2 min-w-0">
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-8 shrink-0 p-0"
            :title="isUserSettingsRoute ? $t('common.back') : $t('sidebar.settings')"
            @click="onUserSettingsAction"
          >
            <Avatar class="size-8">
              <AvatarImage
                v-if="userInfo.avatarUrl"
                :src="userInfo.avatarUrl"
                :alt="displayTitle"
              />
              <AvatarFallback class="text-xs">
                {{ avatarFallback }}
              </AvatarFallback>
            </Avatar>
          </Button>
          <span class="text-sm truncate min-w-0 flex-1 group-data-[state=collapsed]:hidden">
            {{ displayNameLabel }}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            class="shrink-0 group-data-[state=collapsed]:hidden"
            :title="isUserSettingsRoute ? $t('common.back') : $t('sidebar.settings')"
            @click="onUserSettingsAction"
          >
            <FontAwesomeIcon
              :icon="isUserSettingsRoute ? ['fas', 'arrow-left'] : ['fas', 'gear']"
              class="size-3.5"
            />
          </Button>
        </div>
      </SidebarFooter>
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
} from '@memoh/ui'
import { computed, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import ChatListMenu from './lists/chat-list-menu.vue'
import SettingsListMenu from './lists/settings-list-menu.vue'
import { useUserStore } from '@/store/user'

const router = useRouter()
const route = useRoute()

const { userInfo } = useUserStore()
const displayNameLabel = computed(() => userInfo.displayName || userInfo.username || userInfo.id || '-')
const displayTitle = computed(() => userInfo.displayName || userInfo.username || userInfo.id || 'User')
const avatarFallback = computed(() => displayTitle.value.slice(0, 2).toUpperCase() || 'U')
const collapsedHiddenClass = 'group-data-[state=collapsed]:hidden'
const isUserSettingsRoute = computed(() => route.name === 'settings-user')
const lastNonUserSettingsPath = ref('')

watch(
  () => route.fullPath,
  () => {
    if (route.name !== 'settings-user') {
      lastNonUserSettingsPath.value = route.fullPath
    }
  },
  { immediate: true },
)

function onLogoClick() {
  if (route.name === 'chat') {
    return
  }
  void router.push({ name: 'chat' }).catch(() => undefined)
}

function onUserSettingsAction() {
  if (!isUserSettingsRoute.value) {
    void router.push({ name: 'settings-user' }).catch(() => undefined)
    return
  }

  const fallbackPath = '/main/settings'
  const targetPath = lastNonUserSettingsPath.value.trim()
  if (targetPath && targetPath !== route.fullPath) {
    void router.push(targetPath).catch(() => undefined)
    return
  }
  void router.push(fallbackPath).catch(() => undefined)
}
</script>
