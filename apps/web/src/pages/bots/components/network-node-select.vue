<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select exit node'"
    :search-placeholder="$t('network.searchPlaceholder')"
    search-aria-label="Search exit nodes"
    :empty-text="$t('network.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="!selected ? '' : undefined"
        type="button"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select exit node'"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
          <CircleDot
            v-if="selected"
            class="size-3.5 shrink-0"
            :class="selectedOption?.online ? 'text-success' : 'text-muted-foreground'"
          />
          <span class="line-clamp-1">{{ displayLabel || placeholder }}</span>
        </span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </template>

    <template #option-icon="{ option }">
      <CircleDot
        v-if="option.value"
        class="size-3.5 shrink-0"
        :class="option.meta?.online ? 'text-success' : 'text-muted-foreground'"
      />
    </template>

    <template #option-label="{ option }">
      <span
        class="flex-1 truncate text-left"
        :class="{ 'text-muted-foreground': !option.value }"
        :title="option.label"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { CircleDot, ChevronsUpDown } from 'lucide-vue-next'
import { selectTriggerClass } from '@memohai/ui'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import type { NetworkNodeOption } from '@/pages/network/api'

const props = defineProps<{
  nodes: NetworkNodeOption[]
  placeholder?: string
}>()

const selected = defineModel<string>({ default: '' })
const { t } = useI18n()

const selectedOption = computed(() =>
  props.nodes.find(node => node.value === selected.value),
)

const options = computed<SearchableSelectOption[]>(() => {
  const noneOption: SearchableSelectOption = {
    value: '',
    label: t('common.none'),
    keywords: [t('common.none')],
  }
  const nodeOptions = props.nodes.map(node => {
    const addresses = node.addresses?.join(' ') ?? ''
    return {
      value: node.value || '',
      label: node.display_name || node.value || '',
      description: [node.description, addresses].filter(Boolean).join(' | '),
      keywords: [
        node.display_name ?? '',
        node.value ?? '',
        node.description ?? '',
        addresses,
      ],
      meta: {
        online: node.online ?? false,
      },
    } satisfies SearchableSelectOption
  })
  return [noneOption, ...nodeOptions]
})
</script>
