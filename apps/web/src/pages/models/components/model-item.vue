<template>
  <Item variant="outline">
    <ItemContent>
      <ItemTitle class="flex items-center gap-2">
        {{ model.name || model.model_id }}
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
      <ItemDescription class="gap-2 flex flex-wrap items-center mt-3">
        <Badge variant="outline">
          {{ model.type }}
        </Badge>
        <Badge
          v-for="cap in (model.config?.compatibilities || [])"
          :key="cap"
          variant="secondary"
          class="text-xs"
        >
          {{ $t(`models.compatibility.${cap}`, cap) }}
        </Badge>
        <Badge
          v-for="effort in reasoningEfforts"
          :key="effort"
          variant="secondary"
          class="text-xs"
        >
          {{ effort }}
        </Badge>
        <span
          v-if="model.config?.context_window"
          class="text-xs text-muted-foreground"
        >
          ctx: {{ model.config.context_window.toLocaleString() }}
        </span>
        <span
          v-if="testResult && testResult.status !== 'ok' && testResult.message"
          class="text-destructive text-xs"
        >
          {{ testResult.message }}
        </span>
      </ItemDescription>
    </ItemContent>
    <ItemActions>
      <Button
        type="button"
        variant="outline"
        class="cursor-pointer"
        :disabled="testLoading"
        :aria-label="$t('models.testModel')"
        @click="runTest"
      >
        <FontAwesomeIcon :icon="['fas', 'rotate']" />
      </Button>

      <Button
        type="button"
        variant="outline"
        class="cursor-pointer"
        :aria-label="$t('common.edit')"
        @click="$emit('edit', model)"
      >
        <FontAwesomeIcon :icon="['fas', 'gear']" />
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
            <FontAwesomeIcon :icon="['far', 'trash-can']" />
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
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { postModelsByIdTest } from '@memohai/sdk'
import type { ModelsGetResponse, ModelsTestResponse } from '@memohai/sdk'
import { ref, computed } from 'vue'

type ModelConfigWithReasoning = {
  reasoning_efforts?: string[]
}

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
const reasoningEfforts = computed(() => ((props.model.config as ModelConfigWithReasoning | undefined)?.reasoning_efforts ?? []))

const statusDotClass = computed(() => {
  switch (testResult.value?.status) {
    case 'ok': return 'bg-green-500'
    case 'auth_error': return 'bg-yellow-500'
    case 'error': return 'bg-red-500'
    default: return 'bg-gray-400'
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
