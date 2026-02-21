<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select search provider'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <SearchProviderLogo
            v-if="selectedProvider"
            :provider="selectedProvider.provider || ''"
            size="xs"
          />
          <span class="truncate">{{ displayLabel || placeholder }}</span>
        </span>
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
          class="ml-2 size-3.5 shrink-0 text-muted-foreground"
        />
      </Button>
    </PopoverTrigger>
    <PopoverContent
      class="w-[--reka-popover-trigger-width] p-0"
      align="start"
    >
      <!-- Search input -->
      <div class="flex items-center border-b px-3">
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
          class="mr-2 size-3.5 shrink-0 text-muted-foreground"
        />
        <input
          v-model="searchTerm"
          :placeholder="$t('searchProvider.searchPlaceholder')"
          aria-label="Search providers"
          class="flex h-10 w-full bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground"
        >
      </div>

      <!-- Provider list -->
      <ScrollArea
        class="max-h-64"
        role="listbox"
      >
        <div
          v-if="filteredProviders.length === 0 && !searchTerm"
          class="py-6 text-center text-sm text-muted-foreground"
        >
          {{ $t('searchProvider.empty') }}
        </div>
        <div
          v-else-if="filteredProviders.length === 0"
          class="py-6 text-center text-sm text-muted-foreground"
        >
          {{ $t('searchProvider.empty') }}
        </div>

        <div class="p-1">
          <!-- None option -->
          <button
            type="button"
            role="option"
            :aria-selected="!selected"
            class="relative flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
            :class="{ 'bg-accent': !selected }"
            @click="selectProvider('')"
          >
            <FontAwesomeIcon
              v-if="!selected"
              :icon="['fas', 'check']"
              class="size-3.5"
            />
            <span
              v-else
              class="size-3.5"
            />
            <span class="text-muted-foreground">{{ $t('common.none') }}</span>
          </button>

          <!-- Provider items -->
          <button
            v-for="item in filteredProviders"
            :key="item.id"
            type="button"
            role="option"
            :aria-selected="selected === item.id"
            class="relative flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
            :class="{ 'bg-accent': selected === item.id }"
            @click="selectProvider(item.id || '')"
          >
            <FontAwesomeIcon
              v-if="selected === item.id"
              :icon="['fas', 'check']"
              class="size-3.5"
            />
            <span
              v-else
              class="size-3.5"
            />
            <SearchProviderLogo
              :provider="item.provider || ''"
              size="xs"
            />
            <span class="truncate">{{ item.name || item.id }}</span>
            <span
              v-if="item.provider"
              class="ml-auto text-xs text-muted-foreground"
            >
              {{ item.provider }}
            </span>
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
import type { SearchprovidersGetResponse } from '@memoh/sdk'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'

const props = defineProps<{
  providers: SearchprovidersGetResponse[]
  placeholder?: string
}>()

const selected = defineModel<string>({ default: '' })
const searchTerm = ref('')
const open = ref(false)

// 打开时清空搜索
watch(open, (val) => {
  if (val) searchTerm.value = ''
})

// 搜索过滤
const filteredProviders = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase()
  if (!keyword) return props.providers
  return props.providers.filter(
    (p) =>
      (p.name?.toLowerCase().includes(keyword) ?? false)
      || (p.provider?.toLowerCase().includes(keyword) ?? false),
  )
})

// 选中的 provider 对象
const selectedProvider = computed(() => {
  if (!selected.value) return undefined
  return props.providers.find((p) => p.id === selected.value)
})

// 显示选中的名称
const displayLabel = computed(() => {
  if (!selected.value) return ''
  return selectedProvider.value?.name || selectedProvider.value?.id || selected.value
})

function selectProvider(id: string) {
  selected.value = id
  open.value = false
}
</script>
