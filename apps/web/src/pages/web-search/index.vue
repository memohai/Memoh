<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
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
import { getSearchProviders } from '@memohai/sdk'
import type { SearchprovidersGetResponse } from '@memohai/sdk'
import { Globe, Plus, Search } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddSearchProvider from './components/add-search-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'

const { t } = useI18n()

const { data: providerData } = useQuery({
  key: () => ['search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({ throwOnError: true })
    return data
  },
})

const curProvider = ref<SearchprovidersGetResponse>()
provide('curSearchProvider', curProvider)

const { view, direction, openDetail, backToList } = useViewSwap()
const searchQuery = ref('')
const openStatus = reactive({ addOpen: false })

const providers = computed<SearchprovidersGetResponse[]>(() => {
  if (!Array.isArray(providerData.value)) return []
  return [...providerData.value].sort((a, b) => {
    const ae = a.enable !== false ? 1 : 0
    const be = b.enable !== false ? 1 : 0
    return be - ae
  })
})

// Always offer search once there's anything to filter — a hidden-then-appearing
// box read as inconsistent (some providers showed it, some didn't).
const showSearch = computed(() => providers.value.length > 0)

const filteredProviders = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter((p) =>
    (p.name ?? '').toLowerCase().includes(keyword)
    || (p.provider ?? '').toLowerCase().includes(keyword),
  )
})

function openProvider(provider: SearchprovidersGetResponse) {
  curProvider.value = provider
  openDetail()
}

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
    <!-- Backend list -->
    <section
      v-if="view === 'list'"
      class="mx-auto max-w-3xl px-6 pt-10 pb-12"
    >
      <header class="mb-6 flex items-center justify-between gap-4">
        <h1 class="px-2 text-lg font-semibold">
          {{ t('webSearch.title') }}
        </h1>
        <div class="flex items-center gap-2">
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
                :placeholder="t('webSearch.searchPlaceholder')"
              />
            </InputGroup>
          </div>
          <Button @click="openStatus.addOpen = true">
            <Plus class="size-4" />
            {{ t('webSearch.add') }}
          </Button>
        </div>
      </header>

      <div
        v-if="providers.length > 0"
        class="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <BackendCard
          v-for="provider in filteredProviders"
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

        <button
          type="button"
          class="group/add flex min-h-[4.5rem] items-center justify-center gap-2 rounded-[var(--radius-menu-shell)] border border-dashed border-border bg-background text-sm text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="openStatus.addOpen = true"
        >
          <Plus class="size-4" />
          {{ t('webSearch.add') }}
        </button>
      </div>

      <Empty
        v-else
        class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-16"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Globe />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ t('webSearch.emptyTitle') }}</EmptyTitle>
        <EmptyDescription>{{ t('webSearch.emptyDescription') }}</EmptyDescription>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('webSearch.add') }}
          </Button>
        </EmptyContent>
      </Empty>

      <AddSearchProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
      />
    </section>

    <!-- Engine detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('webSearch.title')"
      @back="backToList()"
    >
      <ProviderSetting v-if="curProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
