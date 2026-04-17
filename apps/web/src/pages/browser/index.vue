<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQuery } from '@pinia/colada'
import {
  ScrollArea,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Toggle,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Button,
} from '@memohai/ui'
import { getBrowserContexts } from '@memohai/sdk'
import type { BrowsercontextsBrowserContext } from '@memohai/sdk'
import AddBrowserContext from './components/add-browser-context.vue'
import ContextSetting from './components/context-setting.vue'
import { AppWindow, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: contextData } = useQuery({
  key: () => ['browser-contexts'],
  query: async () => {
    const { data } = await getBrowserContexts({
      throwOnError: true,
    })
    return data
  },
})

const curContext = ref<BrowsercontextsBrowserContext>()
provide('curBrowserContext', curContext)

const selectContext = (id: string) => computed(() => {
  return curContext.value?.id === id
})

const filteredContexts = computed(() => {
  if (!Array.isArray(contextData.value)) return []
  return contextData.value
})

watch(filteredContexts, () => {
  if (filteredContexts.value.length > 0) {
    curContext.value = filteredContexts.value[0]
  } else {
    curContext.value = undefined
  }
}, { immediate: true })

const openStatus = reactive({
  addOpen: false,
})
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu
        v-for="item in filteredContexts"
        :key="item.id"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="`py-4 border border-transparent ${curContext?.id === item.id ? 'border-inherit' : ''}`"
              :model-value="selectContext(item.id as string).value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curContext = item
                }
              }"
            >
              <AppWindow
                class="mr-2"
              />
              <span>
                {{ item.name }}
              </span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddBrowserContext v-model:open="openStatus.addOpen" />
    </template>

    <template #detail>
      <ScrollArea
        v-if="curContext?.id"
        class="max-h-full h-full"
      >
        <ContextSetting />
      </ScrollArea>
      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <AppWindow />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('browser.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('browser.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <Plus
              class="mr-1"
            /> {{ $t('browser.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
