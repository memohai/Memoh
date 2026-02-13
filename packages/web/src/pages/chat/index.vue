<template>
  <div class="flex h-[calc(100vh-calc(var(--spacing)*20))]">
    <!-- Left: Bot sidebar -->
    <BotSidebar />

    <!-- Right: Chat area -->
    <ChatArea />
  </div>
</template>

<script setup lang="ts">
import BotSidebar from './components/bot-sidebar.vue'
import ChatArea from './components/chat-area.vue'
</script>
