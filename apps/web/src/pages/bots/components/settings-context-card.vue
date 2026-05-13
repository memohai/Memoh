<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <!-- Header Section -->
    <div class="space-y-0.5">
      <h4 class="text-xs font-medium text-foreground">
        {{ $t('bots.settings.blocks.context') }}
      </h4>
      <p class="text-[11px] text-muted-foreground">
        {{ $t('bots.settings.blocks.contextDescription') }}
      </p>
    </div>

    <!-- Configuration inputs with compact spacing -->
    <div class="space-y-3 pt-1">
      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.searchProvider') }}</Label>
        <SearchProviderSelect
          v-model="form.search_provider_id"
          :providers="searchProviders"
          :placeholder="$t('bots.settings.searchProviderPlaceholder')"
        />
      </div>

      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.memoryProvider') }}</Label>
        <MemoryProviderSelect
          v-model="form.memory_provider_id"
          :providers="memoryProviders"
          :placeholder="$t('bots.settings.memoryProviderPlaceholder')"
        />
        
        <!-- Bento-style memory status refactoring -->
        <div
          v-if="showMemoryProviderStatusCard"
          class="mt-3 space-y-2"
        >
          <div class="flex items-center justify-between rounded-md border border-border bg-muted/20 px-3 py-2 shadow-none">
            <div class="space-y-0.5">
              <p class="text-xs font-medium text-foreground">
                {{ indexedMemoryStatusTitle }}
              </p>
              <p class="text-[10px] text-muted-foreground">
                {{ isSelectedMemoryProviderPersisted ? $t('bots.settings.memoryHealthOk') : $t('bots.settings.indexedMemoryStatusPendingSave') }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                :disabled="!isSelectedMemoryProviderPersisted || isRebuilding || !memoryStatus?.can_manual_sync"
                class="shadow-none h-7 px-3 text-xs bg-background"
                @click="$emit('sync-memory')"
              >
                <Spinner
                  v-if="isRebuilding"
                  class="mr-1.5 size-3"
                />
                {{ $t('bots.settings.memorySyncAction') }}
              </Button>
              <Button
                v-if="isSelectedMemoryProviderPersisted"
                variant="ghost"
                size="sm"
                class="h-7 px-3 text-xs text-muted-foreground hover:text-foreground shadow-none"
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
              class="text-xs text-muted-foreground col-span-full py-4 text-center border rounded-md border-dashed"
            >
              <Spinner class="inline-block mr-2 align-text-bottom size-3" />
              {{ $t('common.loading') }}
            </div>
            
            <template v-else-if="statusCardData">
              <div class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]">
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memorySourceDir') }}
                </p>
                <p class="mt-1 text-xs font-mono font-medium text-foreground break-all leading-snug">
                  {{ statusCardData.source_dir || '-' }}
                </p>
              </div>

              <div class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]">
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memoryOverviewPath') }}
                </p>
                <p class="mt-1 text-xs font-mono font-medium text-foreground break-all leading-snug">
                  {{ statusCardData.overview_path || '-' }}
                </p>
              </div>

              <div class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]">
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memoryMarkdownFiles') }}
                </p>
                <p class="mt-1 text-base font-mono font-semibold text-foreground leading-none">
                  {{ statusCardData.markdown_file_count ?? 0 }}
                </p>
              </div>

              <div class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]">
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memorySourceEntries') }}
                </p>
                <p class="mt-1 text-base font-mono font-semibold text-foreground leading-none">
                  {{ statusCardData.source_count ?? 0 }}
                </p>
              </div>

              <div class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]">
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memoryIndexedEntries') }}
                </p>
                <p class="mt-1 text-base font-mono font-semibold text-foreground leading-none">
                  {{ statusCardData.indexed_count ?? 0 }}
                </p>
              </div>

              <div
                v-if="showQdrantDetails"
                class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px] sm:col-span-1"
              >
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memoryQdrantCollection') }}
                </p>
                <p class="mt-1 text-xs font-mono font-medium text-foreground break-all leading-snug">
                  {{ statusCardData.qdrant_collection || '-' }}
                </p>
              </div>

              <div
                v-if="showEncoderHealth"
                class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]"
              >
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ encoderHealthLabel }}
                </p>
                <div class="flex items-center gap-1.5 mt-1">
                  <div
                    class="size-1.5 rounded-full"
                    :class="statusCardData.encoder?.ok ? 'bg-foreground' : 'bg-destructive'"
                  />
                  <p
                    class="text-xs font-medium leading-none"
                    :class="healthTextClass(statusCardData.encoder?.ok)"
                  >
                    {{ healthLabel(statusCardData.encoder?.ok, statusCardData.encoder?.error) }}
                  </p>
                </div>
              </div>

              <div
                v-if="showQdrantHealth"
                class="rounded-md border border-border bg-background p-3 flex flex-col justify-between min-h-[70px]"
              >
                <p class="text-[10px] text-muted-foreground tracking-tight">
                  {{ $t('bots.settings.memoryQdrantHealth') }}
                </p>
                <div class="flex items-center gap-1.5 mt-1">
                  <div
                    class="size-1.5 rounded-full"
                    :class="statusCardData.qdrant?.ok ? 'bg-foreground' : 'bg-destructive'"
                  />
                  <p
                    class="text-xs font-medium leading-none"
                    :class="healthTextClass(statusCardData.qdrant?.ok)"
                  >
                    {{ healthLabel(statusCardData.qdrant?.ok, statusCardData.qdrant?.error) }}
                  </p>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Label, Button, Spinner } from '@memohai/ui'
import { ChevronDown } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import SearchProviderSelect from './search-provider-select.vue'
import MemoryProviderSelect from './memory-provider-select.vue'
import type { 
  SettingsSettings, 
  AdaptersProviderGetResponse, 
  AdaptersMemoryStatusResponse 
} from '@memohai/sdk'

const props = defineProps<{
  form: SettingsSettings
  searchProviders: AdaptersProviderGetResponse[]
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
const selectedBuiltinMemoryMode = computed(() =>
  (selectedBuiltinMemoryProvider.value?.config as Record<string, string> | undefined)?.memory_mode || 'off',
)
const isSelectedMemoryProviderPersisted = computed(() =>
  !!props.form.memory_provider_id && props.form.memory_provider_id === props.persistedMemoryProviderID,
)
const showBuiltinIndexedMemoryStatus = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showMemoryProviderStatusCard = computed(() =>
  showBuiltinIndexedMemoryStatus.value || !!selectedMem0MemoryProvider.value,
)

const indexedMemoryStatusTitle = computed(() =>
  selectedMemoryProviderType.value === 'mem0'
    ? t('bots.settings.mem0StatusTitle')
    : selectedBuiltinMemoryMode.value === 'dense'
    ? t('bots.settings.denseStatusTitle')
    : t('bots.settings.sparseStatusTitle'),
)

const statusCardData = computed(() => props.memoryStatus)
const showQdrantDetails = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showEncoderHealth = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showQdrantHealth = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const encoderHealthLabel = computed(() =>
  selectedBuiltinMemoryMode.value === 'dense'
    ? t('bots.settings.memoryDenseEmbeddingHealth')
    : t('bots.settings.memoryEncoderHealth'),
)

function healthTextClass(ok: boolean | undefined) {
  return ok ? 'text-foreground' : 'text-destructive'
}

function healthLabel(ok: boolean | undefined, error?: string) {
  if (ok) return t('bots.settings.memoryHealthOk')
  if (error) return error
  return t('bots.settings.memoryHealthUnavailable')
}
</script>
