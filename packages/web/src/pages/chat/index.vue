<template>
  <div class="flex h-full">
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
