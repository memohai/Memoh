<template>
  <div class="flex h-full w-full items-center justify-center bg-card">
    <div class="text-center px-6">
      <p class="text-xs font-medium text-foreground">
        {{ t('chat.workspaceEmpty') }}
      </p>
      <p class="mt-1 text-xs text-muted-foreground">
        {{ t('chat.workspaceEmptyHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
