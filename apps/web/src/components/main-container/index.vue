<template>
  <SidebarInset class="flex flex-col overflow-hidden">
    <section class="flex-1 overflow-y-auto">
      <router-view v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </router-view>
    </section>
  </SidebarInset>
</template>

<script setup lang="ts">
import { SidebarInset } from '@memohai/ui'
</script>
