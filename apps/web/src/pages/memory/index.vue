<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import { Button } from '@memohai/ui'
import { getMemoryProviders } from '@memohai/sdk'
import type { AdaptersProviderGetResponse } from '@memohai/sdk'
import { Brain, ChevronRight, Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddMemoryProvider from './components/add-memory-provider.vue'
import BuiltinConfig from './components/builtin-config.vue'
import ProviderSetting from './components/provider-setting.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'

const { t } = useI18n()

const { data: providerData } = useQuery({
  key: () => ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const providers = computed<AdaptersProviderGetResponse[]>(() =>
  Array.isArray(providerData.value) ? providerData.value : [],
)
const builtinProvider = computed(() => providers.value.find((p) => p.provider === 'builtin') ?? null)
const externalProviders = computed(() => providers.value.filter((p) => p.provider !== 'builtin'))

// Only the external (advanced) backend opened in the detail pane uses this.
const curProvider = ref<AdaptersProviderGetResponse | null>(null)
provide('curMemoryProvider', curProvider)

const { view, direction, openDetail, backToList } = useViewSwap()
const advancedOpen = ref(false)
const openStatus = reactive({ addOpen: false })

function openExternal(provider: AdaptersProviderGetResponse) {
  curProvider.value = provider
  openDetail()
}

watch(externalProviders, (list) => {
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
    <!-- Capability config -->
    <section
      v-if="view === 'list'"
      class="mx-auto max-w-3xl px-6 pt-10 pb-12 space-y-8"
    >
      <header>
        <h1 class="text-lg font-semibold">
          {{ t('sidebar.memory') }}
        </h1>
      </header>

      <BuiltinConfig :provider="builtinProvider" />

      <div class="space-y-3 border-t border-border pt-6">
        <button
          type="button"
          class="flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none"
          @click="advancedOpen = !advancedOpen"
        >
          <ChevronRight
            class="size-4 transition-transform"
            :class="advancedOpen && 'rotate-90'"
          />
          {{ t('memory.advanced') }}
        </button>

        <template v-if="advancedOpen">
          <p class="text-xs text-muted-foreground">
            {{ t('memory.advancedHint') }}
          </p>

          <div
            v-if="externalProviders.length > 0"
            class="grid grid-cols-1 gap-3 sm:grid-cols-2"
          >
            <BackendCard
              v-for="provider in externalProviders"
              :key="provider.id"
              :name="provider.name ?? ''"
              :subtitle="t(`memory.providerNames.${provider.provider}`, provider.provider ?? '')"
              @click="openExternal(provider)"
            >
              <template #leading>
                <span class="flex size-10 items-center justify-center rounded-full bg-muted">
                  <Brain class="size-5 text-muted-foreground" />
                </span>
              </template>
            </BackendCard>
          </div>

          <Button
            variant="outline"
            size="sm"
            @click="openStatus.addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('memory.add') }}
          </Button>
        </template>
      </div>

      <AddMemoryProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
      />
    </section>

    <!-- External backend detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('sidebar.memory')"
      @back="backToList()"
    >
      <ProviderSetting v-if="curProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
