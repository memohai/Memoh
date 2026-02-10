<template>
  <section class="p-6 max-w-7xl mx-auto">
    <!-- Header -->
    <div class="flex items-center gap-4 mb-8">
      <Avatar class="size-16 shrink-0">
        <AvatarImage
          v-if="bot?.avatar_url"
          :src="bot.avatar_url"
          :alt="bot.display_name"
        />
        <AvatarFallback class="text-xl">
          {{ avatarFallback }}
        </AvatarFallback>
      </Avatar>
      <div>
        <h2 class="text-2xl font-semibold tracking-tight">
          {{ bot?.display_name || botId }}
        </h2>
        <div class="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Badge
            v-if="bot"
            :variant="bot.is_active ? 'default' : 'secondary'"
            class="text-xs"
          >
            {{ bot.is_active ? $t('bots.active') : $t('bots.inactive') }}
          </Badge>
          <span v-if="bot?.type">{{ bot.type }}</span>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <Tabs
      v-model="activeTab"
      class="w-full"
    >
      <TabsList class="w-full justify-start">
        <TabsTrigger value="overview">
          {{ $t('bots.tabs.overview') }}
        </TabsTrigger>
        <TabsTrigger value="memory">
          {{ $t('bots.tabs.memory') }}
        </TabsTrigger>
        <TabsTrigger value="channels">
          {{ $t('bots.tabs.channels') }}
        </TabsTrigger>
        <TabsTrigger value="mcp">
          {{ $t('bots.tabs.mcp') }}
        </TabsTrigger>
        <TabsTrigger value="subagents">
          {{ $t('bots.tabs.subagents') }}
        </TabsTrigger>
        <TabsTrigger value="history">
          {{ $t('bots.tabs.history') }}
        </TabsTrigger>
        <TabsTrigger value="settings">
          {{ $t('bots.tabs.settings') }}
        </TabsTrigger>
      </TabsList>

      <TabsContent
        value="overview"
        class="mt-6"
      >
        <!-- TODO: Overview content -->
      </TabsContent>
      <TabsContent
        value="memory"
        class="mt-6"
      >
        <!-- TODO: Memory content -->
      </TabsContent>
      <TabsContent
        value="channels"
        class="mt-6"
      >
        <BotChannels :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="mcp"
        class="mt-6"
      >
        <!-- TODO: MCP content -->
      </TabsContent>
      <TabsContent
        value="subagents"
        class="mt-6"
      >
        <!-- TODO: Subagents content -->
      </TabsContent>
      <TabsContent
        value="history"
        class="mt-6"
      >
        <!-- TODO: History content -->
      </TabsContent>
      <TabsContent
        value="settings"
        class="mt-6"
      >
        <BotSettings :bot-id="botId" />
      </TabsContent>
    </Tabs>
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@memoh/ui'
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useBotDetail } from '@/composables/api/useBots'
import BotSettings from './components/bot-settings.vue'
import BotChannels from './components/bot-channels.vue'

const route = useRoute()
const botId = computed(() => route.params.botId as string)

const { data: bot } = useBotDetail(botId)

// 加载到 bot 数据后，用名称替换 breadcrumb 中的 botId
watch(bot, (val) => {
  if (val?.display_name) {
    route.meta.breadcrumb = () => val.display_name
  }
})

const activeTab = ref('overview')

const avatarFallback = computed(() => {
  const name = bot.value?.display_name || botId.value || ''
  return name.slice(0, 2).toUpperCase()
})
</script>
