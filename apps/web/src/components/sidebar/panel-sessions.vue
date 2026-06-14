<template>
  <div class="flex flex-col h-full min-w-0">
    <!-- Quick Actions: a named cluster of "things you can start" (New Chat,
         Scheduled Jobs), mirroring the Recents block below (actions you take vs
         history you return to). The label does double duty: it names the group
         AND its top padding is what separates this cluster from the nav above —
         without it New Chat butts against the nav, and since the active Chat pill
         bleeds left on hover the whole top reads as one smudged block. Same
         label style/size as Recents so the two section headings match. -->
    <div class="shrink-0 px-2 pb-0.5 pt-2">
      <span class="pl-[11px] text-xs font-[550] tracking-[-0.02em] text-muted-foreground/80">
        {{ t('chat.quickActions') }}
      </span>
    </div>
    <!-- Action rows share the sidebar icon column: px-[11px] + 18px icon puts the
         glyph at x=19 and the label at x=45, matching the nav tab, session rows
         and Settings so icons line up vertically and labels share one x. -->
    <div class="flex flex-col px-2 pb-0.5 shrink-0">
      <Button
        variant="ghost"
        block
        class="h-9 justify-start gap-[9px] px-[11px] text-control font-medium text-foreground/92 dark:text-[color:oklch(0.86_0_0)]"
        :disabled="!currentBotId"
        @click="handleNewSession"
      >
        <SquarePen
          :stroke-width="1.75"
          class="size-[18px]"
        />
        {{ t('chat.newSession') }}
      </Button>
      <Button
        variant="ghost"
        block
        class="h-9 justify-start gap-[9px] px-[11px] text-control font-medium text-foreground/92 dark:text-[color:oklch(0.86_0_0)]"
        :disabled="!currentBotId"
        @click="handleScheduledJobs"
      >
        <Clock
          :stroke-width="1.75"
          class="size-[18px]"
        />
        {{ t('chat.scheduledJobs') }}
      </Button>
    </div>
    <Recents class="flex-1 min-h-0" />
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Button } from '@memohai/ui'
import { SquarePen, Clock } from 'lucide-vue-next'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import Recents from './recents.vue'

const { t } = useI18n()
const router = useRouter()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId } = storeToRefs(chatStore)

function handleNewSession() {
  if (!currentBotId.value) return
  void chatStore.createNewSession()
  workspaceTabs.openChat(t('chat.newSession'))
}

// Scheduled jobs live on the bot's settings page as a tab. The bot-detail route
// accepts an id in its botName slot (same shortcut schedule-trigger-block uses),
// so we can deep-link straight to the current bot's schedule tab without
// resolving a name first.
function handleScheduledJobs() {
  const botId = currentBotId.value
  if (!botId) return
  void router.push({ name: 'bot-detail', params: { botName: botId }, query: { tab: 'schedule' } })
}
</script>
