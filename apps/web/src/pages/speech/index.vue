<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQuery } from '@pinia/colada'
import {
  Button,
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
} from '@memohai/ui'
import { getTtsProviders } from '@memohai/sdk'
import type { TtsProviderResponse } from '@memohai/sdk'
import AddTtsProvider from './components/add-tts-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import { Volume2, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['tts-providers'],
  query: async () => {
    const { data } = await getTtsProviders({ throwOnError: true })
    return data
  },
})
const curProvider = ref<TtsProviderResponse>()
provide('curTtsProvider', curProvider)

const selectProvider = (name: string) => computed(() => {
  return curProvider.value?.name === name
})

const filteredProviders = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  return [...providerData.value].sort((a, b) => {
    const ae = a.enable !== false ? 1 : 0
    const be = b.enable !== false ? 1 : 0
    return be - ae
  })
})

watch(filteredProviders, (list) => {
  if (!list || list.length === 0) {
    curProvider.value = { id: '' }
    return
  }
  const currentId = curProvider.value?.id
  if (currentId) {
    const stillExists = list.find((p: TtsProviderResponse) => p.id === currentId)
    if (stillExists) {
      curProvider.value = stillExists
      return
    }
  }
  curProvider.value = list[0]
}, { immediate: true })

const openStatus = reactive({ addOpen: false })
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu
        v-for="item in filteredProviders"
        :key="item.id"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="['py-4 border', curProvider?.id === item.id ? 'border-border' : 'border-transparent']"
              :model-value="selectProvider(item.name ?? '').value"
              @update:model-value="(isSelect) => { if (isSelect) curProvider = item }"
            >
              <span class="relative shrink-0">
                <span class="flex size-7 items-center justify-center rounded-full bg-muted">
                  <Volume2
                    class="size-3.5 text-muted-foreground"
                  />
                </span>
                <span
                  v-if="item.enable !== false"
                  class="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full bg-green-500 ring-2 ring-background"
                />
              </span>
              <span class="truncate">{{ item.name }}</span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddTtsProvider v-model:open="openStatus.addOpen" />
    </template>

    <template #detail>
      <ScrollArea
        v-if="curProvider?.id"
        class="max-h-full h-full"
      >
        <ProviderSetting />
      </ScrollArea>
      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Volume2 />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('speech.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('speech.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <Plus
              class="mr-1"
            /> {{ $t('speech.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
