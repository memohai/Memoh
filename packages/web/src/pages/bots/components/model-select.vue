<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        class="w-full justify-between font-normal"
      >
        <span class="truncate">
          {{ displayLabel || placeholder }}
        </span>
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
          class="ml-2 size-3.5 shrink-0 text-muted-foreground"
        />
      </Button>
    </PopoverTrigger>
    <PopoverContent
      class="w-[--reka-popover-trigger-width] p-0"
      align="start"
    >
      <!-- Search input -->
      <div class="flex items-center border-b px-3">
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
          class="mr-2 size-3.5 shrink-0 text-muted-foreground"
        />
        <input
          v-model="searchTerm"
          :placeholder="$t('bots.settings.searchModel')"
          class="flex h-10 w-full bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground"
        >
      </div>

      <!-- Model list -->
      <ScrollArea class="max-h-64">
        <div
          v-if="filteredGroups.length === 0"
          class="py-6 text-center text-sm text-muted-foreground"
        >
          {{ $t('bots.settings.noModel') }}
        </div>

        <div
          v-for="group in filteredGroups"
          :key="group.providerName"
          class="p-1"
        >
          <div class="px-2 py-1.5 text-xs font-medium text-muted-foreground">
            {{ group.providerName }}
          </div>
          <button
            v-for="model in group.models"
            :key="model.model_id"
            class="relative flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
            :class="{ 'bg-accent': selected === model.model_id }"
            @click="selectModel(model.model_id)"
          >
            <FontAwesomeIcon
              v-if="selected === model.model_id"
              :icon="['fas', 'check']"
              class="size-3.5"
            />
            <span
              v-else
              class="size-3.5"
            />
            <span class="truncate">{{ model.name || model.model_id }}</span>
            <span
              v-if="model.name"
              class="ml-auto text-xs text-muted-foreground"
            >
              {{ model.model_id }}
            </span>
          </button>
        </div>
      </ScrollArea>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  Button,
  ScrollArea,
} from '@memoh/ui'
import { computed, ref, watch } from 'vue'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memoh/sdk'

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType: 'chat' | 'embedding'
  placeholder?: string
}>()

const selected = defineModel<string>({ default: '' })
const searchTerm = ref('')
const open = ref(false)

// 打开时清空搜索
watch(open, (val) => {
  if (val) searchTerm.value = ''
})

const typeFilteredModels = computed(() =>
  props.models.filter((m) => m.type === props.modelType),
)

const providerMap = computed(() => {
  const map = new Map<string, string>()
  for (const p of props.providers) {
    map.set(p.id, p.name ?? p.id)
  }
  return map
})

// 搜索过滤后按 Provider 分组
const filteredGroups = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase()
  const models = keyword
    ? typeFilteredModels.value.filter(
      (m) =>
        m.model_id.toLowerCase().includes(keyword)
        || (m.name?.toLowerCase().includes(keyword) ?? false),
    )
    : typeFilteredModels.value

  const groups = new Map<string, { providerName: string; models: ModelsGetResponse[] }>()
  for (const model of models) {
    const pid = model.llm_provider_id
    const providerName = providerMap.value.get(pid) ?? pid
    if (!groups.has(pid)) {
      groups.set(pid, { providerName, models: [] })
    }
    groups.get(pid)!.models.push(model)
  }
  return Array.from(groups.values())
})

// 显示选中模型的名称
const displayLabel = computed(() => {
  if (!selected.value) return ''
  const model = typeFilteredModels.value.find((m) => m.model_id === selected.value)
  return model?.name || model?.model_id || selected.value
})

function selectModel(modelId: string) {
  selected.value = modelId
  open.value = false
}
</script>
