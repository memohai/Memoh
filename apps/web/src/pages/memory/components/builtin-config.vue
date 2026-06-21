<template>
  <div class="space-y-5">
    <div class="space-y-2">
      <div>
        <h2 class="text-sm font-medium text-foreground">
          {{ $t('memory.graphTitle') }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphDescription') }}
        </p>
      </div>
    </div>

    <div
      v-if="graphStatus"
      class="grid grid-cols-3 gap-3"
    >
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.source_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphNodes') }}
        </p>
      </div>
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.indexed_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphEdges') }}
        </p>
      </div>
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.markdown_file_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphFiles') }}
        </p>
      </div>
    </div>

    <div class="flex justify-end">
      <LoadingButton
        v-if="!provider?.id"
        :loading="saveLoading"
        @click="handleSave"
      >
        {{ $t('common.save') }}
      </LoadingButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { toast } from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  getMemoryProvidersByIdStatus,
  postMemoryProviders,
  putMemoryProvidersById,
} from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderStatusResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import LoadingButton from '@/components/loading-button/index.vue'

const props = defineProps<{
  provider?: AdaptersProviderGetResponse | null
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const saveLoading = ref(false)

const { data: statusData } = useQuery({
  key: () => ['memory-provider-status', props.provider?.id ?? ''],
  query: async () => {
    const id = props.provider?.id
    if (!id) return null
    const { data } = await getMemoryProvidersByIdStatus({ path: { id }, throwOnError: true })
    return data
  },
  enabled: () => !!props.provider?.id,
})

const graphStatus = computed(() => statusData.value as AdaptersProviderStatusResponse | null)

async function handleSave() {
  saveLoading.value = true
  try {
    const config = { memory_mode: 'graph' }
    if (props.provider?.id) {
      await putMemoryProvidersById({
        path: { id: props.provider.id },
        body: { name: props.provider.name ?? 'Built-in', config },
        throwOnError: true,
      })
    } else {
      await postMemoryProviders({
        body: { name: 'Built-in', provider: 'builtin', config },
        throwOnError: true,
      })
    }
    toast.success(t('memory.saveSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
    if (props.provider?.id) {
      queryCache.invalidateQueries({ key: ['memory-provider-status', props.provider.id] })
    }
  } catch (error) {
    console.error('Failed to save memory provider:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    saveLoading.value = false
  }
}
</script>
