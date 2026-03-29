<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQuery} from '@pinia/colada'
import {
  Button,
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
} from '@memohai/ui'
import { getEmailProviders } from '@memohai/sdk'
import type { EmailProviderResponse } from '@memohai/sdk'
import AddEmailProvider from './components/add-email-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import { Mail, Plus } from 'lucide-vue-next'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['email-providers'],
  query: async () => {
    const { data } = await getEmailProviders({ throwOnError: true })
    return data
  },
})
const curProvider = ref<EmailProviderResponse>()
provide('curEmailProvider', curProvider)

const selectProvider = (name: string) => computed(() => {
  return curProvider.value?.name === name
})

const filteredProviders = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  return providerData.value
})

watch(filteredProviders, (list) => {
  if (!list || list.length === 0) {
    curProvider.value = { id: '' }
    return
  }
  const currentId = curProvider.value?.id
  if (currentId) {
    const stillExists = list.find((p: EmailProviderResponse) => p.id === currentId)
    if (stillExists) {
      curProvider.value = stillExists
      return
    }
  }
  curProvider.value = list[0]
}, { immediate: true })

const openStatus = reactive({ addOpen: false })
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu
        v-for="item in filteredProviders"
        :key="item.id"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="['py-4 border', curProvider?.id === item.id ? 'border-border' : 'border-transparent']"
              :model-value="selectProvider(item.name ?? '').value"
              @update:model-value="(isSelect) => { if (isSelect) curProvider = item }"
            >
              {{ item.name }}
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddEmailProvider v-model:open="openStatus.addOpen" />
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
            <Mail />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('email.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('email.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <Plus
              class="mr-1"
            /> {{ $t('email.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
