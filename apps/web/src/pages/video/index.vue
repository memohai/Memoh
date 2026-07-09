<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Button } from '@felinic/ui'
import { getVideoProviders, postVideoProvidersByIdImportModels } from '@memohai/sdk'
import type { VideoProviderResponse } from '@memohai/sdk'
import { Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddProvider from '@/components/add-provider/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import PageShell from '@/components/page-shell/index.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import VideoProviderSetting from './provider-setting.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: providersData } = useQuery({
  key: () => ['video-providers'],
  query: async () => {
    const { data } = await getVideoProviders({ throwOnError: true })
    return data
  },
})

const curProvider = ref<VideoProviderResponse>()
provide('curVideoProvider', curProvider)

// 'detail' query key: see useViewSwap.ts — makes re-clicking Video in the
// settings sidebar while a provider's detail is open actually navigate back.
const { view, direction, openDetail, backToList } = useViewSwap('detail')
const openStatus = reactive({ addOpen: false })

const providers = computed<VideoProviderResponse[]>(() => {
  const list = Array.isArray(providersData.value) ? providersData.value : []
  return [...list].sort((a, b) => Number(b.enable !== false) - Number(a.enable !== false))
})

const addProviderNames = computed(() => providers.value.map((p) => ({ name: p.name })))

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function openProvider(provider: VideoProviderResponse) {
  curProvider.value = provider
  openDetail()
}

async function importVideoModels(providerId: string) {
  const { data } = await postVideoProvidersByIdImportModels({
    path: { id: providerId },
    throwOnError: true,
  })
  queryCache.invalidateQueries({ key: ['video-providers'] })
  queryCache.invalidateQueries({ key: ['video-models'] })
  queryCache.invalidateQueries({ key: ['video-provider-models', providerId] })
  return data
}

watch(() => openStatus.addOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['video-providers'] })
    queryCache.invalidateQueries({ key: ['video-models'] })
  }
})

watch(providers, (list) => {
  const id = curProvider.value?.id
  if (!id) return
  const found = list.find((p) => p.id === id)
  if (found) curProvider.value = found
  else if (view.value === 'detail') backToList()
})
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Single provider group — same shell as the providers gallery: PageShell owns
         the title, the hint (as its description), and the Add action; the body is a
         bare BackendCard grid. NOT a SectionGroup — that owner is only for pages that
         stack SEVERAL provider groups (voice TTS/STT, web-search search/fetch). The
         page-level "视频生成" heading was dropped: title + description already say it. -->
    <PageShell
      v-if="view === 'list'"
      :title="t('video.title')"
      :description="t('video.providersHint')"
    >
      <template #actions>
        <Button @click="openStatus.addOpen = true">
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
                {{ getInitials(provider.name) }}
              </span>
            </span>
          </template>
        </BackendCard>
      </div>
      <p
        v-else
        class="px-2 text-xs text-muted-foreground"
      >
        {{ t('video.empty') }}
      </p>

      <AddProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
        preset-domain="video"
        :providers="addProviderNames"
        :import-models="importVideoModels"
      />
    </PageShell>

    <DetailPane
      v-else
      width="narrow"
      :back-label="t('video.title')"
      @back="backToList()"
    >
      <VideoProviderSetting />
    </DetailPane>
  </SwapTransition>
</template>
