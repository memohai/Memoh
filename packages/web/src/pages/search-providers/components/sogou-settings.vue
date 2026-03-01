<template>
  <div class="space-y-4">
    <div class="space-y-2">
      <Label for="sogou-secret-id">Secret ID</Label>
      <Input
        id="sogou-secret-id"
        v-model="localConfig.secret_id"
        type="password"
        aria-label="Secret ID"
      />
    </div>
    <div class="space-y-2">
      <Label for="sogou-secret-key">Secret Key</Label>
      <Input
        id="sogou-secret-key"
        v-model="localConfig.secret_key"
        type="password"
        aria-label="Secret Key"
      />
    </div>
    <div class="space-y-2">
      <Label for="sogou-base-url">Base URL</Label>
      <Input
        id="sogou-base-url"
        v-model="localConfig.base_url"
        aria-label="Base URL"
        placeholder="wsa.tencentcloudapi.com"
      />
    </div>
    <div class="space-y-2">
      <Label for="sogou-timeout-seconds">Timeout (seconds)</Label>
      <Input
        id="sogou-timeout-seconds"
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
import { Input, Label } from '@memoh/ui'

const props = defineProps<{
  modelValue: Record<string, unknown>
}>()

const emit = defineEmits<{
  'update:modelValue': [value: Record<string, unknown>]
}>()

const localConfig = reactive({
  secret_id: '',
  secret_key: '',
  base_url: 'wsa.tencentcloudapi.com',
  timeout_seconds: 15,
})

watch(
  () => props.modelValue,
  (val) => {
    localConfig.secret_id = String(val?.secret_id ?? '')
    localConfig.secret_key = String(val?.secret_key ?? '')
    localConfig.base_url = String(val?.base_url ?? 'wsa.tencentcloudapi.com')
    const timeout = Number(val?.timeout_seconds ?? 15)
    localConfig.timeout_seconds = Number.isFinite(timeout) && timeout > 0 ? timeout : 15
  },
  { immediate: true, deep: true },
)

watch(localConfig, () => {
  emit('update:modelValue', {
    secret_id: localConfig.secret_id,
    secret_key: localConfig.secret_key,
    base_url: localConfig.base_url,
    timeout_seconds: localConfig.timeout_seconds,
  })
}, { deep: true })
</script>
