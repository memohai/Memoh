<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="displayLabel ? undefined : ''"
        type="button"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select model'"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span
          class="line-clamp-1"
          :title="displayLabel || placeholder"
        >
          {{ displayLabel || placeholder }}
        </span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </PopoverTrigger>
    <PopoverContent
      menu
      align="end"
      class="min-w-[var(--reka-popover-trigger-width)] w-80 p-0"
    >
      <div :class="menuChromeClass">
        <ModelOptions
          v-model="selected"
          :models="models"
          :providers="providers"
          :model-type="modelType"
          :open="open"
          :none-label="noneLabel"
        />
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronsUpDown } from 'lucide-vue-next'
import { Popover, PopoverTrigger, PopoverContent, menuChromeClass, selectTriggerClass } from '@memohai/ui'
import type { ModelsGetResponse, ModelsModelType, ProvidersGetResponse } from '@memohai/sdk'
import ModelOptions from './model-options.vue'

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType: ModelsModelType
  placeholder?: string
  noneLabel?: string
}>()

const selected = defineModel<string>({ default: '' })
const open = ref(false)

watch(selected, () => {
  open.value = false
})

const displayLabel = computed(() => {
  const model = props.models.find((m) => (m.id || m.model_id) === selected.value)
  return model?.name || model?.model_id || selected.value
})
</script>
