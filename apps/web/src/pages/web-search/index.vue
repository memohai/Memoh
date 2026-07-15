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

// 'webProvider' query key (unique per settings page — see useViewSwap.ts),
// valued `<kind>:<id>` — the detail pane can show either a search or a fetch
// provider, so the URL must carry which list the ID belongs to for a refresh
// to restore the right pane. Also keeps the sidebar re-click affordance working.
const { view, direction, queryValue, openDetail, backToList } = useViewSwap('webProvider')
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
  openDetail(`search:${provider.id}`)
}

function openFetchProvider(provider: FetchprovidersGetResponse) {
  curFetchProvider.value = provider
  detailKind.value = 'fetch'
  openDetail(`fetch:${provider.id}`)
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

// Resolve the URL's `<kind>:<id>` against the matching list: restores the open
// provider (and which pane kind) on refresh, follows refetched data, and falls
// back to the list if it was deleted while open. Each kind only consults its
// own list and only treats "not found" as deleted once that list's data has
// actually arrived — the empty list during the initial fetch proves nothing.
watch([queryValue, providers, fetchProviders], ([raw, search, fetchList]) => {
  if (!raw) return
  const sep = raw.indexOf(':')
  if (sep === -1) {
    // Malformed value (hand-edited URL) — nothing will ever match it.
    backToList()
    return
  }
  const kind = raw.slice(0, sep)
  const id = raw.slice(sep + 1)
  if (kind === 'search') {
    const found = search.find((p) => p.id === id)
    if (found) {
      detailKind.value = 'search'
      curProvider.value = found
    } else if (providerData.value !== undefined) {
      backToList()
    }
  } else if (kind === 'fetch') {
    const found = fetchList.find((p) => p.id === id)
    if (found) {
      detailKind.value = 'fetch'
      curFetchProvider.value = found
    } else if (fetchProviderData.value !== undefined) {
      backToList()
    }
  } else {
    // Unknown kind — nothing will ever match it.
    backToList()
  }
}, { immediate: true })
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
      @back="backToList()"
    >
      <ProviderSetting v-if="detailKind === 'search' && curProvider?.id" />
      <FetchProviderSetting v-else-if="detailKind === 'fetch' && curFetchProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
