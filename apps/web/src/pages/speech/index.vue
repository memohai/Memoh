<script setup lang="ts">
import { computed, ref, provide, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import {
  ScrollArea,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Toggle,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Button,
} from '@memohai/ui'
import { getSpeechProviders } from '@memohai/sdk'
import type { TtsSpeechProviderResponse } from '@memohai/sdk'
import ProviderSetting from './components/provider-setting.vue'
import AddSpeechProvider from './components/add-speech-provider.vue'
import { Volume2, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['speech-providers'],
  query: async () => {
    const { data } = await getSpeechProviders({ throwOnError: true })
    return data
  },
})
const curProvider = ref<TtsSpeechProviderResponse>()
const showAddProvider = ref(false)
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
    const stillExists = list.find((p: TtsSpeechProviderResponse) => p.id === currentId)
    if (stillExists) {
      curProvider.value = stillExists
      return
    }
  }
  curProvider.value = list[0]
}, { immediate: true })

</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <AddSpeechProvider v-model:open="showAddProvider" />
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
        <Button
          variant="outline"
          class="mt-2"
          @click="showAddProvider = true"
        >
          <Plus class="mr-1" />
          {{ $t('speech.add') }}
        </Button>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
