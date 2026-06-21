<template>
  <SettingsSection>
    <SettingsRow
      :label="$t('memory.graphTitle')"
      :description="$t('memory.graphDescription')"
      stack="sm"
      align="start"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphNodes')"
      :description="String(graphStatus.source_count ?? 0)"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphEdges')"
      :description="String(graphStatus.indexed_count ?? 0)"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphFiles')"
      :description="String(graphStatus.markdown_file_count ?? 0)"
    />
  </SettingsSection>
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
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'

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
const hasChanges = computed(() => {
  const config = (props.provider?.config ?? {}) as Record<string, unknown>
  return !props.provider?.id || config.memory_mode !== 'graph'
})

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

defineExpose({ hasChanges, saveLoading, save: handleSave })
</script>
