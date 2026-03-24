<template>
  <Collapsible v-model:open="isOpen">
    <CollapsibleTrigger class="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors cursor-pointer group">
      <FontAwesomeIcon
        :icon="['fas', 'chevron-right']"
        class="size-3 transition-transform"
        :class="{ 'rotate-90': isOpen }"
      />
      <span class="flex items-center gap-1.5">
        <template v-if="streaming">
          <FontAwesomeIcon
            :icon="['fas', 'spinner']"
            class="size-3 animate-spin"
          />
          {{ $t('chat.thinkingInProgress') }}
        </template>
        <template v-else>
          💭 {{ $t('chat.thinkingDone') }}
        </template>
      </span>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div class="mt-2 ml-5 pl-3 border-l-2 border-muted text-sm text-muted-foreground">
        <div
          class="whitespace-pre-wrap leading-relaxed"
          v-text="block.content"
        />
      </div>
    </CollapsibleContent>
  </Collapsible>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memohai/ui'
import type { ThinkingBlock } from '@/store/chat-list'

defineProps<{
  block: ThinkingBlock
  streaming: boolean
}>()

const isOpen = ref(true)
</script>
