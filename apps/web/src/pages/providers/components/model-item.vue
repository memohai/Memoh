<template>
  <!-- A flush list row, not a nested card: name on the left, actions on the
       right, separated from the next model by an inset hairline (the last row
       drops it). Same row language as the Configuration card above. -->
  <div
    class="mx-4 flex min-h-[3.25rem] items-center justify-between gap-3 border-b border-border py-2.5 last:border-b-0"
  >
    <!-- A hair of left padding so the name doesn't sit dead-flush on the row's
         inset edge when the search box is above it — just breathing room, not a
         full align to the placeholder text. -->
    <div
      class="min-w-0 flex-1"
      :class="{ 'pl-1': searchAligned }"
    >
      <div class="flex min-w-0 items-center gap-2">
        <span class="truncate text-[13px] font-medium text-foreground">
          {{ model.name || model.model_id }}
        </span>
        <Badge
          v-if="model.type === 'embedding'"
          variant="outline"
          size="sm"
          class="inline-flex shrink-0 items-center gap-1"
        >
          <Binary class="size-3" />
          {{ model.type }}
        </Badge>
        <span
          v-if="testResult"
          class="inline-flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground"
        >
          <span
            class="inline-block size-2 rounded-full"
            :class="statusDotClass"
          />
          <span v-if="testResult.latency_ms">{{ testResult.latency_ms }}ms</span>
        </span>
        <Spinner
          v-if="testLoading"
          class="size-3.5 shrink-0"
        />
      </div>
      <!-- Second line only when it carries new info: the real id (when a custom
           display name is hiding it) or a test error. When the name already is
           the id, repeating it is pure noise, so the line is dropped. -->
      <div
        v-if="showModelId || showError"
        class="mt-0.5 flex flex-wrap items-center gap-2"
      >
        <span
          v-if="showModelId"
          class="truncate text-xs text-muted-foreground"
        >
          {{ model.model_id }}
        </span>
        <span
          v-if="showError"
          class="text-xs text-destructive"
        >
          {{ testResult?.message }}
        </span>
      </div>
    </div>

    <div class="flex shrink-0 items-center gap-0.5">
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        :disabled="testLoading"
        :aria-label="$t('models.testModel')"
        @click="runTest"
      >
        <Zap class="size-4" />
      </Button>

      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        :aria-label="$t('common.edit')"
        @click="$emit('edit', model)"
      >
        <Settings class="size-4" />
      </Button>

      <ConfirmPopover
        :message="$t('models.deleteModelConfirm')"
        :loading="deleteLoading"
        @confirm="$emit('delete', model.id ?? '')"
      >
        <template #trigger>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            :aria-label="$t('common.delete')"
          >
            <Trash2 class="size-4" />
          </Button>
        </template>
      </ConfirmPopover>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
  Button,
  Spinner,
} from '@memohai/ui'
import { Zap, Settings, Trash2, Binary } from 'lucide-vue-next'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { postModelsByIdTest } from '@memohai/sdk'
import type { ModelsGetResponse, ModelsTestResponse } from '@memohai/sdk'
import { ref, computed } from 'vue'

const props = withDefaults(defineProps<{
  model: ModelsGetResponse
  deleteLoading: boolean
  searchAligned?: boolean
}>(), {
  searchAligned: false,
})

defineEmits<{
  edit: [model: ModelsGetResponse]
  delete: [id: string]
}>()

const testLoading = ref(false)
const testResult = ref<ModelsTestResponse | null>(null)

// Show the id as a second line only when a real custom name is hiding it.
const showModelId = computed(() => {
  const name = props.model.name?.trim()
  return !!name && !!props.model.model_id && name !== props.model.model_id
})

const showError = computed(
  () => !!(testResult.value && testResult.value.status !== 'ok' && testResult.value.message),
)

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
