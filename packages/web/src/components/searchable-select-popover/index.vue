<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <slot
        name="trigger"
        :open="open"
        :display-label="displayLabel"
        :selected-option="selectedOption"
        :placeholder="placeholder"
      >
        <Button
          variant="outline"
          role="combobox"
          :aria-expanded="open"
          :aria-label="ariaLabel || placeholder"
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
      </slot>
    </PopoverTrigger>
    <PopoverContent
      class="w-[--reka-popover-trigger-width] p-0"
      align="start"
    >
      <div class="flex items-center border-b px-3">
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
          class="mr-2 size-3.5 shrink-0 text-muted-foreground"
        />
        <input
          v-model="searchTerm"
          :placeholder="searchPlaceholder"
          :aria-label="searchAriaLabel"
          class="flex h-10 w-full bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground"
        >
      </div>

      <ScrollArea
        class="max-h-64"
        role="listbox"
      >
        <div
          v-if="filteredGroups.length === 0"
          class="py-6 text-center text-sm text-muted-foreground"
        >
          {{ emptyText }}
        </div>

        <div
          v-for="group in filteredGroups"
          :key="group.key"
          class="p-1"
        >
          <div
            v-if="showGroupHeaders && group.label"
            class="px-2 py-1.5 text-xs font-medium text-muted-foreground"
          >
            <slot
              name="group-label"
              :group="group"
            >
              {{ group.label }}
            </slot>
          </div>

          <button
            v-for="option in group.items"
            :key="option.value"
            type="button"
            role="option"
            :aria-selected="selected === option.value"
            class="relative flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
            :class="{ 'bg-accent': selected === option.value }"
            @click="selectOption(option.value)"
          >
            <FontAwesomeIcon
              v-if="selected === option.value"
              :icon="['fas', 'check']"
              class="size-3.5"
            />
            <span
              v-else
              class="size-3.5"
            />
            <slot
              name="option-icon"
              :option="option"
            />
            <slot
              name="option-label"
              :option="option"
            >
              <span class="truncate">{{ option.label }}</span>
            </slot>
            <slot
              name="option-suffix"
              :option="option"
            >
              <span
                v-if="option.description"
                class="ml-auto text-xs text-muted-foreground"
              >
                {{ option.description }}
              </span>
            </slot>
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

export interface SearchableSelectOption {
  value: string
  label: string
  description?: string
  group?: string
  groupLabel?: string
  keywords?: string[]
  meta?: unknown
}

const props = withDefaults(defineProps<{
  options: SearchableSelectOption[]
  placeholder?: string
  ariaLabel?: string
  searchPlaceholder?: string
  searchAriaLabel?: string
  emptyText?: string
  showGroupHeaders?: boolean
}>(), {
  placeholder: '',
  ariaLabel: '',
  searchPlaceholder: 'Search...',
  searchAriaLabel: 'Search options',
  emptyText: 'No results.',
  showGroupHeaders: true,
})

const selected = defineModel<string>({ default: '' })
const searchTerm = ref('')
const open = ref(false)

watch(open, (value) => {
  if (value) {
    searchTerm.value = ''
  }
})

const selectedOption = computed(() =>
  props.options.find((option) => option.value === selected.value),
)

const displayLabel = computed(() =>
  selectedOption.value?.label ?? selected.value,
)

const filteredOptions = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase()
  if (!keyword) {
    return props.options
  }
  return props.options.filter((option) => {
    const terms = [option.label, option.description, ...(option.keywords ?? [])]
      .filter((term): term is string => Boolean(term))
      .join(' ')
      .toLowerCase()
    return terms.includes(keyword)
  })
})

const filteredGroups = computed(() => {
  const groups = new Map<string, { key: string, label: string, items: SearchableSelectOption[] }>()
  for (const option of filteredOptions.value) {
    const key = option.group ?? '__ungrouped__'
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        label: option.groupLabel ?? option.group ?? '',
        items: [],
      })
    }
    groups.get(key)!.items.push(option)
  }
  return Array.from(groups.values())
})

function selectOption(value: string) {
  selected.value = value
  open.value = false
}
</script>
