<template>
  <Item
    variant="outline"
    class="h-full flex-col items-stretch min-w-0"
  >
    <ItemContent class="w-full min-w-0">
      <ItemTitle class="flex items-center gap-2 w-full min-w-0">
        <span class="truncate min-w-0 flex-1">{{ model.name || model.model_id }}</span>
        <Badge
          v-if="model.type"
          variant="outline"
          size="sm"
          class="inline-flex items-center gap-1"
        >
          <component
            :is="typeIcon"
            class="size-3"
          />
          {{ model.type }}
        </Badge>
        <span
          v-if="testResult"
          class="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
        >
          <span
            class="inline-block size-2 rounded-full"
            :class="statusDotClass"
          />
          <span v-if="testResult.latency_ms">{{ testResult.latency_ms }}ms</span>
        </span>
        <Spinner
          v-if="testLoading"
          class="size-3.5"
        />
      </ItemTitle>
      <ItemDescription class="gap-2 flex flex-wrap items-center mt-3 min-h-6">
        <ModelCapabilities :compatibilities="model.config?.compatibilities || []" />
        <ContextWindowBadge :context-window="model.config?.context_window" />
        <span
          v-if="testResult && testResult.status !== 'ok' && testResult.message"
          class="text-destructive text-xs"
        >
          {{ testResult.message }}
        </span>
      </ItemDescription>
    </ItemContent>
    <ItemActions class="mt-auto w-full justify-end">
      <Button
        type="button"
        variant="outline"
        class="cursor-pointer"
        :disabled="testLoading"
        :aria-label="$t('models.testModel')"
        @click="runTest"
      >
        <RefreshCw />
      </Button>

      <Button
        type="button"
        variant="outline"
        class="cursor-pointer"
        :aria-label="$t('common.edit')"
        @click="$emit('edit', model)"
      >
        <Settings />
      </Button>

      <ConfirmPopover
        :message="$t('models.deleteModelConfirm')"
        :loading="deleteLoading"
        @confirm="$emit('delete', model.id ?? '')"
      >
        <template #trigger>
          <Button
            type="button"
            variant="outline"
            :aria-label="$t('common.delete')"
          >
            <Trash2 />
          </Button>
        </template>
      </ConfirmPopover>
    </ItemActions>
  </Item>
</template>

<script setup lang="ts">
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemActions,
  ItemTitle,
  Badge,
  Button,
  Spinner,
} from '@memohai/ui'
import { RefreshCw, Settings, Trash2, MessageSquare, Binary } from 'lucide-vue-next'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelCapabilities from '@/components/model-capabilities/index.vue'
import ContextWindowBadge from '@/components/context-window-badge/index.vue'
import { postModelsByIdTest } from '@memohai/sdk'
import type { ModelsGetResponse, ModelsTestResponse } from '@memohai/sdk'
import { ref, computed } from 'vue'

const props = defineProps<{
  model: ModelsGetResponse
  deleteLoading: boolean
}>()

defineEmits<{
  edit: [model: ModelsGetResponse]
  delete: [id: string]
}>()

const testLoading = ref(false)
const testResult = ref<ModelsTestResponse | null>(null)

const typeIcon = computed(() => {
  return props.model.type === 'embedding' ? Binary : MessageSquare
})

const statusDotClass = computed(() => {
  switch (testResult.value?.status) {
    case 'ok': return 'bg-success'
    case 'auth_error': return 'bg-warning'
    case 'error': return 'bg-destructive'
    default: return 'bg-muted-foreground'
  }
})

async function runTest() {
  if (!props.model.id) return
  testLoading.value = true
  testResult.value = null
  try {
    const { data } = await postModelsByIdTest({
      path: { id: props.model.id },
      throwOnError: true,
    })
    testResult.value = data ?? null
  } catch {
    testResult.value = { status: 'error' }
  } finally {
    testLoading.value = false
  }
}
</script>
