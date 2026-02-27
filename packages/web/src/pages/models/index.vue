<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import modelSetting from './model-setting.vue'
import { useQueryCache } from '@pinia/colada'
import {
  ScrollArea,
  InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput,
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
} from '@memoh/ui'
import { getProviders } from '@memoh/sdk'
import type { ProvidersGetResponse } from '@memoh/sdk'
import AddProvider from '@/components/add-provider/index.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import { useQuery } from '@pinia/colada'

const { data: providerData } = useQuery({
  key: () => ['providers'],
  query: async () => {
    const { data } = await getProviders({
      throwOnError: true,
    })
    return data
  },
})
const queryCache = useQueryCache()

const curProvider = ref<ProvidersGetResponse>()
provide('curProvider', curProvider)

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
  return providerData.value.filter((provider: ProvidersGetResponse) => {
    return (provider.name ?? '').toLowerCase().includes(keyword)
  })
})

watch(curFilterProvider, () => {
  if (curFilterProvider.value.length > 0) {
    curProvider.value = curFilterProvider.value[0]
  } else {
    curProvider.value = { id: '' }
  }
}, {
  immediate: true
})

const openStatus = reactive({
  provideOpen: false
})

</script>

<template>
  <MasterDetailSidebarLayout class="[&_td:last-child]:w-45">
    <template #sidebar-header>
      <InputGroup class="shadow-none">
        <InputGroupInput
          v-model="searchInput"
          :placeholder="$t('models.searchPlaceholder')"
          aria-label="Search models"
        />
        <InputGroupAddon
          align="inline-end"
        >
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search models"
            @click="searchText = searchInput"
          >
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

    <template #sidebar-content>
      <SidebarMenu
        v-for="providerItem in curFilterProvider"
        :key="providerItem.name"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="['py-4 border', curProvider?.name === providerItem.name ? 'border-border' : 'border-transparent']"
              :model-value="selectProvider(providerItem.name ?? '').value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curProvider = providerItem
                }
              }"
            >
              {{ providerItem.name }}
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddProvider v-model:open="openStatus.provideOpen" />
    </template>

    <template #detail>
      <ScrollArea
        v-if="curProvider?.id"
        class="max-h-full h-full"
      >
        <model-setting />
      </ScrollArea>
      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FontAwesomeIcon :icon="['far', 'rectangle-list']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('provider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('provider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <AddProvider v-model:open="openStatus.provideOpen" />
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>    
