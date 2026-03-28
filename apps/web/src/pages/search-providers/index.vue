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
import { getSearchProviders } from '@memohai/sdk'
import type { SearchprovidersGetResponse } from '@memohai/sdk'
import AddSearchProvider from './components/add-search-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
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
  return providerData.value
})

watch(curFilterProvider, () => {
  if (curFilterProvider.value.length > 0) {
    curProvider.value = curFilterProvider.value[0]
  } else {
    curProvider.value = { id: '' }
  }
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
              :class="`py-4 border border-transparent ${curProvider?.name === item.name ? 'border-inherit' : ''}`"
              :model-value="selectProvider(item.name as string).value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curProvider = item
                }
              }"
            >
              <SearchProviderLogo
                :provider="item.provider || ''"
                size="sm"
                class="mr-2"
              />
              {{ item.name }}
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
            <FontAwesomeIcon :icon="['fas', 'globe']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('searchProvider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('searchProvider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button            
            variant="outline"
            @click="openStatus.addOpen=true"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="mr-1"
            /> {{ $t('searchProvider.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
