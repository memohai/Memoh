<template>
  <!-- One reserved row under every turn. The height is always present so the
       layout never jumps; only visibility toggles. While the turn is still
       streaming the row stays fully hidden (no hover reveal) — actions on an
       in-flight answer are meaningless. The latest FINISHED turn keeps it
       visible; every other turn reveals it on pointer/focus within the turn's
       hover scope (group/msg, set on the message content wrapper). -->
  <div
    class="chat-message-meta flex h-7 items-center gap-0.5 transition-opacity duration-150 motion-reduce:transition-none"
    :class="[
      align === 'end' ? 'justify-end' : 'justify-start',
      streaming
        ? 'opacity-0 pointer-events-none'
        : persistent
          ? 'opacity-100'
          : 'opacity-0 pointer-events-none group-hover/msg:opacity-100 group-hover/msg:pointer-events-auto focus-within:opacity-100 focus-within:pointer-events-auto',
    ]"
  >
    <TooltipProvider :delay-duration="0">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground"
            :aria-label="copied ? t('chat.actions.copied') : t('chat.actions.copy')"
            @click="handleCopy"
          >
            <Check
              v-if="copied"
              class="size-3.5"
            />
            <Copy
              v-else
              class="size-3.5"
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ copied ? t('chat.actions.copied') : t('chat.actions.copy') }}</TooltipContent>
      </Tooltip>

      <span
        class="px-1.5 text-muted-foreground/80 select-none"
        :title="fullTime"
      >{{ relativeTime }}</span>
    </TooltipProvider>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, Check } from 'lucide-vue-next'
import { Button, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@memohai/ui'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  copyText: string
  relativeTime: string
  fullTime: string
  persistent?: boolean
  streaming?: boolean
  align?: 'start' | 'end'
}>()

const { t } = useI18n()
const { copyText: writeClipboard } = useClipboard()

const copied = ref(false)
let resetTimer: ReturnType<typeof setTimeout> | null = null

async function handleCopy() {
  const ok = await writeClipboard(props.copyText)
  if (!ok) return
  copied.value = true
  if (resetTimer) clearTimeout(resetTimer)
  resetTimer = setTimeout(() => { copied.value = false }, 1500)
}
</script>
