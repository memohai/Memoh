<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Button } from '@memohai/ui'
import { getFetchProviders, getSearchProviders } from '@memohai/sdk'
import type { FetchprovidersGetResponse, SearchprovidersGetResponse } from '@memohai/sdk'
import { Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddFetchProvider from './components/add-fetch-provider.vue'
import AddSearchProvider from './components/add-search-provider.vue'
import FetchProviderSetting from './components/fetch-provider-setting.vue'
import ProviderSetting from './components/provider-setting.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: providerData } = useQuery({
  key: () => ['search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({ throwOnError: true })
    return data
  },
})

const { data: fetchProviderData } = useQuery({
  key: () => ['fetch-providers'],
  query: async () => {
    const { data } = await getFetchProviders({ throwOnError: true })
    return data
  },
})

const curProvider = ref<SearchprovidersGetResponse>()
const curFetchProvider = ref<FetchprovidersGetResponse>()
provide('curSearchProvider', curProvider)
provide('curFetchProvider', curFetchProvider)

const { view, direction, openDetail, backToList } = useViewSwap()
const detailKind = ref<'search' | 'fetch'>('search')
const openStatus = reactive({
  addSearchOpen: false,
  addFetchOpen: false,
})

function sortByEnabled<T extends { enable?: boolean }>(list: T[]) {
  return [...list].sort((a, b) => Number(b.enable !== false) - Number(a.enable !== false))
}

const providers = computed<SearchprovidersGetResponse[]>(() =>
  Array.isArray(providerData.value) ? sortByEnabled(providerData.value) : [],
)

const fetchProviders = computed<FetchprovidersGetResponse[]>(() => {
  if (!Array.isArray(fetchProviderData.value)) return []
  return [...fetchProviderData.value].sort((a, b) => {
    if (a.provider === 'native') return -1
    if (b.provider === 'native') return 1
    return Number(b.enable !== false) - Number(a.enable !== false)
  })
})

function openProvider(provider: SearchprovidersGetResponse) {
  curProvider.value = provider
  detailKind.value = 'search'
  openDetail()
}

function openFetchProvider(provider: FetchprovidersGetResponse) {
  curFetchProvider.value = provider
  detailKind.value = 'fetch'
  openDetail()
}

watch(() => openStatus.addSearchOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['search-providers'] })
  }
})

watch(() => openStatus.addFetchOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['fetch-providers'] })
  }
})

watch(providers, (list) => {
  const id = curProvider.value?.id
  if (!id) return
  const found = list.find((p) => p.id === id)
  if (found) curProvider.value = found
  else if (view.value === 'detail' && detailKind.value === 'search') backToList()
})

watch(fetchProviders, (list) => {
  const id = curFetchProvider.value?.id
  if (!id) return
  const found = list.find((p) => p.id === id)
  if (found) curFetchProvider.value = found
  else if (view.value === 'detail' && detailKind.value === 'fetch') backToList()
})
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Two provider sections -->
    <section
      v-if="view === 'list'"
      class="mx-auto max-w-3xl px-6 pt-10 pb-12 space-y-8"
    >
      <h1 class="px-2 text-lg font-semibold">
        {{ t('webSearch.title') }}
      </h1>

      <!-- Search providers -->
      <section class="space-y-2.5">
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0 px-2">
            <h2 class="text-label font-medium text-foreground">
              {{ t('webSearch.searchProviders') }}
            </h2>
            <p class="text-xs text-muted-foreground">
              {{ t('webSearch.searchHint') }}
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            class="shrink-0"
            @click="openStatus.addSearchOpen = true"
          >
            <Plus class="size-4" />
            {{ t('common.add') }}
          </Button>
        </div>

        <div
          v-if="providers.length > 0"
          class="grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <BackendCard
            v-for="provider in providers"
            :key="provider.id"
            :name="provider.name ?? ''"
            :enabled="provider.enable !== false"
            @click="openProvider(provider)"
          >
            <template #leading>
              <span class="flex size-10 items-center justify-center">
                <SearchProviderLogo
                  :provider="provider.provider || ''"
                  size="md"
                />
              </span>
            </template>
          </BackendCard>
        </div>
        <p
          v-else
          class="px-2 text-xs text-muted-foreground"
        >
          {{ t('webSearch.empty') }}
        </p>
      </section>

      <!-- Fetch providers -->
      <section class="space-y-2.5">
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0 px-2">
            <h2 class="text-label font-medium text-foreground">
              {{ t('webSearch.fetchProviders') }}
            </h2>
            <p class="text-xs text-muted-foreground">
              {{ t('webSearch.fetchHint') }}
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            class="shrink-0"
            @click="openStatus.addFetchOpen = true"
          >
            <Plus class="size-4" />
            {{ t('common.add') }}
          </Button>
        </div>

        <div
          v-if="fetchProviders.length > 0"
          class="grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <BackendCard
            v-for="provider in fetchProviders"
            :key="provider.id"
            :name="provider.name ?? ''"
            :enabled="provider.enable !== false"
            @click="openFetchProvider(provider)"
          >
            <template #leading>
              <span class="flex size-10 items-center justify-center">
                <SearchProviderLogo
                  :provider="provider.provider || ''"
                  size="md"
                />
              </span>
            </template>
          </BackendCard>
        </div>
        <p
          v-else
          class="px-2 text-xs text-muted-foreground"
        >
          {{ t('webSearch.emptyFetch') }}
        </p>
      </section>

      <AddSearchProvider
        v-model:open="openStatus.addSearchOpen"
        hide-trigger
      />
      <AddFetchProvider
        v-model:open="openStatus.addFetchOpen"
        hide-trigger
      />
    </section>

    <!-- Provider detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('webSearch.title')"
      @back="backToList()"
    >
      <ProviderSetting v-if="detailKind === 'search' && curProvider?.id" />
      <FetchProviderSetting v-else-if="detailKind === 'fetch' && curFetchProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
