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
  Badge,
} from '@memohai/ui'
import { getTranscriptionProviders } from '@memohai/sdk'
import type { AudioSpeechProviderResponse } from '@memohai/sdk'
import ProviderSetting from './provider-setting.vue'
import { AudioLines } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

const { data: providerData } = useQuery({
  key: () => ['transcription-providers'],
  query: async () => {
    const { data } = await getTranscriptionProviders({ throwOnError: true })
    return (data ?? []) as AudioSpeechProviderResponse[]
  },
})
const curProvider = ref<AudioSpeechProviderResponse>()
provide('curTranscriptionProvider', curProvider)

const selectProvider = (name: string) => computed(() => curProvider.value?.name === name)

const filteredProviders = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  return [...providerData.value].sort((a, b) => Number(b.enable !== false) - Number(a.enable !== false))
})

watch(filteredProviders, (list) => {
  if (!list || list.length === 0) {
    curProvider.value = { id: '' }
    return
  }
  const currentId = curProvider.value?.id
  if (currentId) {
    const stillExists = list.find(p => p.id === currentId)
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
                  <ProviderIcon
                    v-if="item.icon"
                    :icon="item.icon"
                    size="1.25em"
                  />
                  <span
                    v-else
                    class="text-xs font-medium text-muted-foreground"
                  >
                    {{ getInitials(item.name) }}
                  </span>
                </span>
                <Badge
                  v-if="item.enable !== false"
                  class="absolute bottom-[2px] right-[2px] p-0.5 rounded-full"
                  variant="destructive"
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
            <AudioLines />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('transcription.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('transcription.emptyDescription') }}</EmptyDescription>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
