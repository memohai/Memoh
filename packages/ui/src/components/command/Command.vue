<script setup lang="ts">
import type { ListboxRootEmits, ListboxRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ListboxRoot, useFilter, useForwardPropsEmits } from 'reka-ui'
import { onMounted, reactive, ref, watch } from 'vue'
import { cn } from '#/lib/utils'
import { provideCommandContext } from '.'

const props = withDefaults(defineProps<ListboxRootProps & {
  class?: HTMLAttributes['class']
  // cmdk-style palettes pre-highlight the first row on open so Enter selects it.
  // Combobox-style pickers set this false: opening should land on NOTHING (the
  // value-less default), and the highlight should only follow the pointer/arrows.
  highlightFirstOnOpen?: boolean
}>(), {
  modelValue: '',
  highlightFirstOnOpen: true,
})

const emits = defineEmits<ListboxRootEmits>()

const delegatedProps = reactiveOmit(props, 'class', 'highlightFirstOnOpen')

const forwarded = useForwardPropsEmits(delegatedProps, emits)

const allItems = ref<Map<string, string>>(new Map())
const allGroups = ref<Map<string, Set<string>>>(new Map())

const { contains } = useFilter({ sensitivity: 'base' })
const filterState = reactive({
  search: '',
  filtered: {
    /** The count of all visible items. */
    count: 0,
    /** Map from visible item id to its search score. */
    items: new Map() as Map<string, number>,
    /** Set of groups with at least one visible item. */
    groups: new Set() as Set<string>,
  },
})

function filterItems() {
  if (!filterState.search) {
    filterState.filtered.count = allItems.value.size
    // Do nothing, each item will know to show itself because search is empty
    return
  }

  // Reset the groups
  filterState.filtered.groups = new Set()
  let itemCount = 0

  // Check which items should be included
  for (const [id, value] of allItems.value) {
    const score = contains(value, filterState.search)
    filterState.filtered.items.set(id, score ? 1 : 0)
    if (score)
      itemCount++
  }

  // Check which groups have at least 1 item shown
  for (const [groupId, group] of allGroups.value) {
    for (const itemId of group) {
      if (filterState.filtered.items.get(itemId)! > 0) {
        filterState.filtered.groups.add(groupId)
        break
      }
    }
  }

  filterState.filtered.count = itemCount
}

watch(() => filterState.search, () => {
  filterItems()
})

provideCommandContext({
  allItems,
  allGroups,
  filterState,
})

// Combobox pickers (highlightFirstOnOpen=false) must open with NO row highlighted.
// reka pre-highlights the first row two ways: (1) an immediate watch that, lacking a
// selected value, falls back to collection[0]; (2) RovingFocus "entry focus" firing
// when the filter input autofocuses, which re-highlights the first/previous row.
//
// (2) is blocked by preventing the cancelable entryFocus event. With it gone, the
// ONLY automatic highlight left is (1), which fires exactly once on open. So we arm a
// one-shot and undo it on that first `highlight` emit — nulling reka's highlightedElement
// (exposed ref) synchronously in the same tick reka set it, before first paint (no
// flash). Clearing the real state, not just visuals, means a bare Enter on a fresh,
// untouched picker selects nothing. Every later highlight is user-driven (pointer /
// arrows) and passes through. A real selection is preserved: the one-shot is never armed
// when a value is present, so the selected row stays highlighted on reopen.
const listboxRef = ref<{ highlightedElement: HTMLElement | null } | null>(null)
let clearInitialHighlight = false

function onEntryFocus(event: Event) {
  if (!props.highlightFirstOnOpen)
    event.preventDefault()
}

function onHighlight() {
  if (!clearInitialHighlight)
    return
  clearInitialHighlight = false
  if (listboxRef.value)
    listboxRef.value.highlightedElement = null
}

onMounted(() => {
  if (props.highlightFirstOnOpen)
    return
  const v = props.modelValue
  const hasValue = Array.isArray(v) ? v.length > 0 : v != null && v !== ''
  clearInitialHighlight = !hasValue
})
</script>

<template>
  <ListboxRoot
    ref="listboxRef"
    data-slot="command"
    v-bind="forwarded"
    :class="cn('bg-popover text-popover-foreground flex h-full w-full flex-col overflow-hidden rounded-menu-shell', props.class)"
    @entry-focus="onEntryFocus"
    @highlight="onHighlight"
  >
    <slot />
  </ListboxRoot>
</template>
