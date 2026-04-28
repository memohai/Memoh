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
  Badge,
} from '@memohai/ui'
import { getSearchProviders } from '@memohai/sdk'
import type { SearchprovidersGetResponse } from '@memohai/sdk'
import AddSearchProvider from './components/add-search-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import { Globe, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({
      throwOnError: true,
    })
    return data
  },
})

const curProvider = ref<SearchprovidersGetResponse>()
provide('curSearchProvider', curProvider)

const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) {
    return []
  }
  return [...providerData.value].sort((a, b) => {
    const ae = a.enable !== false ? 1 : 0
    const be = b.enable !== false ? 1 : 0
    return be - ae
  })
})

watch(curFilterProvider, (providers) => {
  if (providers.length === 0) {
    curProvider.value = { id: '' }
    return
  }
  const currentId = curProvider.value?.id
  if (currentId) {
    const stillExists = providers.find((p) => p.id === currentId)
    if (stillExists) {
      curProvider.value = stillExists
      return
    }
  }
  curProvider.value = providers[0]
}, {
  immediate: true,
})

const openStatus = reactive({
  addOpen: false,
})
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu
        v-for="item in curFilterProvider"
        :key="item.name"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="[
                'py-4 border',
                curProvider?.id === item.id ? 'border-border' : 'border-transparent',
              ]"
              :model-value="selectProvider(item.name as string).value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curProvider = item
                }
              }"
            >
              <span class="relative shrink-0">
                <SearchProviderLogo
                  :provider="item.provider || ''"
                  size="sm"
                />
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

    <template #sidebar-footer>
      <AddSearchProvider v-model:open="openStatus.addOpen" />
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
            <Globe />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('webSearch.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('webSearch.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button            
            variant="outline"
            @click="openStatus.addOpen=true"
          >
            <Plus
              class="mr-1"
            /> {{ $t('webSearch.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
