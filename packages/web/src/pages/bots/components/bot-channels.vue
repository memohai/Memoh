<template>
  <div class="flex gap-6 min-h-[400px]  mx-auto">
    <!-- Left: Channel list -->
    <div class="w-60 shrink-0 flex flex-col border rounded-lg">
      <div class="flex-1 overflow-y-auto">
        <!-- Loading -->
        <div
          v-if="isLoading && configuredChannels.length === 0"
          class="flex items-center justify-center h-full p-4"
        >
          <FontAwesomeIcon
            :icon="['fas', 'spinner']"
            class="size-4 animate-spin text-muted-foreground"
          />
        </div>

        <!-- Empty -->
        <div
          v-else-if="configuredChannels.length === 0"
          class="flex flex-col items-center justify-center h-full p-4 text-center"
        >
          <p class="text-sm text-muted-foreground">
            {{ $t('bots.channels.emptyTitle') }}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ $t('bots.channels.emptyDescription') }}
          </p>
        </div>

        <!-- Configured channels -->
        <div
          v-else
          class="p-1"
        >
          <button
            v-for="item in configuredChannels"
            :key="item.meta.type"
            type="button"
            :aria-pressed="selectedType === item.meta.type"
            class="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors hover:bg-accent"
            :class="{ 'bg-accent': selectedType === item.meta.type }"
            @click="selectedType = item.meta.type"
          >
            <div
              class="flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-bold uppercase"
              :class="channelBadgeClass(item.meta.type)"
            >
              {{ channelIcon(item.meta.type) }}
            </div>
            <div class="flex-1 text-left">
              <div class="font-medium">
                {{ item.meta.display_name }}
              </div>
              <div class="text-xs">
                <span
                  v-if="!item.config?.disabled"
                  class="text-green-600 dark:text-green-400"
                >
                  {{ $t('bots.channels.statusActive') }}
                </span>
                <span
                  v-else
                  class="text-muted-foreground"
                >
                  {{ $t('bots.channels.configured') }}
                </span>
              </div>
            </div>
          </button>
        </div>
      </div>

      <!-- Add button -->
      <div class="border-t p-2">
        <Popover v-model:open="addPopoverOpen">
          <PopoverTrigger as-child>
            <Button
              variant="outline"
              class="w-full"
              size="sm"
              :disabled="unconfiguredChannels.length === 0 && !isLoading"
            >
              <FontAwesomeIcon
                :icon="['fas', 'plus']"
                class="mr-2 size-3"
              />
              {{ $t('bots.channels.addChannel') }}
            </Button>
          </PopoverTrigger>
          <PopoverContent
            class="w-56 p-1"
            align="start"
          >
            <div
              v-if="unconfiguredChannels.length === 0"
              class="px-3 py-2 text-sm text-muted-foreground text-center"
            >
              {{ $t('bots.channels.noAvailableTypes') }}
            </div>
            <button
              v-for="item in unconfiguredChannels"
              :key="item.meta.type"
              type="button"
              class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm hover:bg-accent transition-colors"
              @click="addChannel(item.meta.type)"
            >
              <div
                class="flex size-7 shrink-0 items-center justify-center rounded-md text-xs font-bold uppercase"
                :class="channelBadgeClass(item.meta.type)"
              >
                {{ channelIcon(item.meta.type) }}
              </div>
              <span>{{ item.meta.display_name }}</span>
            </button>
          </PopoverContent>
        </Popover>
      </div>
    </div>

    <!-- Right: Channel settings -->
    <div class="flex-1 min-w-0">
      <div
        v-if="!selectedType || !selectedItem"
        class="flex h-full items-center justify-center text-sm text-muted-foreground"
      >
        {{ configuredChannels.length > 0 ? $t('bots.channels.selectType') : '' }}
      </div>

      <ChannelSettingsPanel
        v-else
        :key="selectedType"
        :bot-id="botId"
        :channel-item="selectedItem"
        @saved="refetch()"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Button,
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@memoh/ui'
import { useQuery } from '@pinia/colada'
import { getChannels, getBotsByIdChannelByPlatform } from '@memoh/sdk'
import type { HandlersChannelMeta, ChannelChannelConfig } from '@memoh/sdk'
import ChannelSettingsPanel from './channel-settings-panel.vue'

export interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{
  botId: string
}>()

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
          const { data: config } = await getBotsByIdChannelByPlatform({
            path: { id: botIdRef.value, platform: meta.type },
            throwOnError: true,
          })
          return { meta, config: config ?? null, configured: true } as BotChannelItem
        } catch {
          return { meta, config: null, configured: false } as BotChannelItem
        }
      }),
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

const selectedItem = computed(() =>
  allChannels.value.find((c) => c.meta.type === selectedType.value) ?? null,
)

watch(configuredChannels, (list) => {
  if (list.length > 0 && !selectedType.value) {
    selectedType.value = list[0].meta.type
  }
}, { immediate: true })

function addChannel(type: string) {
  addPopoverOpen.value = false
  selectedType.value = type
}

function channelIcon(type: string): string {
  const icons: Record<string, string> = {
    telegram: 'TG',
    feishu: '飞',
  }
  return icons[type] ?? type.slice(0, 2).toUpperCase()
}

function channelBadgeClass(type: string): string {
  const classes: Record<string, string> = {
    telegram: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300',
    feishu: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900 dark:text-indigo-300',
  }
  return classes[type] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
}
</script>
