<template>
  <div class="grid gap-4 md:grid-cols-2">
    <div class="space-y-2 md:col-span-2">
      <Label for="jina-reader-api-key">API Key</Label>
      <Input
        id="jina-reader-api-key"
        v-model="localConfig.api_key"
        type="password"
        aria-label="API Key"
      />
    </div>
    <div class="space-y-2 md:col-span-2">
      <Label for="jina-reader-base-url">Base URL</Label>
      <Input
        id="jina-reader-base-url"
        v-model="localConfig.base_url"
        aria-label="Base URL"
      />
    </div>
    <div class="space-y-2">
      <Label for="jina-reader-timeout-seconds">Timeout (seconds)</Label>
      <Input
        id="jina-reader-timeout-seconds"
        v-model.number="localConfig.timeout_seconds"
        type="number"
        :min="1"
        aria-label="Timeout (seconds)"
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
  base_url: 'https://r.jina.ai/',
  timeout_seconds: 30,
})

watch(
  () => props.modelValue,
  (val) => {
    localConfig.api_key = String(val?.api_key ?? '')
    localConfig.base_url = String(val?.base_url ?? 'https://r.jina.ai/')
    const timeout = Number(val?.timeout_seconds ?? 30)
    localConfig.timeout_seconds = Number.isFinite(timeout) && timeout > 0 ? timeout : 30
  },
  { immediate: true, deep: true },
)

watch(localConfig, () => {
  emit('update:modelValue', {
    api_key: localConfig.api_key,
    base_url: localConfig.base_url,
    timeout_seconds: localConfig.timeout_seconds,
  })
}, { deep: true })
</script>
