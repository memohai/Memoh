<script setup lang="ts">
// import type { Payment } from '@/components/columns'
import { computed, ref, provide, watch, reactive } from 'vue'
import modelSetting from './modelSetting.vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  ScrollArea,
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  InputGroup, InputGroupAddon, InputGroupInput,
  SidebarFooter,
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
import { mdiMagnify,mdiListBoxOutline } from '@mdi/js'
// import DataTable from '@/components/DataTable/index.vue'
import SvgIcon from '@jamescoyle/vue-icon'
import request from '@/utils/request'
import { type ProviderInfo } from '@memoh/shared'
import AddProvider from '@/components/AddProvider/index.vue'
import { clientType } from '@memoh/shared'

const filterProvider = ref('')
const { data: providerData } = useQuery({
  key: ['provider'],
  query: () => request({
    url: `/providers?client_type=${filterProvider.value}`,

  }).then(fetchValue => fetchValue.data)
})
const queryCache = useQueryCache()

watch(filterProvider, () => {

  queryCache.invalidateQueries({
    key: ['provider']
  })
}, {
  immediate: true
})

const curProvider = ref<Partial<ProviderInfo> & { id: string }>()
const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})

const searchProviderTxt = reactive({
  temp_value: '',
  value: ''
})

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) {
    return []
  }
  const searchReg = new RegExp([...searchProviderTxt.value].map(v => `\\u{${v.codePointAt(0)?.toString(16)}}`).join(''), 'u')
  return providerData.value.filter((provider: Partial<ProviderInfo> & { id: string }) => {
    return searchReg.test(provider.name as string)
  })
})

watch(curFilterProvider, () => {
  if (Array.isArray(curFilterProvider.value) && curFilterProvider.value.length > 0) {
    curProvider.value = curFilterProvider.value[0]
  } else {
    curProvider.value = {
      id:''
    }
  }
}, {
  immediate: true
})
provide('curProvider', curProvider)

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
                v-model="searchProviderTxt.temp_value"
                placeholder="搜索模型平台"
              />
              <InputGroupAddon
                align="inline-end"
                class="cursor-pointer"
                @click="() => {
                  searchProviderTxt.value = searchProviderTxt.temp_value
                }"
              >
                <svg-icon
                  type="mdi"
                  :path="mdiMagnify"
                  class="translate-icon"
                />
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
            <Select v-model:model-value="filterProvider">
              <SelectTrigger class="w-full">
                <SelectValue :placeholder="$t('prompt.select', { msg: 'Type' })" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem
                    v-for="type in clientType"
                    :key="type"
                    :value="type"
                  >
                    {{ type }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
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
                <svg-icon
                  type="mdi"
                  :path="mdiListBoxOutline"
                />
              </EmptyMedia>
            </EmptyHeader>
            <EmptyTitle>No Provider</EmptyTitle>
            <EmptyDescription>没有添加模型提供商,无法配置模型</EmptyDescription>
            <EmptyContent>
              <!-- <Button>Add data</Button> -->
              <AddProvider v-model:open="openStatus.provideOpen" />
            </EmptyContent>
          </Empty>
        </section>
      </SidebarProvider>
    </div>
  </div>
</template>