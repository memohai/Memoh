<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQueryCache, useQuery } from '@pinia/colada'
import {
  ScrollArea,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput,
  Toggle,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@memoh/ui'
import { getSearchProviders } from '@memoh/sdk'
import type { SearchprovidersGetResponse } from '@memoh/sdk'
import AddSearchProvider from './components/add-search-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const PROVIDER_TYPES = ['brave', 'bing', 'google', 'tavily', 'sogou', 'serper', 'searxng', 'jina', 'exa', 'bocha', 'duckduckgo', 'yandex'] as const

const filterProvider = ref('')
const { data: providerData } = useQuery({
  key: () => ['search-providers', filterProvider.value],
  query: async () => {
    const { data } = await getSearchProviders({
      query: filterProvider.value ? { provider: filterProvider.value } : undefined,
      throwOnError: true,
    })
    return data
  },
})
const queryCache = useQueryCache()

watch(filterProvider, () => {
  queryCache.invalidateQueries({ key: ['search-providers'] })
}, { immediate: true })

const curProvider = ref<SearchprovidersGetResponse>()
provide('curSearchProvider', curProvider)

const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})

const searchText = ref('')
const searchInput = ref('')

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) {
    return []
  }
  if (!searchText.value) {
    return providerData.value
  }
  const keyword = searchText.value.toLowerCase()
  return providerData.value.filter((p: SearchprovidersGetResponse) => {
    return (p.name ?? '').toLowerCase().includes(keyword)
  })
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
    <template #sidebar-header>
      <InputGroup class="shadow-none">
        <InputGroupInput
          v-model="searchInput"
          :placeholder="$t('searchProvider.searchPlaceholder')"
          aria-label="Search providers"
        />
        <InputGroupAddon
          align="inline-end"
        >
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search providers"
            @click="searchText = searchInput"
          >
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

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
      <Select v-model:model-value="filterProvider">
        <SelectTrigger
          class="w-full"
          :aria-label="$t('searchProvider.provider')"
        >
          <SelectValue :placeholder="$t('common.typePlaceholder')" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem
              v-for="type in PROVIDER_TYPES"
              :key="type"
              :value="type"
            >
              {{ $t(`searchProvider.providerNames.${type}`, type) }}
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
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
          <AddSearchProvider v-model:open="openStatus.addOpen" />
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
