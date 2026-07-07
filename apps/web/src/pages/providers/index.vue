<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import {
  Button,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@memohai/ui'
import { getModels, getProviders } from '@memohai/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { Boxes, Box, ChevronRight, Plus, Search } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddProvider from '@/components/add-provider/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import { avatarInitials } from '@/composables/useAvatarInitials'
import SwapTransition from '@/components/settings/swap-transition.vue'
import PageShell from '@/components/page-shell/index.vue'
import ModelSetting from './model-setting.vue'

const { t } = useI18n()

const { data: providerData } = useQuery({
  key: () => ['providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: modelData } = useQuery({
  key: () => ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const curProvider = ref<ProvidersGetResponse>()

// 'detail' query key: mirrors detail-open into the URL so re-clicking Providers
// in the settings sidebar while a provider's detail is open actually navigates
// back to the list, instead of being dropped as a duplicate push (see
// useViewSwap.ts).
const { view, direction, openDetail, backToList } = useViewSwap('detail')
const searchQuery = ref('')
const addOpen = ref(false)

const providers = computed<ProvidersGetResponse[]>(() => {
  if (!Array.isArray(providerData.value)) return []
  return [...providerData.value].sort((a, b) => {
    const ae = a.enable !== false ? 1 : 0
    const be = b.enable !== false ? 1 : 0
    return be - ae
  })
})

const modelCountByProvider = computed(() => {
  const counts: Record<string, number> = {}
  for (const model of (modelData.value as ModelsGetResponse[] | undefined) ?? []) {
    const id = model.provider_id
    if (!id) continue
    counts[id] = (counts[id] ?? 0) + 1
  }
  return counts
})

// Always offer search once there's anything to filter — a hidden-then-appearing
// box read as inconsistent (some providers showed it, some didn't).
const showSearch = computed(() => providers.value.length > 0)

const filteredProviders = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter((p) => {
    const name = (p.name ?? '').toLowerCase()
    const url = providerSubtitle(p).toLowerCase()
    return name.includes(keyword) || url.includes(keyword)
  })
})

function providerSubtitle(provider: ProvidersGetResponse) {
  const baseUrl = (provider.config as Record<string, unknown> | undefined)?.base_url
  if (typeof baseUrl === 'string' && baseUrl) {
    return baseUrl.replace(/^https?:\/\//, '')
  }
  return provider.client_type ?? ''
}

function modelCount(id: string | undefined) {
  return id ? (modelCountByProvider.value[id] ?? 0) : 0
}

function openProvider(provider: ProvidersGetResponse) {
  curProvider.value = provider
  openDetail()
}

// Sync the open provider with refreshed data; if it was deleted while open,
// fall back to the gallery.
watch(providers, (list) => {
  const currentId = curProvider.value?.id
  if (!currentId) return
  const stillExists = list.find((p) => p.id === currentId)
  if (stillExists) {
    curProvider.value = stillExists
  } else if (view.value === 'detail') {
    backToList()
  }
})
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Gallery -->
    <PageShell
      v-if="view === 'list'"
      :title="t('sidebar.providers')"
    >
      <template #actions>
        <div
          v-if="showSearch"
          class="w-44 sm:w-56"
        >
          <InputGroup class="w-full">
            <InputGroupAddon align="inline-start">
              <Search class="size-3.5 text-muted-foreground" />
            </InputGroupAddon>
            <InputGroupInput
              v-model="searchQuery"
              :placeholder="t('provider.searchPlaceholder')"
            />
          </InputGroup>
        </div>
        <Button @click="addOpen = true">
          <Plus class="size-4" />
          {{ t('provider.addBtn') }}
        </Button>
      </template>

      <div
        v-if="providers.length > 0"
        class="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <BackendCard
          v-for="provider in filteredProviders"
          :key="provider.id"
          :name="provider.name ?? ''"
          :subtitle="providerSubtitle(provider)"
          :enabled="provider.enable !== false"
          @click="openProvider(provider)"
        >
          <template #leading>
            <span class="flex size-10 items-center justify-center rounded-full bg-muted">
              <ProviderIcon
                v-if="provider.icon"
                :icon="provider.icon"
                size="1.5em"
              />
              <span
                v-else
                class="text-xs font-medium text-muted-foreground"
              >
                {{ avatarInitials(provider.name, '?') }}
              </span>
            </span>
          </template>
          <template #trailing>
            <span
              v-if="modelCount(provider.id) > 0"
              class="flex shrink-0 items-center gap-1 text-xs text-muted-foreground"
            >
              <Boxes class="size-3.5" />
              {{ modelCount(provider.id) }}
            </span>
            <ChevronRight
              v-else
              class="size-4 shrink-0 text-muted-foreground/60"
            />
          </template>
        </BackendCard>
      </div>

      <Empty
        v-else
        class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-16"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Box />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ t('provider.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ t('provider.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('provider.addBtn') }}
          </Button>
        </EmptyContent>
      </Empty>

      <AddProvider
        v-model:open="addOpen"
        :providers="providers"
        hide-trigger
      />
    </PageShell>

    <!-- Detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('sidebar.providers')"
      @back="backToList()"
    >
      <ModelSetting
        v-if="curProvider?.id"
        v-model:provider="curProvider"
      />
    </DetailPane>
  </SwapTransition>
</template>
