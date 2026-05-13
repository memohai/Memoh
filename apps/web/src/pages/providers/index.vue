<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import modelSetting from './model-setting.vue'
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
import { getProviders } from '@memohai/sdk'
import type { ProvidersGetResponse } from '@memohai/sdk'
import AddProvider from '@/components/add-provider/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import { List } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

const { data: providerData } = useQuery({
  key: () => ['providers'],
  query: async () => {
    const { data } = await getProviders({
      throwOnError: true,
    })
    return data
  },
})

const curProvider = ref<ProvidersGetResponse>()
provide('curProvider', curProvider)

const selectProvider = (value: string) => computed(() => {
  return curProvider.value?.id === value
})

const curFilterProvider = computed(() => {
  if (!Array.isArray(providerData.value)) {
    return []
  }
  let list = providerData.value as ProvidersGetResponse[]
  return [...list].sort((a, b) => {
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
  immediate: true
})

const openStatus = reactive({
  provideOpen: false
})

</script>

<template>
  <MasterDetailSidebarLayout class="[&_td:last-child]:w-45">
    <template
      #sidebar-content
    >
      <SidebarMenu
        v-for="providerItem in curFilterProvider"
        :key="providerItem.name"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            :is-active="selectProvider(providerItem.id ?? '').value"
            class="justify-start py-0! px-0 h-11 before:hidden"
          >
            <Toggle
              class="w-full justify-start h-11 px-3 border-0 bg-transparent! transition-colors gap-3"
              :model-value="selectProvider(providerItem.id ?? '').value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curProvider = providerItem
                }
              }"
            >
              <span class="relative shrink-0">
                <span class="flex size-6 items-center justify-center rounded-full bg-muted">
                  <ProviderIcon
                    v-if="providerItem.icon"
                    :icon="providerItem.icon"
                    size="1em"
                  />
                  <span
                    v-else
                    class="text-[10px] font-medium text-muted-foreground"
                  >
                    {{ getInitials(providerItem.name) }}
                  </span>
                </span>
                <Badge
                  v-if="providerItem.enable !== false"
                  class="absolute -bottom-0.5 -right-0.5 size-2.5 p-0 rounded-full ring-2 ring-background"
                  variant="success"
                />
              </span>
              <span class="truncate text-xs font-medium">{{ providerItem.name }}</span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddProvider
        v-model:open="openStatus.provideOpen"
      />
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
            <List />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('provider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('provider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.provideOpen=true"
          >
            {{ $t('provider.addBtn') }}
          </Button>          
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>    
