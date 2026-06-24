<template>
  <div class="flex h-10 shrink-0 items-center gap-2 border-b border-border/40 px-3.5">
    <input
      v-model="searchTerm"
      role="combobox"
      :aria-controls="listboxId"
      :aria-expanded="open"
      :aria-activedescendant="activeIndex >= 0 ? `${listboxId}-${activeIndex}` : undefined"
      :placeholder="$t('bots.settings.searchModel')"
      aria-label="Search models"
      class="flex h-full w-full bg-transparent text-control outline-hidden placeholder:text-muted-foreground"
      @keydown="onKeydown"
    >
  </div>

  <div
    :id="listboxId"
    ref="scrollEl"
    class="max-h-64 overflow-y-auto px-1"
    role="listbox"
  >
    <div
      v-if="rows.length === 0"
      class="py-6 text-center text-xs text-muted-foreground"
    >
      {{ $t('bots.settings.noModel') }}
    </div>

    <div
      v-else
      :style="{ height: `${totalSize}px`, width: '100%', position: 'relative' }"
    >
      <div
        v-for="vRow in virtualRows"
        :key="vRow.key"
        :ref="measureRow"
        :data-index="vRow.virtual.index"
        class="py-0.5"
        :style="{ position: 'absolute', top: '0', left: '0', width: '100%', transform: `translateY(${vRow.virtual.start}px)` }"
      >
        <div
          v-if="vRow.row.type === 'header'"
          class="px-2 py-1.5 text-xs font-medium text-muted-foreground"
        >
          {{ vRow.row.label }}
        </div>

        <button
          v-else
          :id="`${listboxId}-${vRow.virtual.index}`"
          type="button"
          role="option"
          :aria-selected="modelValue === vRow.row.option.value"
          :aria-setsize="optionCount"
          :aria-posinset="vRow.row.posinset"
          :data-highlighted="activeIndex === vRow.virtual.index ? '' : undefined"
          :class="[menuItemClass, 'h-8']"
          @click="$emit('update:modelValue', vRow.row.option.value)"
          @pointermove="activeIndex = vRow.virtual.index"
        >
          <span
            class="min-w-0 flex-1 truncate text-left"
            :title="vRow.row.option.label"
          >{{ vRow.row.option.label }}</span>
          <Check
            v-if="modelValue === vRow.row.option.value"
            class="ml-2 size-4 shrink-0"
          />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { Check } from 'lucide-vue-next'
import { menuItemClass } from '@memohai/ui'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
import { useListboxKeyboard } from '@/composables/useListboxKeyboard'

export interface ModelOption {
  value: string
  label: string
  description?: string
  groupKey: string
  groupLabel: string
  keywords: string[]
  compatibilities?: string[]
  contextWindow?: number
}

interface HeaderRow {
  type: 'header'
  key: string
  label: string
}

interface ItemRow {
  type: 'item'
  key: string
  option: ModelOption
  // 1-based position among option rows (excludes headers), for aria-posinset
  posinset: number
}

type Row = HeaderRow | ItemRow

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType: 'chat' | 'embedding' | 'video'
  open?: boolean
  showTags?: boolean
  showIcons?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const modelValue = defineModel<string>({ default: '' })

const searchTerm = ref('')
const scrollEl = ref<HTMLElement | null>(null)

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

const options = computed<ModelOption[]>(() =>
  typeFilteredModels.value.map((model) => {
    const providerId = model.provider_id ?? ''
    const config = model.config as { compatibilities?: string[]; context_window?: number } | undefined
    return {
      value: model.id || model.model_id || '',
      label: model.name || model.model_id || '',
      description: model.name ? model.model_id : undefined,
      groupKey: providerId,
      groupLabel: providerMap.value.get(providerId) ?? providerId,
      keywords: [model.model_id ?? '', model.name ?? ''],
      compatibilities: config?.compatibilities,
      contextWindow: config?.context_window,
    }
  }),
)

const filteredOptions = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase()
  if (!keyword) return options.value
  return options.value.filter((opt) => {
    const terms = [opt.label, opt.description, ...opt.keywords]
      .filter((t): t is string => Boolean(t))
      .join(' ')
      .toLowerCase()
    return terms.includes(keyword)
  })
})

const filteredGroups = computed(() => {
  const groups = new Map<string, { key: string; label: string; items: ModelOption[] }>()
  for (const opt of filteredOptions.value) {
    if (!groups.has(opt.groupKey)) {
      groups.set(opt.groupKey, { key: opt.groupKey, label: opt.groupLabel, items: [] })
    }
    groups.get(opt.groupKey)!.items.push(opt)
  }
  return Array.from(groups.values())
})

// Flatten the grouped options into one linear list so the dropdown can be
// virtualized: rendering every row at once instantiates hundreds of buttons
// (each with capability-icon sub-components) synchronously and freezes the
// main thread when a provider exposes hundreds of models. Heights are measured
// at runtime by the virtualizer, so no row heights are hard-coded here.
const rows = computed<Row[]>(() => {
  const result: Row[] = []
  let posinset = 0
  for (const group of filteredGroups.value) {
    if (group.label) {
      result.push({ type: 'header', key: `header:${group.key}`, label: group.label })
    }
    for (const option of group.items) {
      posinset += 1
      result.push({
          type: 'item',
          key: option.value,
          option,
          posinset,
        })
    }
  }
  return result
})

// Total option count (excludes group headers) for aria-setsize: virtualization
// drops off-screen options from the DOM, so screen readers need this to know the
// real set size rather than only the rendered window.
const optionCount = computed(() => filteredOptions.value.length)

const virtualizer = useVirtualizer<HTMLElement, HTMLElement>(
  computed(() => ({
    count: rows.value.length,
    getScrollElement: () => scrollEl.value,
    // Per-row size estimate, kept close to the real rendered heights so the
    // total scroll size barely shifts as rows get measured — otherwise the
    // scrollbar drifts (estimate vs. measured mismatch). These are only seeds:
    // measureRow measures the true height at runtime, so being slightly off
    // causes minor jitter at worst, never clipping/misalignment.
    estimateSize: (index) => {
      const row = rows.value[index]
      if (!row) return 36
      return row.type === 'header' ? 32 : 36
    },
    overscan: 8,
    getItemKey: (index: number) => rows.value[index]?.key ?? index,
  })),
)

const totalSize = computed(() => virtualizer.value.getTotalSize())

const virtualRows = computed(() =>
  virtualizer.value.getVirtualItems().flatMap((vi) => {
    const row = rows.value[vi.index]
    return row ? [{ key: String(vi.key), virtual: vi, row }] : []
  }),
)

const measureRow = (el: unknown) => {
  if (el instanceof HTMLElement) virtualizer.value.measureElement(el)
}

const listboxId = useId()
const { activeIndex, onKeydown, reset: resetActive } = useListboxKeyboard<Row>({
  rows,
  scrollToIndex: (index) => virtualizer.value.scrollToIndex(index),
  onSelect: (row) => {
    if (row.type === 'item') emit('update:modelValue', row.option.value)
  },
})

watch(() => props.open, (v) => {
  if (v) {
    searchTerm.value = ''
    resetActive()
    nextTick(() => virtualizer.value.scrollToOffset(0))
  }
})

watch(searchTerm, () => virtualizer.value.scrollToOffset(0))
</script>
