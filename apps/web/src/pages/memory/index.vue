<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import { Button, Collapsible, CollapsibleContent, CollapsibleTrigger, Spinner } from '@felinic/ui'
import { getMemoryProviders } from '@memohai/sdk'
import type { AdaptersProviderGetResponse } from '@memohai/sdk'
import { Brain, ChevronRight, Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddMemoryProvider from './components/add-memory-provider.vue'
import BuiltinConfig from './components/builtin-config.vue'
import ProviderSetting from './components/provider-setting.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import PageShell from '@/components/page-shell/index.vue'
import { useRoutedViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'

const { t } = useI18n()

const { data: providerData, isLoading: providersLoading } = useQuery({
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

const advancedOpen = ref(false)
const openStatus = reactive({ addOpen: false })

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
  getRouteValue: provider => provider.id ?? '',
  isLoading: () => providersLoading.value,
  isReady: () => providerData.value !== undefined,
})

watch(externalProviders, (list) => {
  if (!didAutoOpen && list.length > 0) {
    advancedOpen.value = true
    didAutoOpen = true
  }
}, { immediate: true })
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
          </CollapsibleContent>
        </Collapsible>
      </div>

      <AddMemoryProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
      />
    </PageShell>

    <!-- External backend detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('sidebar.memory')"
      :loading="isDetailLoading || !curProvider?.id"
      @back="closeExternal"
    >
      <ProviderSetting v-if="curProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
