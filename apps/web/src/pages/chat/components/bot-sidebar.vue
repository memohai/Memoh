<template>
  <section :class="mountNode.id">
    <Teleport :to="mountNode.leftDefault">
      <SidebarProvider>
        <SidebarInset>
          <Sidebar class="relative! **:[[role=navigation]]:relative">
            <SidebarHeader
              class="h-12 flex flex-rows justify-center"
              @click="collpaseSider"
            >
              <section class="flex items-center gap-2">
                <FontAwesomeIcon :icon="['fas', 'bars']" />
                <h3 class="font-semibold text-sm">
                  {{ $t('sidebar.session') }}
                </h3>
              </section>
            </SidebarHeader>
            <Separator />
            <SidebarContent>
              <ScrollArea class="h-full px-2">
                <SidebarMenu class="my-4">
                  <InputGroup>
                    <InputGroupInput placeholder="Search..." />
                    <InputGroupAddon>
                      <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
                    </InputGroupAddon>
                  </InputGroup>
                </SidebarMenu>
                <h4 class="text-xs uppercase text-muted-foreground tracking-wide mb-2">
                  我的SESSION
                </h4>
                <SidebarMenu ref="session-item">
                  <SidebarMenuItem
                    v-for="bot in bots"
                    :key="bot.id"
                    class="relative hover:[&_svg]:visible"
                  >
                    <FontAwesomeIcon
                      :icon="['fas', 'grip-vertical']"
                      class="can-dragable  cursor-pointer absolute top-0 bottom-0 m-auto w-1.5! left-1! invisible z-100 text-[#C7C7C7]!"
                    />

                    <BotItem :bot="bot" />
                  </SidebarMenuItem>
                </SidebarMenu>
                <SidebarMenu>
                  <div
                    v-if="isLoading"
                    class="flex justify-center py-4"
                  >
                    <FontAwesomeIcon
                      :icon="['fas', 'spinner']"
                      class="size-4 animate-spin text-muted-foreground"
                    />
                  </div>
                  <div
                    v-if="!isLoading && bots.length === 0"
                    class="px-3 py-6 text-center text-sm text-muted-foreground"
                  >
                    {{ $t('bots.emptyTitle') }}
                  </div>
                </SidebarMenu>
              </ScrollArea>
            </SidebarContent>
            <SidebarFooter>
              <Button class="mb-4 justify-start gap-4">
                <FontAwesomeIcon :icon="['fas', 'plus']" />
                Session
              </Button>
            </SidebarFooter>
          </Sidebar>
        </SidebarInset>
      </SidebarProvider>
    </Teleport>
    <section class="hidden-clip-section" />
  </section>
</template>

<script setup lang="ts">
import { computed, useTemplateRef, watch } from 'vue'
// import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import Sortable from 'sortablejs'
import {
  SidebarMenu,
  SidebarMenuItem,
  SidebarHeader,
  SidebarProvider,
  SidebarContent,
  Sidebar,
  SidebarInset,
  Separator,
  SidebarFooter,
  Button,
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  ScrollArea
} from '@memohai/ui'
import BotItem from './bot-item.vue'
import useControlVisibleStatus from '@/utils/useControlVisibleStatus'
import { useSidebar } from '@memohai/ui'

const { toggleSidebar } = useSidebar()
const collpaseSider = () => {
  console.log('enter')
  toggleSidebar()
}

const sessionItem = useTemplateRef('session-item')

watch(sessionItem, () => {
  const el = sessionItem.value?.$el
  if (sessionItem.value?.$el) {
    new Sortable(el, {
      animation: 150,
      handle: '.can-dragable'
    })
  }
}, {
  immediate: true
})

console.log(Sortable)

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => botData.value?.items?.concat({
  'id': '991cd528-0c10-41a0-93e6-a6a7006a433cd',
  'owner_user_id': '9b7390f6-a336-4c76-bf58-df616942c9a6',
  'type': 'personal',
  'display_name': 'feafew',
  'is_active': true,
  'allow_guest': false,
  'status': 'loading',
  'check_state': 'ok',
  'check_issue_count': 0,
  'created_at': '2026-03-23T10:14:13.269928+08:00',
  'updated_at': '2026-03-23T10:14:13.435601+08:00'
}, {
  'id': '991cd528-0c10-41a0-93e6-a6a754006ac3cd',
  'owner_user_id': '9b7390f6-a336-4c76-bf58-df616942c9a6',
  'type': 'personal',
  'display_name': 'feafew',
  'is_active': true,
  'allow_guest': false,
  'status': 'error',
  'check_state': 'ok',
  'check_issue_count': 0,
  'created_at': '2026-03-23T10:14:13.269928+08:00',
  'updated_at': '2026-03-23T10:14:13.435601+08:00'
}, {
  'id': '991cd528-0c10-41a0fewf-93e6-a6a754006ac3cd',
  'owner_user_id': '9b7390f6-a336-4c76-bf58-df616942c9a6',
  'type': 'personal',
  'display_name': 'feafew',
  'is_active': true,
  'allow_guest': false,
  'status': 'no-setting',
  'check_state': 'ok',
  'check_issue_count': 0,
  'created_at': '2026-03-23T10:14:13.269928+08:00',
  'updated_at': '2026-03-23T10:14:13.435601+08:00'
}) ?? [])


const mountNode = useControlVisibleStatus()
</script>