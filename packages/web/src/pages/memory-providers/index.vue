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
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Button
} from '@memoh/ui'
import { getMemoryProviders } from '@memoh/sdk'
import type { MemoryprovidersGetResponse } from '@memoh/sdk'
import AddMemoryProvider from './components/add-memory-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})
const queryCache = useQueryCache()

const curProvider = ref<MemoryprovidersGetResponse>()
provide('curMemoryProvider', curProvider)

const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})

const searchText = ref('')
const searchInput = ref('')

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  if (!searchText.value) return providerData.value
  const keyword = searchText.value.toLowerCase()
  return providerData.value.filter((p) => p.name.toLowerCase().includes(keyword))
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
    <template #sidebar-header>
      <InputGroup class="shadow-none">
        <InputGroupInput
          v-model="searchInput"
          :placeholder="$t('memoryProvider.searchPlaceholder')"
          aria-label="Search memory providers"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search memory providers"
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
              <FontAwesomeIcon
                :icon="['fas', 'brain']"
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
            <FontAwesomeIcon :icon="['fas', 'brain']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('memoryProvider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('memoryProvider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>        
          <Button
            variant="outline"
            class="w-full"
            @click="openStatus.addOpen=true"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="mr-2"
            />
            {{ $t('memoryProvider.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
