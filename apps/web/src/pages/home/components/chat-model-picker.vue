<template>
  <!-- The flyout anchors to the selected row's right edge, then shifts upward so
       the Reasoning label aligns with the Options label itself. -->
  <Popover v-model:open="optionsOpen">
    <div class="flex flex-col">
      <!-- Search: no leading glyph — the placeholder already says what to do,
           and a magnifier on a 1-row field is decoration that eats width. -->
      <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border/40 px-3">
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

      <!-- This list scrolls, but its scrollbar must not reserve a layout gutter:
           the row highlight and trailing controls need the full menu width. -->
      <div class="composer-model-list max-h-80 space-y-px overflow-y-auto overscroll-contain p-1">
        <div
          v-if="filteredGroups.length === 0"
          class="py-6 text-center text-body text-muted-foreground"
        >
          {{ $t('bots.settings.noModel') }}
        </div>

        <template
          v-for="group in filteredGroups"
          :key="group.key"
        >
          <div class="px-3 pt-1.5 pb-0.5 text-caption font-medium text-muted-foreground">
            {{ group.label }}
          </div>
          <div
            v-for="option in group.items"
            :key="option.value"
            class="group/row relative flex items-center gap-1 rounded-md px-1 transition-colors duration-75"
            :class="modelValue === option.value ? 'bg-[var(--overlay-hover)]' : 'hover:bg-[var(--overlay-hover-light)]'"
          >
            <PopoverAnchor
              v-if="optionsForValue === option.value"
              as-child
            >
              <span
                class="pointer-events-none absolute right-0 top-0 h-full w-0"
                aria-hidden="true"
              />
            </PopoverAnchor>
            <button
              type="button"
              class="flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left text-control"
              :class="modelValue === option.value ? 'font-medium text-foreground' : 'text-foreground'"
              @click="commitModel(option.value)"
            >
              <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
            </button>

            <div class="flex shrink-0 items-center gap-1 pr-1">
              <!-- Options surfaces on hover for ANY reasoning-capable model so
                     its reasoning support is discoverable before it's picked;
                     clicking it adopts that model and opens its effort card. -->
              <button
                v-if="supportsReasoning(option)"
                type="button"
                class="rounded px-1 py-0.5 text-control transition-[color,opacity]"
                :class="(modelValue === option.value || optionsForValue === option.value)
                  ? 'text-foreground opacity-100'
                  : 'text-muted-foreground opacity-0 group-hover/row:opacity-100 hover:text-foreground'"
                @click="toggleOptions(option.value)"
              >
                {{ $t('chat.modelOptions') }}
              </button>
              <Check
                v-if="modelValue === option.value"
                class="size-3.5 shrink-0 text-muted-foreground"
              />
            </div>
          </div>
        </template>
      </div>
    </div>

    <PopoverContent
      side="right"
      align="start"
      :side-offset="12"
      :align-offset="-4"
      :align-flip="false"
      :collision-padding="8"
      class="w-44 p-1"
      @open-auto-focus.prevent
    >
      <div class="flex flex-col gap-0.5">
        <!-- Whole row toggles reasoning (the switch is just the indicator), so
             the target is the full strip rather than a 32px puck. -->
        <div
          role="button"
          tabindex="0"
          class="flex cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-[var(--overlay-hover-light)]"
          @click="toggleReasoning(!reasoningActive)"
          @keydown.enter.prevent="toggleReasoning(!reasoningActive)"
          @keydown.space.prevent="toggleReasoning(!reasoningActive)"
        >
          <span class="text-control text-foreground">{{ $t('chat.reasoningEffort') }}</span>
          <Switch
            size="sm"
            tabindex="-1"
            class="pointer-events-none"
            :model-value="reasoningActive"
          />
        </div>

        <template v-if="reasoningActive && effortLevels.length">
          <div class="mx-1 my-1 h-px bg-border/60" />
          <div class="px-2 pb-0.5 text-caption font-medium text-muted-foreground">
            {{ $t('chat.modelEffort') }}
          </div>
          <button
            v-for="level in effortLevels"
            :key="level"
            type="button"
            class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-control transition-colors hover:bg-[var(--overlay-hover-light)]"
            :class="reasoningEffort === level ? 'font-medium text-foreground' : 'text-foreground'"
            @click="setEffort(level)"
          >
            <span>{{ $t(EFFORT_LABELS[level] ?? 'chat.reasoningOff') }}</span>
            <Check
              v-if="reasoningEffort === level"
              class="size-3.5 shrink-0 text-muted-foreground"
            />
          </button>
        </template>
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { X, Check } from 'lucide-vue-next'
import { Switch, Popover, PopoverAnchor, PopoverContent } from '@memohai/ui'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'
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
  close: []
}>()

const modelValue = defineModel<string>({ default: '' })
const reasoningEffort = defineModel<string>('reasoningEffort', { default: '' })

const searchTerm = ref('')
const optionsOpen = ref(false)
// Which row owns the reasoning fly-out (drives both its anchor and visibility).
const optionsForValue = ref('')

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
  config: ModelsGetResponse['config']
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
      config: model.config,
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

  // Float the active model's provider (and the model itself) to the top so the
  // current selection is the first thing the eye lands on.
  const list = Array.from(groups.values())
  const selected = options.value.find((o) => o.value === modelValue.value)
  if (!selected) return list
  list.sort((a, b) => Number(b.key === selected.groupKey) - Number(a.key === selected.groupKey))
  const activeGroup = list.find((g) => g.key === selected.groupKey)
  if (activeGroup) {
    activeGroup.items = [...activeGroup.items].sort(
      (a, b) => Number(b.value === selected.value) - Number(a.value === selected.value),
    )
  }
  return list
})

const activeModel = computed(() =>
  options.value.find((o) => o.value === modelValue.value),
)

function supportsReasoning(option: ModelOption): boolean {
  return resolveThinkingMode(option.config) !== 'none'
}

const activeClientType = computed(() =>
  props.providers.find((p) => p.id === activeModel.value?.providerId)?.client_type,
)

const availableEfforts = computed(() => {
  if (!activeModel.value) return []
  return availableEffortsForMode(
    resolveThinkingMode(activeModel.value.config),
    resolveEffortLevels(activeModel.value.config, activeClientType.value),
  )
})

// The selectable effort tiers, dropping the "off" sentinel — that toggle lives
// in the Reasoning switch above the list.
const effortLevels = computed(() =>
  availableEfforts.value.filter((e) => e !== REASONING_EFFORT_DISABLE),
)

const reasoningActive = computed(() =>
  Boolean(reasoningEffort.value)
  && reasoningEffort.value !== REASONING_EFFORT_DISABLE
  && availableEfforts.value.length > 0,
)

// Picking a model by its name commits the choice and dismisses the menu.
function commitModel(value: string) {
  optionsOpen.value = false
  if (value !== modelValue.value) modelValue.value = value
  emit('close')
}

// Opening Options adopts the model (so the effort context is unambiguous) but
// keeps the menu open so the fly-out can render against its row.
function toggleOptions(value: string) {
  const reopening = optionsForValue.value === value && optionsOpen.value
  if (value !== modelValue.value) modelValue.value = value
  optionsForValue.value = value
  optionsOpen.value = !reopening
}

function toggleReasoning(next: boolean) {
  if (next) {
    const levels = effortLevels.value
    reasoningEffort.value = levels.includes('medium') ? 'medium' : (levels[0] ?? REASONING_EFFORT_DISABLE)
  } else {
    reasoningEffort.value = REASONING_EFFORT_DISABLE
  }
}

function setEffort(level: string) {
  reasoningEffort.value = level
}

watch(() => props.open, (v) => {
  if (v) {
    searchTerm.value = ''
  } else {
    optionsOpen.value = false
    optionsForValue.value = ''
  }
})

// Filtering can unmount the row the fly-out is anchored to; drop it so the card
// never floats against a missing anchor.
watch(searchTerm, () => {
  if (optionsOpen.value) {
    optionsOpen.value = false
    optionsForValue.value = ''
  }
})

// When the fly-out dismisses (toggle, outside-click), forget its row so the
// "Options" affordance falls back to hover/selection visibility.
watch(optionsOpen, (v) => {
  if (!v) optionsForValue.value = ''
})
</script>
