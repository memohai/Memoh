<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select TTS model'"
    :search-placeholder="$t('speech.searchPlaceholder')"
    search-aria-label="Search TTS models"
    :empty-text="$t('speech.emptyTitle')"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select TTS model'"
        class="w-full justify-between font-normal text-xs shadow-none h-9"
      >
        <span class="flex min-w-0 items-center gap-2 truncate">
          <Volume2
            v-if="selected"
            class="size-3.5 shrink-0 text-muted-foreground"
          />
          <span
            class="truncate"
            :title="displayLabel || placeholder"
          >{{ displayLabel || placeholder }}</span>
        </span>
        <Search
          class="ml-2 size-3.5 shrink-0 text-muted-foreground"
        />
      </Button>
    </template>

    <template #option-icon="{ option }">
      <Volume2
        v-if="option.value && showIcons"
        class="size-3.5 shrink-0 text-muted-foreground"
      />
    </template>

    <template #option-label="{ option }">
      <span
        class="truncate flex-1 text-left"
        :class="{ 'text-muted-foreground': !option.value }"
        :title="option.label"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { Volume2, Search } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

export interface TtsModelOption {
  id: string
  model_id: string
  name: string
  provider_id: string
  provider_type?: string
}

export interface TtsProviderOption {
  id: string
  name: string
  client_type: string
}

const props = defineProps<{
  models: TtsModelOption[]
  providers: TtsProviderOption[]
  placeholder?: string
  showIcons?: boolean
}>()
const { t } = useI18n()

const selected = defineModel<string>({ default: '' })

const providerMap = computed(() => {
  const map = new Map<string, string>()
  for (const p of props.providers) {
    map.set(p.id, p.name ?? p.id)
  }
  return map
})

const options = computed<SearchableSelectOption[]>(() => {
  const noneOption: SearchableSelectOption = {
    value: '',
    label: t('common.none'),
    keywords: [t('common.none')],
  }
  const modelOptions = props.models.map((model) => ({
    value: model.id || '',
    label: model.name || model.model_id || '',
    description: model.model_id,
    group: model.provider_id,
    groupLabel: providerMap.value.get(model.provider_id) ?? model.provider_id,
    keywords: [model.name ?? '', model.model_id ?? '', model.provider_type ?? ''],
  }))
  return [noneOption, ...modelOptions]
})
</script>
