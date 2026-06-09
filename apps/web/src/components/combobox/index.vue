<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="selectedLabel ? undefined : ''"
        type="button"
        :aria-expanded="open"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="line-clamp-1">{{ selectedLabel || placeholder }}</span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </PopoverTrigger>

    <PopoverContent
      menu
      align="start"
      class="w-[var(--reka-popover-trigger-width)]"
    >
      <div class="flex flex-col overflow-hidden rounded-[var(--radius-menu-shell)] border border-[color:var(--border-menu)] bg-popover text-popover-foreground shadow-[var(--shadow-dropdown)]">
        <!-- Search (combobox-style: no leading magnifier, query lines up with rows) -->
        <div class="flex h-10 shrink-0 items-center gap-2 border-b border-border/40 px-3.5">
          <input
            v-model="search"
            role="combobox"
            :aria-controls="listboxId"
            :aria-expanded="open"
            :aria-activedescendant="activeIndex >= 0 ? `${listboxId}-${activeIndex}` : undefined"
            :placeholder="searchPlaceholder"
            :aria-label="searchPlaceholder"
            class="flex h-full w-full bg-transparent text-control outline-hidden placeholder:text-muted-foreground"
            @keydown="onKeydown"
          >
        </div>

        <!-- Virtualized result list -->
        <div
          :id="listboxId"
          ref="scrollEl"
          role="listbox"
          class="max-h-64 scroll-my-1 overflow-y-auto px-1"
        >
          <div
            v-if="filtered.length === 0"
            class="py-6 text-center text-control text-muted-foreground"
          >
            {{ emptyText }}
          </div>

          <div
            v-else
            :style="{ height: `${totalSize}px`, width: '100%', position: 'relative' }"
          >
            <div
              v-for="vRow in virtualRows"
              :key="vRow.key"
              :data-index="vRow.index"
              class="py-0.5"
              :style="{ position: 'absolute', top: '0', left: '0', width: '100%', transform: `translateY(${vRow.start}px)` }"
            >
              <button
                :id="`${listboxId}-${vRow.index}`"
                type="button"
                role="option"
                :aria-selected="modelValue === vRow.option.value"
                :data-highlighted="activeIndex === vRow.index ? '' : undefined"
                :class="[menuItemClass, 'h-8']"
                @click="select(vRow.option.value)"
                @pointermove="activeIndex = vRow.index"
              >
                <span class="min-w-0 flex-1 truncate text-left">{{ vRow.option.label }}</span>
                <span
                  v-if="vRow.option.description"
                  class="ml-auto shrink-0 text-xs text-muted-foreground"
                >
                  {{ vRow.option.description }}
                </span>
                <Check
                  v-if="modelValue === vRow.option.value"
                  class="ml-2 size-4 shrink-0"
                />
              </button>
            </div>
          </div>
        </div>
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { menuItemClass, Popover, PopoverContent, PopoverTrigger, selectTriggerClass } from '@memohai/ui'
import { Check, ChevronsUpDown } from 'lucide-vue-next'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useListboxKeyboard } from '@/composables/useListboxKeyboard'

export interface ComboboxOption {
  value: string
  label: string
  description?: string
  keywords?: string[]
}

const props = withDefaults(defineProps<{
  options: ComboboxOption[]
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
}>(), {
  placeholder: '',
  searchPlaceholder: 'Search...',
  emptyText: 'No results.',
})

const modelValue = defineModel<string>({ default: '' })

const open = ref(false)
const search = ref('')
const scrollEl = ref<HTMLElement | null>(null)
const listboxId = useId()

const selectedLabel = computed(() =>
  props.options.find((option) => option.value === modelValue.value)?.label ?? '',
)

const filtered = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return props.options
  return props.options.filter((option) => {
    const terms = [option.label, option.description, ...(option.keywords ?? [])]
      .filter((term): term is string => Boolean(term))
      .join(' ')
      .toLowerCase()
    return terms.includes(keyword)
  })
})

const virtualizer = useVirtualizer<HTMLElement, HTMLElement>(
  computed(() => ({
    count: filtered.value.length,
    getScrollElement: () => scrollEl.value,
    // Rows are pinned to an exact height — the option button is h-8 (32px) and the
    // row wrapper adds py-0.5 (4px) = 36px — so this estimate IS the real height for
    // every row. That determinism is why we don't measureElement: there is no
    // variance to measure, the translateY offsets stay pixel-exact, and we skip a
    // getBoundingClientRect reflow per visible row on open/scroll.
    estimateSize: () => 36,
    overscan: 8,
    getItemKey: (index: number) => filtered.value[index]?.value ?? index,
  })),
)

const totalSize = computed(() => virtualizer.value.getTotalSize())

const virtualRows = computed(() =>
  virtualizer.value.getVirtualItems().flatMap((vi) => {
    const option = filtered.value[vi.index]
    return option ? [{ key: String(vi.key), index: vi.index, start: vi.start, option }] : []
  }),
)

// useListboxKeyboard skips non-'item' rows, so wrap the flat options as item rows.
// Indexes stay 1:1 with `filtered`, so activeIndex lines up with the virtual rows.
const keyboardRows = computed(() =>
  filtered.value.map((option) => ({ type: 'item' as const, value: option.value })),
)

const { activeIndex, onKeydown, reset: resetActive } = useListboxKeyboard({
  rows: keyboardRows,
  scrollToIndex: (index) => virtualizer.value.scrollToIndex(index),
  onSelect: (row) => select(row.value),
})

watch(open, (value) => {
  if (!value) return
  search.value = ''
  resetActive()
  nextTick(() => {
    const selectedIndex = filtered.value.findIndex((option) => option.value === modelValue.value)
    virtualizer.value.scrollToIndex(selectedIndex >= 0 ? selectedIndex : 0)
  })
})

watch(search, () => virtualizer.value.scrollToOffset(0))

function select(value: string) {
  modelValue.value = value
  open.value = false
}
</script>
