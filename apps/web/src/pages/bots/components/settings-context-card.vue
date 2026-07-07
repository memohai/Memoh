<!-- eslint-disable vue/no-mutating-props -->
<template>
  <SettingsSection :title="$t('bots.settings.blocks.context')">
    <SettingsRow :label="$t('bots.settings.searchProvider')">
      <div class="w-52">
        <SearchProviderSelect
          v-model="form.search_provider_id"
          :providers="searchProviders"
          :placeholder="$t('bots.settings.searchProviderPlaceholder')"
        />
      </div>
    </SettingsRow>

    <SettingsRow :label="$t('bots.settings.fetchProvider')">
      <div class="w-52">
        <FetchProviderSelect
          v-model="form.fetch_provider_id"
          :providers="fetchProviders"
          :placeholder="$t('bots.settings.fetchProviderPlaceholder')"
        />
      </div>
    </SettingsRow>

    <SettingsRow :label="$t('bots.settings.memoryProvider')">
      <div class="w-52">
        <MemoryProviderSelect
          v-model="form.memory_provider_id"
          :providers="memoryProviders"
          :placeholder="$t('bots.settings.memoryProviderPlaceholder')"
        />
      </div>
    </SettingsRow>

    <!-- Memory status (conditional, shown below the provider row) -->
    <div
      v-if="showMemoryProviderStatusCard"
      class="mx-4 pb-4 pt-2 space-y-2"
    >
      <div class="flex items-center justify-between rounded-[var(--radius-menu-shell)] border border-border bg-muted/20 px-3 py-2">
        <div class="space-y-0.5">
          <p class="text-xs font-medium text-foreground">
            {{ indexedMemoryStatusTitle }}
          </p>
          <p class="text-caption text-muted-foreground">
            {{ isSelectedMemoryProviderPersisted ? $t('bots.settings.memoryHealthOk') : $t('bots.settings.indexedMemoryStatusPendingSave') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            :disabled="!isSelectedMemoryProviderPersisted || !memoryStatus?.can_manual_sync"
            :loading="isRebuilding"
            class="h-7 px-3 text-xs"
            @click="$emit('sync-memory')"
          >
            {{ $t('bots.settings.memorySyncAction') }}
          </Button>
          <Button
            v-if="isSelectedMemoryProviderPersisted"
            variant="ghost"
            size="sm"
            class="h-7 px-3 text-xs text-muted-foreground hover:text-foreground"
            @click="showDetails = !showDetails"
          >
            {{ showDetails ? $t('bots.settings.hideMemoryDetails') : $t('bots.settings.showMemoryDetails') }}
            <ChevronDown
              class="ml-1 size-3.5 transition-transform"
              :class="showDetails ? 'rotate-180' : ''"
            />
          </Button>
        </div>
      </div>

      <div
        v-if="showDetails && isSelectedMemoryProviderPersisted"
        class="grid gap-2 sm:grid-cols-3"
      >
        <div
          v-if="isMemoryStatusLoading"
          class="text-xs text-muted-foreground col-span-full py-4 text-center border rounded-[var(--radius-menu-shell)] border-dashed"
        >
          <Spinner class="inline-block mr-2 align-text-bottom size-3" />
          {{ $t('common.loading') }}
        </div>

        <template v-else-if="statusCardData">
          <MetricReadout :label="$t('bots.settings.memorySourceDir')">
            <template #value>
              <span class="text-xs font-mono font-medium text-foreground break-all leading-snug">
                {{ statusCardData.source_dir || '-' }}
              </span>
            </template>
          </MetricReadout>

          <MetricReadout :label="$t('bots.settings.memoryOverviewPath')">
            <template #value>
              <span class="text-xs font-mono font-medium text-foreground break-all leading-snug">
                {{ statusCardData.overview_path || '-' }}
              </span>
            </template>
          </MetricReadout>

          <MetricReadout :label="$t('bots.settings.memoryMarkdownFiles')">
            <template #value>
              <span class="text-base font-mono font-semibold text-foreground leading-none">
                {{ statusCardData.markdown_file_count ?? 0 }}
              </span>
            </template>
          </MetricReadout>

          <MetricReadout :label="$t('bots.settings.memorySourceEntries')">
            <template #value>
              <span class="text-base font-mono font-semibold text-foreground leading-none">
                {{ statusCardData.source_count ?? 0 }}
              </span>
            </template>
          </MetricReadout>

          <MetricReadout
            :label="selectedMemoryProviderType === 'builtin' ? $t('bots.settings.memoryGraphEdges') : $t('bots.settings.memoryIndexedEntries')"
            :value="String(selectedMemoryProviderType === 'builtin' ? (statusCardData.edge_count ?? 0) : (statusCardData.indexed_count ?? 0))"
          />

          <MetricReadout
            v-if="showPgvectorDetails"
            :label="$t('bots.settings.memorySemanticEntries')"
            :value="String(statusCardData.indexed_count ?? 0)"
          />

          <MetricReadout
            v-if="showPgvectorDetails"
            :label="$t('bots.settings.memoryVectorIndex')"
            class="sm:col-span-1"
          >
            <template #value>
              <span class="text-xs font-mono font-medium text-foreground break-all leading-snug">
                {{ statusCardData.vector_index || '-' }}
              </span>
            </template>
          </MetricReadout>

          <MetricReadout
            v-if="showEncoderHealth"
            :label="encoderHealthLabel"
            :status="statusCardData.encoder?.ok ? 'ok' : 'error'"
          >
            <template #value>
              {{ healthLabel(statusCardData.encoder?.ok, statusCardData.encoder?.error) }}
            </template>
          </MetricReadout>

          <MetricReadout
            v-if="showPgvectorHealth"
            :label="$t('bots.settings.memoryPgvectorHealth')"
            :status="statusCardData.pgvector?.ok ? 'ok' : 'error'"
          >
            <template #value>
              {{ healthLabel(statusCardData.pgvector?.ok, statusCardData.pgvector?.error) }}
            </template>
          </MetricReadout>
        </template>
      </div>
    </div>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Button, Spinner } from '@memohai/ui'
import { ChevronDown } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import SearchProviderSelect from './search-provider-select.vue'
import FetchProviderSelect from './fetch-provider-select.vue'
import MemoryProviderSelect from './memory-provider-select.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import MetricReadout from '@/components/settings/metric-readout.vue'
import type {
  SettingsSettings,
  AdaptersProviderGetResponse,
  FetchprovidersGetResponse,
  SearchprovidersGetResponse,
  AdaptersMemoryStatusResponse,
} from '@memohai/sdk'

const props = defineProps<{
  form: SettingsSettings
  searchProviders: SearchprovidersGetResponse[]
  fetchProviders: FetchprovidersGetResponse[]
  memoryProviders: AdaptersProviderGetResponse[]
  persistedMemoryProviderID: string
  memoryStatus: AdaptersMemoryStatusResponse | null
  isMemoryStatusLoading: boolean
  isRebuilding: boolean
}>()

defineEmits<{
  'sync-memory': []
}>()

const { t } = useI18n()

const showDetails = ref(false)

const selectedMemoryProvider = computed(() =>
  props.memoryProviders.find((provider) => provider.id === props.form.memory_provider_id),
)
const selectedMemoryProviderType = computed(() =>
  selectedMemoryProvider.value?.provider ?? '',
)
const selectedBuiltinMemoryProvider = computed(() =>
  selectedMemoryProvider.value?.provider === 'builtin' ? selectedMemoryProvider.value : null,
)
const selectedMem0MemoryProvider = computed(() =>
  selectedMemoryProvider.value?.provider === 'mem0' ? selectedMemoryProvider.value : null,
)
const isSelectedMemoryProviderPersisted = computed(() =>
  !!props.form.memory_provider_id && props.form.memory_provider_id === props.persistedMemoryProviderID,
)
const showBuiltinIndexedMemoryStatus = computed(() => !!selectedBuiltinMemoryProvider.value)
const showMemoryProviderStatusCard = computed(() =>
  showBuiltinIndexedMemoryStatus.value || !!selectedMem0MemoryProvider.value,
)

const indexedMemoryStatusTitle = computed(() => {
  if (selectedMemoryProviderType.value === 'mem0') return t('bots.settings.mem0StatusTitle')
  return t('bots.settings.graphStatusTitle')
})

const statusCardData = computed(() => props.memoryStatus)
const showPgvectorDetails = computed(() => !!statusCardData.value?.vector_index)
const showEncoderHealth = computed(() => false)
const showPgvectorHealth = computed(() => !!statusCardData.value?.vector_index)
const encoderHealthLabel = computed(() => '')

function healthLabel(ok: boolean | undefined, error?: string) {
  if (ok) return t('bots.settings.memoryHealthOk')
  if (error) return error
  return t('bots.settings.memoryHealthUnavailable')
}
</script>
