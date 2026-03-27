<template>
  <div class="h-[53px] shrink-0 flex items-center px-4 border-b border-border">
    <ChatStatus />
    <div class="ml-2 flex flex-col gap-0.5">
      <h4 class="text-sm font-medium">
        Memoh
        <ChatStep />
      </h4>
      <span class="text-xs text-muted-foreground">
        Memoh Bot (GPT-5.2)
      </span>
    </div>
    <div class="ml-auto">
      <Button
        variant="ghost"
        size="icon"
      >
        <FontAwesomeIcon :icon="['fas', 'arrow-rotate-right']" />
      </Button>
    </div>
  </div>
</template>

<script lang="ts" setup>
import ChatStatus from '@/components/chat/chat-status/index.vue'
import ChatStep from '@/components/chat/chat-step/index.vue'
import { Button } from '@memohai/ui'
</script>
