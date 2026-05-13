<template>
  <div class="flex gap-6 absolute inset-0 max-w-4xl mx-auto px-4 pt-4 pb-6 w-full">
    <!-- L3: Silent Hub Rail -->
    <div class="h-full flex-col border rounded-lg overflow-hidden flex sticky top-0 bg-background/50">
      <ScrollArea class="w-60 flex-1 flex flex-col">
        <!-- Skeleton Loading -->
        <div
          v-if="isLoading && configuredChannels.length === 0"
          class="p-2 space-y-2"
        >
          <Skeleton class="h-10 w-full rounded-md" />
          <Skeleton class="h-10 w-full rounded-md" />
          <Skeleton class="h-10 w-full rounded-md" />
        </div>

        <!-- Empty -->
        <div
          v-else-if="configuredChannels.length === 0"
          class="flex flex-col items-center justify-center h-full p-4 text-center"
        >
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.emptyTitle') }}
          </p>
          <p class="mt-1 text-[11px] text-muted-foreground">
            {{ $t('bots.channels.emptyDescription') }}
          </p>
        </div>

        <!-- Platform List (High Density py-1.5) -->
        <div
          v-else
          class="p-1 space-y-0.5"
        >
          <button
            v-for="item in configuredChannels"
            :key="item.meta.type"
            type="button"
            :aria-pressed="selectedType === item.meta.type"
            class="flex w-full items-center gap-3 rounded-md px-3 py-1.5 text-xs transition-colors hover:bg-accent/50 outline-none focus-visible:ring-1 focus-visible:ring-ring"
            :class="{ 'bg-accent/40 font-medium text-foreground': selectedType === item.meta.type, 'text-muted-foreground': selectedType !== item.meta.type }"
            @click="selectedType = item.meta.type ?? ''"
          >
            <span class="flex size-7 shrink-0 items-center justify-center rounded-md bg-transparent">
              <ChannelIcon
                :channel="item.meta.type as string"
                size="1.25em"
              />
            </span>
            <div class="flex-1 text-left min-w-0">
              <div class="truncate flex items-center gap-1">
                {{ channelTitle(item.meta) }}
                <!-- Dirty state indicator (*) -->
                <span
                  v-if="dirtyStates[item.meta.type ?? '']"
                  class="text-warning font-bold"
                  title="Unsaved changes"
                >*</span>
              </div>
              <div class="text-[11px] truncate">
                <span
                  v-if="!item.config?.disabled"
                  class="text-success"
                >{{ $t('bots.channels.statusActive') }}</span>
                <span
                  v-else
                  class="opacity-70"
                >{{ $t('bots.channels.configured') }}</span>
              </div>
            </div>
          </button>
        </div>
      </ScrollArea>
      
      <!-- Add Platform Trigger -->
      <div class="border-t p-2 bg-background">
        <Popover v-model:open="addPopoverOpen">
          <PopoverTrigger as-child>
            <Button
              variant="ghost"
              class="w-full h-8 text-xs text-muted-foreground hover:text-foreground"
              size="sm"
              :disabled="unconfiguredChannels.length === 0 && !isLoading"
            >
              <Plus class="mr-2 size-3" />
              {{ $t('bots.channels.addChannel') }}
            </Button>
          </PopoverTrigger>
          <PopoverContent
            class="w-56 p-1 shadow-md"
            align="start"
          >
            <div
              v-if="unconfiguredChannels.length === 0"
              class="px-3 py-2 text-[11px] text-muted-foreground text-center"
            >
              {{ $t('bots.channels.noAvailableTypes') }}
            </div>
            <button
              v-for="item in unconfiguredChannels"
              :key="item.meta.type"
              type="button"
              class="flex w-full items-center gap-3 rounded-md px-3 py-1.5 text-xs hover:bg-accent transition-colors"
              @click="addChannel(item.meta.type ?? '')"
            >
              <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                <ChannelIcon
                  :channel="item.meta.type ?? ''"
                  size="1em"
                />
              </span>
              <span>{{ channelTitle(item.meta) }}</span>
            </button>
          </PopoverContent>
        </Popover>
      </div>
    </div>

    <!-- L4: Right Workspace -->
    <div class="flex-1 min-w-0 pr-4">
      <ScrollArea class="h-full">
        <div
          v-if="!selectedType || !selectedItem"
          class="flex h-full items-center justify-center text-xs text-muted-foreground"
        >
          {{ configuredChannels.length > 0 ? $t('bots.channels.selectType') : '' }}
        </div>
        
        <!-- Skeleton Placeholder -->
        <div
          v-else-if="isLoading"
          class="space-y-6"
        >
          <Skeleton class="h-20 w-full rounded-md" />
          <Skeleton class="h-64 w-full rounded-md" />
        </div>

        <ChannelSettingsPanel
          v-else
          :key="selectedType"
          :bot-id="botId"
          :channel-item="selectedItem"
          :all-dirty-states="dirtyStates"
          @update:dirty="(isDirty) => updateDirtyState(selectedType!, isDirty)"
          @switch-tab="(type) => selectedType = type"
          @saved="handleSaved"
        />
      </ScrollArea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Plus } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button, Popover, PopoverTrigger, PopoverContent, ScrollArea, Skeleton } from '@memohai/ui'
import { useQuery } from '@pinia/colada'
import { getChannels, getBotsByIdChannelByPlatform } from '@memohai/sdk'
import type { HandlersChannelMeta, ChannelChannelConfig } from '@memohai/sdk'
import ChannelSettingsPanel from './channel-settings-panel.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import { channelTypeDisplayName } from '@/utils/channel-type-label'

export interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{ botId: string }>()
const { t } = useI18n()

// Dirty state tracking
const dirtyStates = ref<Record<string, boolean>>({})

function channelTitle(meta: HandlersChannelMeta) {
  return channelTypeDisplayName(t, meta.type, meta.display_name)
}

const botIdRef = computed(() => props.botId)

const { data: channels, isLoading, refetch } = useQuery({
  key: () => ['bot-channels', botIdRef.value],
  query: async (): Promise<BotChannelItem[]> => {
    const { data: metas } = await getChannels({ throwOnError: true })
    if (!metas) return []
    const configurableTypes = metas.filter((m) => !m.configless)
    const results = await Promise.all(
      configurableTypes.map(async (meta) => {
        try {
          const { data: config } = await getBotsByIdChannelByPlatform({ path: { id: botIdRef.value, platform: meta.type ?? '' }, throwOnError: true })
          return { meta, config: config ?? null, configured: true } as BotChannelItem
        } catch {
          return { meta, config: null, configured: false } as BotChannelItem
        }
      })
    )
    return results
  },
  enabled: () => !!botIdRef.value,
})

const selectedType = ref<string | null>(null)
const addPopoverOpen = ref(false)

const allChannels = computed<BotChannelItem[]>(() => channels.value ?? [])
const configuredChannels = computed(() => allChannels.value.filter((c) => c.configured))
const unconfiguredChannels = computed(() => allChannels.value.filter((c) => !c.configured))

const selectedItem = computed(() => allChannels.value.find((c) => c.meta.type === selectedType.value) ?? null)

watch(configuredChannels, (list) => {
  if (list.length === 0) return
  if (!selectedType.value || !list.some((item) => item.meta.type === selectedType.value)) {
    const configured = list.find((item) => item.configured)
    selectedType.value = configured?.meta.type ?? list[0]?.meta.type ?? null
  }
}, { immediate: true })

function addChannel(type: string) {
  addPopoverOpen.value = false
  selectedType.value = type
}

function updateDirtyState(type: string, isDirty: boolean) {
  dirtyStates.value[type] = isDirty
}

function handleSaved() {
  if (selectedType.value) dirtyStates.value[selectedType.value] = false
  refetch()
}
</script>
