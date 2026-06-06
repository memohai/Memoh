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
  // Default OFF, matching Select/Combobox: opening lands on NOTHING (the value-less
  // default) and the highlight only follows the pointer/arrows. Opening a fresh
  // Select never auto-picks the first option, so neither should a Command palette.
  // Set true only for a true cmdk palette that wants Enter to fire the first row.
  highlightFirstOnOpen?: boolean
}>(), {
  modelValue: '',
  highlightFirstOnOpen: false,
  // Rows must highlight under the pointer, exactly like Select / DropdownMenu /
  // Combobox. reka's ListboxRoot ships this OFF, which made a command surface read
  // as "click-only" (the highlight only followed the keyboard). Default it ON so
  // every Command — inline, dialog, combobox — shares the one menu-row interaction.
  highlightOnHover: true,
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

// reka's ListboxRoot exposes its live highlight as a writable ref; we drive it for
// both the open-with-no-highlight one-shot (below) and the leave-clears-highlight
// parity with Select (next).
const listboxRef = ref<{ highlightedElement: HTMLElement | null } | null>(null)

// Select clears its row highlight the instant the pointer leaves a row (its
// SelectItem blurs via onItemLeave), so hovering the gap/separator between rows
// shows NOTHING highlighted. reka's Listbox does not — it only ever MOVES the
// highlight to the next hovered row and never clears it, so the last row stays lit
// while the pointer sits on a separator. We restore Select's behaviour: each
// CommandItem reports its own pointerleave, and we null reka's highlight if (and
// only if) the leaving row is the one currently lit. Entering the next row re-lights
// it through reka's own pointermove, so adjacent moves are seamless and only the
// dead space (separators / padding) reads as un-highlighted — exactly like Select.
function onItemPointerLeave(el: HTMLElement | null) {
  if (el && listboxRef.value && listboxRef.value.highlightedElement === el)
    listboxRef.value.highlightedElement = null
}

provideCommandContext({
  allItems,
  allGroups,
  filterState,
  onItemPointerLeave,
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
    :class="cn(
      // Command IS a floating menu surface, so it owns the exact same chrome as
      // DropdownMenu/Select (lib/menu.ts: bg-popover, border-menu hairline,
      // dropdown shadow, menu-shell radius). Owning it here — instead of letting
      // each call site hand-roll border/shadow — is what guarantees the inline
      // command, the Combobox (Popover + Command), and the CommandDialog all read
      // identically. A host that supplies its own elevation (e.g. CommandDialog's
      // transparent DialogContent) just lets this surface show through.
      'bg-popover text-popover-foreground border border-[color:var(--border-menu)] rounded-menu-shell shadow-[var(--shadow-dropdown)] flex h-full w-full flex-col overflow-hidden',
      props.class,
    )"
    @entry-focus="onEntryFocus"
    @highlight="onHighlight"
  >
    <slot />
  </ListboxRoot>
</template>
