<template>
  <button
    type="button"
    class="flex items-center gap-2 max-w-full rounded-full border bg-background/95 px-3 py-1.5 text-xs shadow-sm backdrop-blur hover:bg-accent"
    @click="$emit('jump')"
  >
    <LoaderCircle
      v-if="pill.tone === 'running'"
      class="size-3.5 shrink-0 animate-spin text-muted-foreground"
    />
    <CircleCheck
      v-else
      class="size-3.5 shrink-0 text-success-foreground"
    />
    <span class="shrink-0 font-medium">{{ label }}</span>
    <LivePeekLine
      v-if="pill.tone === 'running' && pill.latestLine"
      :text="pill.latestLine"
      mono
      class="min-w-0 flex-1"
    />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, LoaderCircle } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { BgTaskPill } from '@/store/chat-list.utils'
import LivePeekLine from './live-peek-line.vue'

const props = defineProps<{ pill: BgTaskPill }>()
defineEmits<{ jump: [] }>()
const { t } = useI18n()

const label = computed(() =>
  props.pill.tone === 'running'
    ? t('chat.backgroundTask.pillRunning', { count: props.pill.count })
    : t('chat.backgroundTask.pillDone', { count: props.pill.count }),
)
</script>
