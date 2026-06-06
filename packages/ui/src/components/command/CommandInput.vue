<script setup lang="ts">
import type { ListboxFilterProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { Search } from 'lucide-vue-next'
import { ListboxFilter, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'
import { useCommand } from '.'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<ListboxFilterProps & {
  class?: HTMLAttributes['class']
  // The leading magnifier. On by default for the command palette; combobox-style
  // pickers turn it off so the query text lines up flush with the result rows.
  searchIcon?: boolean
  // Search row height. `sm` (h-9, 36px) is the compact command-palette default.
  // `md` (h-10, 40px) sits just under a comfortable 44px result row, matching the
  // search-over-list proportion used by mature combobox pickers.
  size?: 'sm' | 'md'
}>(), {
  searchIcon: true,
  size: 'sm',
})

const delegatedProps = reactiveOmit(props, 'class', 'searchIcon', 'size')

const forwardedProps = useForwardProps(delegatedProps)

const { filterState } = useCommand()
</script>

<template>
  <div
    data-slot="command-input-wrapper"
    :class="cn('flex items-center gap-2 border-b border-border/40 px-3.5', size === 'md' ? 'h-10' : 'h-9')"
  >
    <Search
      v-if="searchIcon"
      class="size-4 shrink-0 opacity-50"
    />
    <ListboxFilter
      v-bind="{ ...forwardedProps, ...$attrs }"
      v-model="filterState.search"
      data-slot="command-input"
      auto-focus
      :class="cn('placeholder:text-muted-foreground flex h-full w-full bg-transparent text-control outline-hidden disabled:cursor-not-allowed disabled:opacity-40', props.class)"
    />
  </div>
</template>
