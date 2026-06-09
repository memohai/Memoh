<template>
  <div class="text-sm leading-relaxed">
    <button
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-0.5 text-muted-foreground hover:text-foreground"
      @click="toggleOpen"
    >
      <Lightbulb class="size-3.5 shrink-0" />
      <span
        class="shrink-0"
        :class="actionClass"
      >{{ actionLabel }}</span>
      <ChevronRight
        v-if="!open"
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
      <ChevronDown
        v-else
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
    </button>

    <LivePeekLine
      v-if="!open && streaming && peekLine"
      :text="block.content"
      :interval-ms="800"
      class="ml-5"
    />

    <div
      v-if="open"
      class="mt-1 ml-5 border-l border-border pl-3 py-1"
    >
      <div
        class="whitespace-pre-wrap text-xs text-muted-foreground leading-relaxed"
        v-text="block.content"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, ChevronRight, Lightbulb } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ThinkingBlock } from '@/store/chat-list'
import { latestOutputLine } from '@/store/chat-list.utils'
import LivePeekLine from './live-peek-line.vue'

const props = defineProps<{
  block: ThinkingBlock
  streaming: boolean
}>()

const { t } = useI18n()

const open = ref(false)

const peekLine = computed(() => latestOutputLine(props.block.content))

const actionLabel = computed(() =>
  props.streaming ? t('chat.thinkingInProgress') : t('chat.thinkingDone'),
)

const actionClass = computed(() => (props.streaming ? 'tool-shimmer-text' : ''))

function toggleOpen() {
  open.value = !open.value
}
</script>
