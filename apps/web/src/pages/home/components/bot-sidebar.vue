<template>
  <section :class="mountNode.id">
    <Teleport :to="mountNode.leftDefault">
      <SidebarProvider
        :open="sidebarOpen"
      >
        <SidebarInset>
          <Sidebar
            class="absolute! "
            collapsible="icon"
          >
            <SidebarHeader
              class="h-12 flex flex-rows justify-center"            
            >
              <section class="flex items-center gap-2">
                <img
                  src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTgiIGhlaWdodD0iMTgiIHZpZXdCb3g9IjAgMCAxOCAxOCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTEuNSA5QzEuNSA2LjE4NzcgMS41IDQuNzgxNTUgMi4yMTYxOSAzLjc5NTgxQzIuNDQ3NDggMy40Nzc0NSAyLjcyNzQ1IDMuMTk3NDggMy4wNDU4MSAyLjk2NjE5QzQuMDMxNTUgMi4yNSA1LjQzNzcgMi4yNSA4LjI1IDIuMjVIOS43NUMxMi41NjIzIDIuMjUgMTMuOTY4NCAyLjI1IDE0Ljk1NDIgMi45NjYxOUMxNS4yNzI2IDMuMTk3NDggMTUuNTUyNSAzLjQ3NzQ1IDE1Ljc4MzggMy43OTU4MUMxNi41IDQuNzgxNTUgMTYuNSA2LjE4NzcgMTYuNSA5QzE2LjUgMTEuODEyMyAxNi41IDEzLjIxODQgMTUuNzgzOCAxNC4yMDQyQzE1LjU1MjUgMTQuNTIyNSAxNS4yNzI2IDE0LjgwMjUgMTQuOTU0MiAxNS4wMzM4QzEzLjk2ODQgMTUuNzUgMTIuNTYyMyAxNS43NSA5Ljc1IDE1Ljc1SDguMjVDNS40Mzc3IDE1Ljc1IDQuMDMxNTUgMTUuNzUgMy4wNDU4MSAxNS4wMzM4QzIuNzI3NDUgMTQuODAyNSAyLjQ0NzQ4IDE0LjUyMjUgMi4yMTYxOSAxNC4yMDQyQzEuNSAxMy4yMTg0IDEuNSAxMS44MTIzIDEuNSA5WiIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCIvPgo8cGF0aCBkPSJNNy4xMjUgMi42MjVMNy4xMjUgMTUuMzc1IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0zLjc1IDUuMjVDMy43NSA1LjI1IDQuNDM1NjYgNS4yNSA0Ljg3NSA1LjI1IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTMuNzUgOC4yNUg0Ljg3NSIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xMi43NSA3LjVMMTEuODMwMSA4LjI5Mjg5QzExLjQ0MzQgOC42MjYyMyAxMS4yNSA4Ljc5Mjg5IDExLjI1IDlDMTEuMjUgOS4yMDcxMSAxMS40NDM0IDkuMzczNzcgMTEuODMwMSA5LjcwNzExTDEyLjc1IDEwLjUiIHN0cm9rZT0iIzBBMEEwQSIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCIvPgo8L3N2Zz4K"
                  alt="折叠"
                  class="group-data-[collapsible=icon]:m-auto"
                  @click="sidebarOpen = !sidebarOpen"
                >

                <h3 class="font-medium text-sm group-data-[collapsible=icon]:hidden">
                  {{ $t('sidebar.session') }}
                </h3>
              </section>
            </SidebarHeader>
            <Separator />
            <section
              class="not-group-data-[collapsible=icon]:hidden flex justify-center"
              style="writing-mode:sideways-rl"
            >
              <span class="flex-none w-[1em] mr-2.5 mt-4 text-muted-foreground text-sm">
                {{ $t('sidebar.session') }}
              </span>
            </section>
            <SidebarContent>
              <SidebarGroup class="group-data-[collapsible=icon]:invisible">
                <SidebarGroupContent>
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
                      <!-- <SidebarMenuButton
                        tooltip="afwef"
                        class="py-5 text-muted-foreground relative before:absolute before:w-0.5! before:top-2 before:bottom-2 data-[active=true]:before:bg-[#8B56E3] hover:before:bg-[#8B56E3] before:left-0.5!"
                      >
                        <FontAwesomeIcon :icon="['fas', 'grip-vertical']" />
                        <span>fwefwef</span>
                      </SidebarMenuButton> -->
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
                      class="px-3 py-6 text-center text-xs text-muted-foreground"
                    >
                      {{ $t('bots.emptyTitle') }}
                    </div>
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
            <SidebarFooter class="group-data-[collapsible=icon]:invisible">
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
import { computed, useTemplateRef, watch,ref } from 'vue'
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
  SidebarGroup,
  SidebarGroupContent
} from '@memohai/ui'
import BotItem from './bot-item.vue'
import useControlVisibleStatus from '@/utils/useControlVisibleStatus'
const sidebarOpen = ref(true)


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
} as BotsBot, {
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
} as BotsBot, {
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
} as BotsBot) ?? [])


const mountNode = useControlVisibleStatus()
</script>