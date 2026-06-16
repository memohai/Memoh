<template>
  <div class="flex flex-col">
    <!-- Search -->
    <div class="flex h-10 shrink-0 items-center gap-2 border-b border-border/40 px-3.5">
      <Search class="size-3.5 shrink-0 text-muted-foreground" />
      <input
        v-model="searchTerm"
        role="combobox"
        :placeholder="$t('bots.settings.searchModel')"
        aria-label="Search models"
        class="flex h-full w-full bg-transparent text-control outline-hidden placeholder:text-muted-foreground"
      >
      <button
        v-if="searchTerm"
        type="button"
        class="shrink-0 text-muted-foreground hover:text-foreground"
        :aria-label="$t('common.clear')"
        @click="searchTerm = ''"
      >
        <X class="size-3.5" />
      </button>
    </div>

    <!-- Model list -->
    <ScrollArea class="max-h-72">
      <div class="p-1">
        <div
          v-if="filteredGroups.length === 0"
          class="py-6 text-center text-xs text-muted-foreground"
        >
          {{ $t('bots.settings.noModel') }}
        </div>

        <template
          v-for="group in filteredGroups"
          :key="group.key"
        >
          <div class="px-2 pt-1.5 pb-1 text-xs font-medium text-muted-foreground">
            {{ group.label }}
          </div>
          <div
            v-for="option in group.items"
            :key="option.value"
            class="group/model flex items-center gap-1 rounded-md px-1 transition-colors"
            :class="modelValue === option.value ? 'bg-[var(--overlay-hover)]' : 'hover:bg-[var(--overlay-hover-light)]'"
          >
            <button
              type="button"
              class="flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm"
              :class="modelValue === option.value ? 'font-medium text-foreground' : 'text-foreground'"
              @click="selectModel(option.value)"
            >
              <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
              <Check
                v-if="modelValue === option.value"
                class="size-3.5 shrink-0 text-muted-foreground"
              />
            </button>

            <!-- Inline reasoning effort (Arkloop pattern: on the RIGHT of model item) -->
            <button
              v-if="modelValue === option.value && supportsReasoning(option)"
              type="button"
              class="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
              :class="reasoningActive ? 'text-foreground' : ''"
              @click.stop="cycleReasoning"
            >
              <Lightbulb
                class="size-3 shrink-0"
                :class="reasoningActive ? '' : 'opacity-40'"
              />
              <span class="text-caption">{{ reasoningLabel }}</span>
            </button>
          </div>
        </template>
      </div>
    </ScrollArea>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search, X, Check, Lightbulb } from 'lucide-vue-next'
import { ScrollArea } from '@memohai/ui'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import {
  REASONING_EFFORT_DISABLE,
  EFFORT_LABELS,
  resolveThinkingMode,
  resolveEffortLevels,
  availableEffortsForMode,
} from '@/pages/bots/components/reasoning-effort'

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType: 'chat' | 'embedding'
  open?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:reasoningEffort': [value: string]
}>()

const modelValue = defineModel<string>({ default: '' })
const reasoningEffort = defineModel<string>('reasoningEffort', { default: '' })

const { t } = useI18n()
const searchTerm = ref('')

const providerMap = computed(() => {
  const map = new Map<string, string>()
  for (const p of props.providers) {
    if (p.id) map.set(p.id, p.name ?? p.id)
  }
  return map
})

const typeFilteredModels = computed(() =>
  props.models.filter((m) => m.type === props.modelType),
)

interface ModelOption {
  value: string
  label: string
  groupKey: string
  groupLabel: string
  config: Record<string, unknown> | undefined
  providerId: string
}

const options = computed<ModelOption[]>(() =>
  typeFilteredModels.value.map((model) => {
    const providerId = model.provider_id ?? ''
    return {
      value: model.id || model.model_id || '',
      label: model.name || model.model_id || '',
      groupKey: providerId,
      groupLabel: providerMap.value.get(providerId) ?? providerId,
      config: model.config as Record<string, unknown> | undefined,
      providerId,
    }
  }),
)

const filteredGroups = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase()
  const filtered = keyword
    ? options.value.filter((opt) =>
        [opt.label, opt.value].some((s) => s.toLowerCase().includes(keyword)),
      )
    : options.value

  const groups = new Map<string, { key: string; label: string; items: ModelOption[] }>()
  for (const opt of filtered) {
    if (!groups.has(opt.groupKey)) {
      groups.set(opt.groupKey, { key: opt.groupKey, label: opt.groupLabel, items: [] })
    }
    groups.get(opt.groupKey)!.items.push(opt)
  }
  return Array.from(groups.values())
})

const activeModel = computed(() =>
  options.value.find((o) => o.value === modelValue.value),
)

function supportsReasoning(option: ModelOption): boolean {
  return resolveThinkingMode(option.config as never) !== 'none'
}

const activeClientType = computed(() =>
  props.providers.find((p) => p.id === activeModel.value?.providerId)?.client_type,
)

const availableEfforts = computed(() => {
  if (!activeModel.value) return []
  return availableEffortsForMode(
    resolveThinkingMode(activeModel.value.config as never),
    resolveEffortLevels(activeModel.value.config as never, activeClientType.value),
  )
})

const reasoningActive = computed(() =>
  Boolean(reasoningEffort.value)
  && reasoningEffort.value !== REASONING_EFFORT_DISABLE
  && availableEfforts.value.length > 0,
)

const reasoningLabel = computed(() => {
  if (!reasoningActive.value) return t('chat.reasoningOff')
  return t(EFFORT_LABELS[reasoningEffort.value] ?? 'chat.reasoningOff')
})

function selectModel(value: string) {
  if (value !== modelValue.value) {
    emit('update:modelValue', value)
  }
}

function cycleReasoning() {
  if (availableEfforts.value.length === 0) return
  const currentIndex = availableEfforts.value.indexOf(reasoningEffort.value)
  // Cycle: off → first effort → ... → last effort → off
  const nextIndex = (currentIndex + 1) % (availableEfforts.value.length + 1)
  if (nextIndex === 0) {
    emit('update:reasoningEffort', availableEfforts.value[0]!)
  } else if (nextIndex >= availableEfforts.value.length) {
    emit('update:reasoningEffort', REASONING_EFFORT_DISABLE)
  } else {
    emit('update:reasoningEffort', availableEfforts.value[nextIndex]!)
  }
}

watch(() => props.open, (v) => {
  if (v) searchTerm.value = ''
})
</script>
