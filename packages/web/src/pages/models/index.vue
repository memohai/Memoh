<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import modelSetting from './model-setting.vue'
import { useQueryCache } from '@pinia/colada'
import {
  ScrollArea,
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput,
  SidebarFooter,
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
    return (provider.name as string).toLowerCase().includes(keyword)
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
  <div class="w-full  mx-auto">
    <div class="[&_td:last-child]:w-45 model-select">
      <SidebarProvider
        :open="true"
        class="min-h-[initial]! flex **:data-[sidebar=sidebar]:bg-transparent absolute inset-0"
      >
        <Sidebar class="h-full relative top-0 ">
          <SidebarHeader>
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
          </SidebarHeader>
          <SidebarContent class="px-2 scrollbar-none">
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
                    :class="`py-4 border border-transparent ${curProvider?.name === providerItem.name ? 'border-inherit' : ''}`"
                    :model-value="selectProvider(providerItem.name as string).value"
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
          </SidebarContent>
          <SidebarFooter>
            <AddProvider v-model:open="openStatus.provideOpen" />
          </SidebarFooter>
        </Sidebar>
        <section class="flex-1 h-full ">
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
        </section>
      </SidebarProvider>
    </div>
  </div>
</template>    
