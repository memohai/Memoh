<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Button,
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
} from '@memoh/ui'
import { getEmailProviders } from '@memoh/sdk'
import type { EmailProviderResponse } from '@memoh/sdk'
import AddEmailProvider from './components/add-email-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: providerData } = useQuery({
  key: () => ['email-providers'],
  query: async () => {
    const { data } = await getEmailProviders({ throwOnError: true })
    return data
  },
})
const queryCache = useQueryCache()

const curProvider = ref<EmailProviderResponse>()
provide('curEmailProvider', curProvider)

const selectProvider = (name: string) => computed(() => {
  return curProvider.value?.name === name
})

const searchText = ref('')
const searchInput = ref('')

const filteredProviders = computed(() => {
  if (!Array.isArray(providerData.value)) return []
  if (!searchText.value) return providerData.value
  const keyword = searchText.value.toLowerCase()
  return providerData.value.filter((p: EmailProviderResponse) =>
    (p.name ?? '').toLowerCase().includes(keyword),
  )
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
    <template #sidebar-header>
      <InputGroup class="shadow-none">
        <InputGroupInput
          v-model="searchInput"
          :placeholder="$t('emailProvider.searchPlaceholder')"
          aria-label="Search email providers"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search"
            @click="searchText = searchInput"
          >
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

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
            <FontAwesomeIcon :icon="['fas', 'envelope']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('emailProvider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('emailProvider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="mr-1"
            /> {{ $t('emailProvider.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
