<template>
  <div class="flex gap-6 absolute inset-0  mx-auto p-4">
    <!-- Left: Channel list -->
    <div class="h-full flex-col border rounded-lg overflow-hidden flex">
      <ScrollArea class="w-60 flex-1 flex flex-col ">
        <!-- Loading -->
        <div
          v-if="isLoading && configuredChannels.length === 0"
          class="flex items-center justify-center h-full p-4"
        >
          <LoaderCircle
            class="size-4 animate-spin text-muted-foreground"
          />
        </div>

        <!-- Empty -->
        <div
          v-else-if="configuredChannels.length === 0"
          class="flex flex-col items-center justify-center h-full p-4 text-center"
        >
          <p class="text-xs text-muted-foreground">
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
            class="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-xs transition-colors hover:bg-accent"
            :class="{ 'bg-accent': selectedType === item.meta.type }"
            @click="selectedType = item.meta.type ?? ''"
          >
            <span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
              <ChannelIcon
                :channel="item.meta.type as string"
                size="1.25em"
              />
            </span>
            <div class="flex-1 text-left">
              <div class="font-medium">
                {{ channelTitle(item.meta) }}
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
      </ScrollArea>
      <!-- Add button -->
      <div class="border-t p-2  ">
        <Popover v-model:open="addPopoverOpen">
          <PopoverTrigger as-child>
            <Button
              variant="outline"
              class="w-full"
              size="sm"
              :disabled="unconfiguredChannels.length === 0 && !isLoading"
            >
              <Plus
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
              class="px-3 py-2 text-xs text-muted-foreground text-center"
            >
              {{ $t('bots.channels.noAvailableTypes') }}
            </div>
            <button
              v-for="item in unconfiguredChannels"
              :key="item.meta.type"
              type="button"
              class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-xs hover:bg-accent transition-colors"
              @click="addChannel(item.meta.type ?? '')"
            >
              <span class="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
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
    <!-- Right: Channel settings -->
    <div class="flex-1 min-w-0">
      <div
        v-if="!selectedType || !selectedItem"
        class="flex h-full items-center justify-center text-xs text-muted-foreground"
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
import { LoaderCircle, Plus } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Button,
  Popover,
  PopoverTrigger,
  PopoverContent,
  ScrollArea
} from '@memohai/ui'
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

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

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
          const { data: config } = await getBotsByIdChannelByPlatform({
            path: { id: botIdRef.value, platform: meta.type ?? '' },
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
  if (list.length === 0) return

  const first = list[0]
  if (first && !selectedType.value) {
    selectedType.value = first.meta.type ?? null
  }

  const current = selectedType.value
  if (current && list.some((item) => item.meta.type === current)) {
    return
  }

  const configured = list.find((item) => item.configured)
  selectedType.value = configured?.meta.type ?? list[0]?.meta.type ?? null
}, { immediate: true })

function addChannel(type: string) {
  addPopoverOpen.value = false
  selectedType.value = type
}
</script>
