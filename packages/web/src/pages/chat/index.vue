<template>
  <div class="flex h-full">
    <MasterDetailSidebarLayout>
      <template #sidebar-header>
        <div class="p-4 border-b">
          <p class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
            {{ $t('sidebar.bots') }}
          </p>
        </div>
      </template>
      <template #sidebar-content>        
        <BotSidebar />
      </template>
      <template #detail>
        <ChatArea />
      </template>
    </MasterDetailSidebarLayout>
  </div>
</template>

<script setup lang="ts">
import BotSidebar from './components/bot-sidebar.vue'
import ChatArea from './components/chat-area.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

</script>
