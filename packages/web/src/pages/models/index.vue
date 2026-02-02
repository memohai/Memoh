<script setup lang="ts">
// import type { Payment } from '@/components/columns'
import { h, computed, ref, provide, watch, type ComputedRef, reactive, inject } from 'vue'
import CreateModel from '@/components/CreateModel/index.vue'
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
} from '@memoh/ui'
import { mdiMagnify } from '@mdi/js'
// import DataTable from '@/components/DataTable/index.vue'
import SvgIcon from '@jamescoyle/vue-icon'
import request from '@/utils/request'
import {type ModelList } from '@memoh/shared'
import AddProvider from '@/components/AddProvider/index.vue'
import {clientType } from '@memoh/shared'

const filterProvider = ref('')
const {data:providerData}=useQuery({
  key: ['provider'],
  query: () => request({
    url: `/providers?client_type=${filterProvider.value}`,

  }).then(fetchValue=>fetchValue.data)
})
const queryCache=useQueryCache()

watch(filterProvider, () => {
 
  queryCache.invalidateQueries({
    key: ['provider']
  })
}, {
  immediate:true
})

const curProvider = ref<Partial<ModelList>>()
const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.name === value
})
watch(providerData, () => {
  if (Array.isArray(providerData.value)&&providerData.value.length > 0) {
    curProvider.value= providerData.value[0]
  }
}, {
  immediate:true
})
provide('curProvider', curProvider)

const openStatus = reactive({
  provideOpen:false  
})

watch(providerData, () => {
  console.log(providerData.value)
})

</script>

<template>
  <div class="w-full  mx-auto">
    <div class="[&_td:last-child]:w-45 relative model-select">
      <SidebarProvider class="min-h-[initial]! flex **:data-[sidebar=sidebar]:bg-transparent">
        <Sidebar class="h-[calc(100vh-calc(var(--spacing)*16)-1px)]! relative top-0 ">
          <SidebarHeader>
            <InputGroup class="shadow-none">
              <InputGroupInput placeholder="搜索模型平台" />
              <InputGroupAddon align="inline-end">
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
              v-for="providerItem in providerData"
              :key="providerItem.name"
            >
              <SidebarMenuItem>
                <SidebarMenuButton
                  as-child
                  class="justify-start py-5! px-4"
                >
                  <Toggle
                    :class="`py-4 border border-transparent ${curProvider?.name===providerItem.name?'border-inherit':''}`"
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
        <section class="flex-1 h-[calc(100vh-calc(var(--spacing)*16)-1px)]! ">
          <ScrollArea class="max-h-full h-full">
            <model-setting />
          </ScrollArea>
        </section>
      </SidebarProvider>
    </div>   
  </div>
</template>