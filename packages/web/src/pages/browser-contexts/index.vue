<script setup lang="ts">
import { computed, ref, provide, watch, reactive } from 'vue'
import { useQuery } from '@pinia/colada'
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
  Button,
} from '@memoh/ui'
import { getBrowserContexts } from '@memoh/sdk'
import type { BrowsercontextsBrowserContext } from '@memoh/sdk'
import AddBrowserContext from './components/add-browser-context.vue'
import ContextSetting from './components/context-setting.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const { data: contextData } = useQuery({
  key: () => ['browser-contexts'],
  query: async () => {
    const { data } = await getBrowserContexts({
      throwOnError: true,
    })
    return data
  },
})

const curContext = ref<BrowsercontextsBrowserContext>()
provide('curBrowserContext', curContext)

const selectContext = (id: string) => computed(() => {
  return curContext.value?.id === id
})

const searchText = ref('')
const searchInput = ref('')

const filteredContexts = computed(() => {
  if (!Array.isArray(contextData.value)) return []
  if (!searchText.value) return contextData.value
  const keyword = searchText.value.toLowerCase()
  return contextData.value.filter((c: BrowsercontextsBrowserContext) => {
    return (c.name ?? '').toLowerCase().includes(keyword)
  })
})

watch(filteredContexts, () => {
  if (filteredContexts.value.length > 0) {
    curContext.value = filteredContexts.value[0]
  } else {
    curContext.value = undefined
  }
}, { immediate: true })

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
          :placeholder="$t('browserContext.searchPlaceholder')"
          aria-label="Search browser contexts"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            type="button"
            size="icon-xs"
            aria-label="Search browser contexts"
            @click="searchText = searchInput"
          >
            <FontAwesomeIcon :icon="['fas', 'magnifying-glass']" />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </template>

    <template #sidebar-content>
      <SidebarMenu
        v-for="item in filteredContexts"
        :key="item.id"
      >
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            class="justify-start py-5! px-4"
          >
            <Toggle
              :class="`py-4 border border-transparent ${curContext?.id === item.id ? 'border-inherit' : ''}`"
              :model-value="selectContext(item.id as string).value"
              @update:model-value="(isSelect) => {
                if (isSelect) {
                  curContext = item
                }
              }"
            >
              <FontAwesomeIcon
                :icon="['fas', 'window-maximize']"
                class="mr-2"
              />
              {{ item.name }}
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #sidebar-footer>
      <AddBrowserContext v-model:open="openStatus.addOpen" />
    </template>

    <template #detail>
      <ScrollArea
        v-if="curContext?.id"
        class="max-h-full h-full"
      >
        <ContextSetting />
      </ScrollArea>
      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FontAwesomeIcon :icon="['fas', 'window-maximize']" />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t('browserContext.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ $t('browserContext.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="mr-1"
            /> {{ $t('browserContext.add') }}
          </Button>
        </EmptyContent>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
