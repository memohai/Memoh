<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select model'"
        class="w-full justify-between font-normal"
      >
        <span class="truncate">
          {{ displayLabel || placeholder }}
        </span>
        <Search
          class="ml-2 size-3.5 shrink-0 text-muted-foreground"
        />
      </Button>
    </PopoverTrigger>
    <PopoverContent
      class="w-[--reka-popover-trigger-width] p-0"
      align="start"
    >
      <ModelOptions
        v-model="selected"
        :models="models"
        :providers="providers"
        :model-type="modelType"
        :open="open"
      />
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search } from 'lucide-vue-next'
import { Popover, PopoverTrigger, PopoverContent, Button } from '@memohai/ui'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import ModelOptions from './model-options.vue'

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType?: 'chat' | 'embedding' | 'image'
  placeholder?: string
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
