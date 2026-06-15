<template>
  <div class="flex flex-col h-full min-w-0">
    <!-- Quick Actions: a named cluster of "things you can start" (New Chat,
         Scheduled Jobs), mirroring the Recents block below (actions you take vs
         history you return to). The label does double duty: it names the group
         AND its top padding is what separates this cluster from the nav above —
         without it New Chat butts against the nav, and since the active Chat pill
         bleeds left on hover the whole top reads as one smudged block. Same
         label style/size as Recents so the two section headings match. -->
    <!-- EXPERIMENT: the Quick Actions heading is commented out so New Chat / Bot
         Settings ride straight up under the Chat nav tab. They already share the
         x=19 icon column with the nav (container px-2 + button px-[11px] + 18px
         icon), so dropping the heading just removes the vertical gap — the icons
         stay vertically aligned under the Chat tab's icon. Re-enable by
         uncommenting. -->
    <!--
    <div class="shrink-0 px-2 pb-0.5 pt-2">
      <span class="pl-[11px] text-xs font-[550] tracking-[-0.02em] text-muted-foreground/80">
        {{ t('chat.quickActions') }}
      </span>
    </div>
    -->
    <!-- Action rows share the sidebar icon column: px-[11px] sets the icon box at
         x=19, matching the nav tab, session rows and Settings so the column lines
         up. The two glyphs here are an OPTICAL exception: SquarePen / Settings2 read
         a touch large and right-heavy, so they're shrunk to ~95% (size-[17px]) and
         translate-x-nudged 1px left so their VISUAL center sits on the column —
         geometric x=19 isn't the optical center for these shapes. pt-1 keeps New
         Chat a hair off the nav row instead of butting against it. -->
    <div class="flex flex-col px-2 pb-0.5 pt-1 shrink-0">
      <Button
        variant="ghost"
        block
        class="h-9 justify-start gap-[9px] px-[11px] text-control font-medium text-foreground/92 dark:text-[color:oklch(0.86_0_0)]"
        :disabled="!currentBotId"
        @click="handleNewSession"
      >
        <SquarePen
          :stroke-width="1.75"
          class="size-[17px] -translate-x-px"
        />
        {{ t('chat.newSession') }}
      </Button>
      <Button
        variant="ghost"
        block
        class="h-9 justify-start gap-[9px] px-[11px] text-control font-medium text-foreground/92 dark:text-[color:oklch(0.86_0_0)]"
        :disabled="!currentBotId"
        @click="handleBotSettings"
      >
        <Settings2
          :stroke-width="1.75"
          class="size-[17px] -translate-x-px"
        />
        {{ t('chat.botSettings') }}
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
import { SquarePen, Settings2 } from 'lucide-vue-next'
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

// Navigate to the current bot's settings overview.
function handleBotSettings() {
  const botId = currentBotId.value
  if (!botId) return
  void router.push({ name: 'bot-detail', params: { botName: botId } })
}
</script>
