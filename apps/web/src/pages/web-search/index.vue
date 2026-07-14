<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Button } from '@felinic/ui'
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
import PageShell from '@/components/page-shell/index.vue'
import SectionGroup from '@/components/section-group/index.vue'
import { useRoutedViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: providerData, isLoading: providersLoading } = useQuery({
  key: () => ['search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({ throwOnError: true })
    return data
  },
})

const { data: fetchProviderData, isLoading: fetchProvidersLoading } = useQuery({
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

type WebDetailKind = 'search' | 'fetch'
type WebDetail =
  | { kind: 'search', provider: SearchprovidersGetResponse }
  | { kind: 'fetch', provider: FetchprovidersGetResponse }
const detailKind = ref<WebDetailKind>('search')
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

const { view, direction, openDetail, backToList: closeProvider } = useRoutedViewSwap<WebDetail>({
  key: 'provider',
  items: () => [
    ...providers.value.map(provider => ({ kind: 'search' as const, provider })),
    ...fetchProviders.value.map(provider => ({ kind: 'fetch' as const, provider })),
  ],
  selected: () => {
    if (detailKind.value === 'search' && curProvider.value) {
      return { kind: 'search', provider: curProvider.value }
    }
    if (detailKind.value === 'fetch' && curFetchProvider.value) {
      return { kind: 'fetch', provider: curFetchProvider.value }
    }
    return undefined
  },
  select: (detail) => {
    detailKind.value = detail?.kind ?? 'search'
    curProvider.value = detail?.kind === 'search' ? detail.provider : undefined
    curFetchProvider.value = detail?.kind === 'fetch' ? detail.provider : undefined
  },
  getRouteValue: detail => `${detail.kind}:${detail.provider.id}`,
  isLoading: routeValue => routeValue.startsWith('search:')
    ? providersLoading.value
    : routeValue.startsWith('fetch:') && fetchProvidersLoading.value,
  isReady: routeValue => routeValue.startsWith('search:')
    ? providerData.value !== undefined
    : routeValue.startsWith('fetch:')
      ? fetchProviderData.value !== undefined
      : true,
})

function openProvider(provider: SearchprovidersGetResponse) {
  openDetail({ kind: 'search', provider })
}

function openFetchProvider(provider: FetchprovidersGetResponse) {
  openDetail({ kind: 'fetch', provider })
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

</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Two provider sections (search + fetch). Twin of the voice/video provider
         pages: PageShell title bar + SectionGroup per provider kind. -->
    <PageShell
      v-if="view === 'list'"
      :title="t('webSearch.title')"
    >
      <div class="space-y-8">
        <!-- Search providers -->
        <SectionGroup
          :title="t('webSearch.searchProviders')"
          :description="t('webSearch.searchHint')"
        >
          <template #actions>
            <Button
              variant="secondary"
              size="sm"
              @click="openStatus.addSearchOpen = true"
            >
              <Plus class="size-4" />
              {{ t('common.add') }}
            </Button>
          </template>

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
        </SectionGroup>

        <!-- Fetch providers -->
        <SectionGroup
          :title="t('webSearch.fetchProviders')"
          :description="t('webSearch.fetchHint')"
        >
          <template #actions>
            <Button
              variant="secondary"
              size="sm"
              @click="openStatus.addFetchOpen = true"
            >
              <Plus class="size-4" />
              {{ t('common.add') }}
            </Button>
          </template>

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
        </SectionGroup>
      </div>

      <AddSearchProvider
        v-model:open="openStatus.addSearchOpen"
        hide-trigger
      />
      <AddFetchProvider
        v-model:open="openStatus.addFetchOpen"
        hide-trigger
      />
    </PageShell>

    <!-- Provider detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('webSearch.title')"
      @back="closeProvider"
    >
      <ProviderSetting v-if="detailKind === 'search' && curProvider?.id" />
      <FetchProviderSetting v-else-if="detailKind === 'fetch' && curFetchProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
