import type { Ref } from 'vue'
import { createContext } from 'reka-ui'

export { default as Command } from './Command.vue'
export { default as CommandDialog } from './CommandDialog.vue'
export { default as CommandEmpty } from './CommandEmpty.vue'
export { default as CommandGroup } from './CommandGroup.vue'
export { default as CommandInput } from './CommandInput.vue'
export { default as CommandItem } from './CommandItem.vue'
export { default as CommandKeyBridge } from './CommandKeyBridge.vue'
export { default as CommandList } from './CommandList.vue'
export { default as CommandSeparator } from './CommandSeparator.vue'
export { default as CommandShortcut } from './CommandShortcut.vue'

export const [useCommand, provideCommandContext] = createContext<{
  allItems: Ref<Map<string, string>>
  allGroups: Ref<Map<string, Set<string>>>
  filterState: {
    search: string
    filtered: { count: number, items: Map<string, number>, groups: Set<string> }
  }
  // Select-parity: a row reports pointerleave so the surface can clear its highlight
  // when the pointer sits on dead space (separators / padding) instead of a row.
  onItemPointerLeave: (el: HTMLElement | null) => void
}>('Command')

export const [useCommandGroup, provideCommandGroupContext] = createContext<{
  id?: string
}>('CommandGroup')
