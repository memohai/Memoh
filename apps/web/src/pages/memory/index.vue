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
  Button
} from '@memohai/ui'
import { getMemoryProviders } from '@memohai/sdk'
import type { MemoryprovidersGetResponse } from '@memohai/sdk'
import AddMemoryProvider from './components/add-memory-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import { Brain, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const curProvider = ref<MemoryprovidersGetResponse>()
provide('curMemoryProvider', curProvider)

const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  return providerData.value
})

watch(curFilterProvider, () => {
  if (curFilterProvider.value.length > 0) {
    curProvider.value = curFilterProvider.value[0]
  } else {
    curProvider.value = undefined
  }
}, { immediate: true })

const openStatus = reactive({ addOpen: false })
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu
        v-for="item in curFilterProvider"
        :key="item.id"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="`py-4 border border-transparent ${curProvider?.id === item.id ? 'border-inherit' : ''}`"
              :model-value="selectProvider(item.name).value"
              @update:model-value="(isSelect) => { if (isSelect) curProvider = item }"
            >
              <Brain
                class="mr-2 size-4 text-primary"
              />
              {{ item.name }}
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddMemoryProvider v-model:open="openStatus.addOpen" />
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
            <Brain />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('memory.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('memory.emptyDescription') }}</EmptyDescription>
        <EmptyContent>        
          <Button
            variant="outline"
            class="w-full"
            @click="openStatus.addOpen=true"
          >
            <Plus
              class="mr-2"
            />
            {{ $t('memory.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
