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
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select exit node'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <CircleDot
            v-if="selected"
            class="size-3.5"
            :class="selectedOption?.online ? 'text-success' : 'text-muted-foreground'"
          />
          <span class="truncate">{{ displayLabel || placeholder }}</span>
        </span>
        <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </template>

    <template #option-icon="{ option }">
      <CircleDot
        v-if="option.value"
        class="size-3.5"
        :class="option.meta?.online ? 'text-success' : 'text-muted-foreground'"
      />
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { CircleDot, Search } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
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
