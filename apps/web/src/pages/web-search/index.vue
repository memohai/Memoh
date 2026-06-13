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
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Button,
  Badge,
} from '@memohai/ui'
import { getFetchProviders, getSearchProviders } from '@memohai/sdk'
import type { FetchprovidersGetResponse, SearchprovidersGetResponse } from '@memohai/sdk'
import AddWebProvider from './components/add-web-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import FetchProviderSetting from './components/fetch-provider-setting.vue'
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

const { data: fetchProviderData } = useQuery({
  key: () => ['fetch-providers'],
  query: async () => {
    const { data } = await getFetchProviders({
      throwOnError: true,
    })
    return data
  },
})

type ProviderKind = 'search' | 'fetch'

const curProvider = ref<SearchprovidersGetResponse>()
const curFetchProvider = ref<FetchprovidersGetResponse>()
provide('curSearchProvider', curProvider)
provide('curFetchProvider', curFetchProvider)

const selectedKind = ref<ProviderKind>('search')

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

const curFetchFilterProvider = computed(() => {
  if (!Array.isArray(fetchProviderData.value)) {
    return []
  }
  return [...fetchProviderData.value].sort((a, b) => {
    const an = a.provider === 'native' ? 1 : 0
    const bn = b.provider === 'native' ? 1 : 0
    if (an !== bn) return bn - an
    const ae = a.enable !== false ? 1 : 0
    const be = b.enable !== false ? 1 : 0
    return be - ae
  })
})

watch(curFilterProvider, (providers) => {
  if (selectedKind.value !== 'search') {
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
  curProvider.value = providers[0] ?? { id: '' }
}, {
  immediate: true,
})

watch(curFetchFilterProvider, (providers) => {
  if (selectedKind.value !== 'fetch') {
    return
  }
  const currentId = curFetchProvider.value?.id
  if (currentId) {
    const stillExists = providers.find((p) => p.id === currentId)
    if (stillExists) {
      curFetchProvider.value = stillExists
      return
    }
  }
  curFetchProvider.value = providers[0] ?? { id: '' }
}, {
  immediate: true,
})

const addOpen = ref(false)

function selectSearchProvider(item: SearchprovidersGetResponse) {
  selectedKind.value = 'search'
  curProvider.value = item
}

function selectFetchProvider(item: FetchprovidersGetResponse) {
  selectedKind.value = 'fetch'
  curFetchProvider.value = item
}
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <div class="px-3 pb-2 pt-3 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        {{ $t('webSearch.searchProviders') }}
      </div>
      <SidebarMenu
        v-for="item in curFilterProvider"
        :key="`search-${item.id}`"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="[
                'py-4 border',
                selectedKind === 'search' && curProvider?.id === item.id ? 'border-border' : 'border-transparent',
              ]"
              :model-value="selectedKind === 'search' && curProvider?.id === item.id"
              @update:model-value="(isSelect) => {
                if (isSelect) selectSearchProvider(item)
              }"
            >
              <span class="relative shrink-0">
                <SearchProviderLogo
                  :provider="item.provider || ''"
                  size="sm"
                />
                <Badge
                  v-if="item.enable !== false"
                  class="absolute -bottom-0.5 -right-0.5 size-2.5 p-0 rounded-full ring-2 ring-background"
                  variant="success"
                />
              </span>
              <span class="truncate">{{ item.name }}</span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>

      <div class="px-3 pb-2 pt-5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        {{ $t('webSearch.fetchProviders') }}
      </div>
      <SidebarMenu
        v-for="item in curFetchFilterProvider"
        :key="`fetch-${item.id}`"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="[
                'py-4 border',
                selectedKind === 'fetch' && curFetchProvider?.id === item.id ? 'border-border' : 'border-transparent',
              ]"
              :model-value="selectedKind === 'fetch' && curFetchProvider?.id === item.id"
              @update:model-value="(isSelect) => {
                if (isSelect) selectFetchProvider(item)
              }"
            >
              <span class="relative shrink-0">
                <SearchProviderLogo
                  :provider="item.provider || ''"
                  size="sm"
                />
                <Badge
                  v-if="item.enable !== false"
                  class="absolute -bottom-0.5 -right-0.5 size-2.5 p-0 rounded-full ring-2 ring-background"
                  variant="success"
                />
              </span>
              <span class="truncate">{{ item.name }}</span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddWebProvider v-model:open="addOpen" />
    </template>

    <template #detail>
      <ScrollArea
        v-if="selectedKind === 'search' && curProvider?.id"
        class="max-h-full h-full"
      >
        <ProviderSetting />
      </ScrollArea>
      <ScrollArea
        v-else-if="selectedKind === 'fetch' && curFetchProvider?.id"
        class="max-h-full h-full"
      >
        <FetchProviderSetting />
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
            @click="addOpen = true"
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
