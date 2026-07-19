<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import { Button, Collapsible, CollapsibleContent, CollapsibleTrigger, Spinner } from '@felinic/ui'
import { getMemoryProviders, getMemoryProvidersMeta } from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderMeta } from '@memohai/sdk'
import { Brain, ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import BuiltinConfig from './components/builtin-config.vue'
import ProviderSetting from './components/provider-setting.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import PageShell from '@/components/page-shell/index.vue'
import { useRoutedViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import { providerConfigDefaults } from '@/utils/provider-template'

const { t } = useI18n()
const MEMORY_PROVIDER_TYPES = ['mem0', 'openviking'] as const

const { data: providerData, isLoading: providersLoading } = useQuery({
  key: () => ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const { data: providerMetaData } = useQuery({
  key: () => ['memory-providers-meta'],
  query: async () => {
    const { data } = await getMemoryProvidersMeta({ throwOnError: true })
    return data
  },
})

const providers = computed<AdaptersProviderGetResponse[]>(() =>
  Array.isArray(providerData.value) ? providerData.value : [],
)
const builtinProvider = computed(() => providers.value.find((p) => p.provider === 'builtin') ?? null)
const providerMetas = computed<AdaptersProviderMeta[]>(() =>
  Array.isArray(providerMetaData.value) ? providerMetaData.value : [],
)
const optimisticProviders = ref<Record<string, AdaptersProviderGetResponse>>({})
const externalProviders = computed<AdaptersProviderGetResponse[]>(() => MEMORY_PROVIDER_TYPES.map((provider) => {
  const meta = providerMetas.value.find(item => item.provider === provider)
  return providers.value.find(instance => instance.provider === provider)
    ?? optimisticProviders.value[provider]
    ?? {
      name: meta?.display_name ?? t(`memory.providerNames.${provider}`, provider),
      provider,
      config: providerConfigDefaults(meta?.config_schema),
    }
}))

// Only the external (advanced) backend opened in the detail pane uses this.
const curProvider = ref<AdaptersProviderGetResponse | null>(null)
provide('curMemoryProvider', curProvider)

const advancedOpen = ref(false)

// The built-in config owns the mode/model draft + save; the Save button lives in
// this page's header (#actions), so read its state off the child instead of
// hoisting all the memory logic up here.
const builtinRef = ref<InstanceType<typeof BuiltinConfig> | null>(null)

// Reveal Advanced on first load only when the user already configured an
// external backend — a set-up backend should never sit hidden — while leaving
// it collapsed for the common built-in-only case. `didAutoOpen` makes this a
// one-shot so a later manual collapse isn't fought by refreshed data.
let didAutoOpen = false

// Page-owned query key (unique under settings KeepAlive — see useViewSwap.ts).
const {
  view,
  direction,
  isDetailLoading,
  openDetail: openExternal,
  backToList: closeExternal,
} = useRoutedViewSwap({
  key: 'memoryBackend',
  items: () => externalProviders.value,
  selected: () => curProvider.value ?? undefined,
  select: provider => curProvider.value = provider ?? null,
  getRouteValue: provider => provider.provider ?? '',
  isLoading: () => providersLoading.value,
  isReady: () => providerData.value !== undefined,
})

watch(externalProviders, (items) => {
  if (!didAutoOpen && items.length > 0) {
    advancedOpen.value = true
    didAutoOpen = true
  }
}, { immediate: true })

function handleMaterialized(provider: AdaptersProviderGetResponse) {
  if (!provider.provider) return
  optimisticProviders.value = {
    ...optimisticProviders.value,
    [provider.provider]: provider,
  }
  curProvider.value = provider
}
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Capability config -->
    <PageShell
      v-if="view === 'list'"
      :title="t('sidebar.memory')"
    >
      <!-- Root-page manual save: switching mode / picking a model provisions an
           index backend, so it batches behind one deliberate Save rather than
           auto-saving each toggle. It lives in the header (disabled while synced)
           — the house pattern for a PageShell page — not a footer band inside
           the card. -->
      <template #actions>
        <Button
          :disabled="!builtinRef?.hasChanges || builtinRef?.saveLoading"
          @click="builtinRef?.save()"
        >
          <Spinner
            v-if="builtinRef?.saveLoading"
            class="size-3"
          />
          {{ t('common.saveChanges') }}
        </Button>
      </template>

      <div class="space-y-8">
        <BuiltinConfig
          ref="builtinRef"
          :provider="builtinProvider"
        />

        <!-- External backends are an advanced, rarely-touched concern, so they
             stay behind a disclosure to keep the common built-in path clean
             (99/1). The reveal is a plain, in-language group — no page-splitting
             hairline; the section rhythm above already separates it. -->
        <Collapsible v-model:open="advancedOpen">
          <CollapsibleTrigger
            class="flex items-center gap-1.5 rounded-[var(--radius-control)] px-2 py-1 text-label font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ChevronRight
              class="size-4 transition-transform"
              :class="advancedOpen && 'rotate-90'"
            />
            {{ t('memory.advanced') }}
          </CollapsibleTrigger>

          <CollapsibleContent class="space-y-3 pt-3">
            <p class="px-2 text-xs text-muted-foreground">
              {{ t('memory.advancedHint') }}
            </p>

            <div
              v-if="externalProviders.length > 0"
              class="grid grid-cols-1 gap-3 sm:grid-cols-2"
            >
              <BackendCard
                v-for="provider in externalProviders"
                :key="provider.provider"
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
          </CollapsibleContent>
        </Collapsible>
      </div>
    </PageShell>

    <!-- External backend detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('sidebar.memory')"
      :loading="isDetailLoading || !curProvider"
      @back="closeExternal"
    >
      <ProviderSetting
        v-if="curProvider"
        @materialized="handleMaterialized"
      />
    </DetailPane>
  </SwapTransition>
</template>
