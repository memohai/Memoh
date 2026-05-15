<template>
  <div class="grid gap-4 md:grid-cols-2">
    <div class="space-y-2 md:col-span-2">
      <Label for="yandex-api-key">{{ $t('provider.apiKey') }}</Label>
      <Input
        id="yandex-api-key"
        v-model="localConfig.api_key"
        type="password"
        :aria-label="$t('provider.apiKey')"
      />
    </div>
    <div class="space-y-2">
      <Label for="yandex-search-type">{{ $t('common.searchType') }}</Label>
      <Input
        id="yandex-search-type"
        v-model="localConfig.search_type"
        :aria-label="$t('common.searchType')"
        placeholder="SEARCH_TYPE_RU"
      />
    </div>
    <div class="space-y-2">
      <Label for="yandex-timeout-seconds">{{ $t('common.timeoutSeconds') }}</Label>
      <Input
        id="yandex-timeout-seconds"
        v-model.number="localConfig.timeout_seconds"
        type="number"
        :min="1"
        :aria-label="$t('common.timeoutSeconds')"
      />
    </div>
    <div class="space-y-2 md:col-span-2">
      <Label for="yandex-base-url">{{ $t('common.baseUrl') }}</Label>
      <Input
        id="yandex-base-url"
        v-model="localConfig.base_url"
        :aria-label="$t('common.baseUrl')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import { Input, Label } from '@memohai/ui'

const props = defineProps<{
  modelValue: Record<string, unknown>
}>()

const emit = defineEmits<{
  'update:modelValue': [value: Record<string, unknown>]
}>()

const localConfig = reactive({
  api_key: '',
  search_type: 'SEARCH_TYPE_RU',
  base_url: 'https://searchapi.api.cloud.yandex.net/v2/web/search',
  timeout_seconds: 15,
})

watch(
  () => props.modelValue,
  (val) => {
    localConfig.api_key = String(val?.api_key ?? '')
    localConfig.search_type = String(val?.search_type ?? 'SEARCH_TYPE_RU')
    localConfig.base_url = String(val?.base_url ?? 'https://searchapi.api.cloud.yandex.net/v2/web/search')
    const timeout = Number(val?.timeout_seconds ?? 15)
    localConfig.timeout_seconds = Number.isFinite(timeout) && timeout > 0 ? timeout : 15
  },
  { immediate: true, deep: true },
)

watch(localConfig, () => {
  emit('update:modelValue', {
    api_key: localConfig.api_key,
    search_type: localConfig.search_type,
    base_url: localConfig.base_url,
    timeout_seconds: localConfig.timeout_seconds,
  })
}, { deep: true })
</script>
