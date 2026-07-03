<template>
  <div class="flex items-center gap-3 py-1 text-caption text-border">
    <span
      aria-hidden="true"
      class="h-px flex-1 bg-border"
    />
    <Button
      type="button"
      variant="ghost"
      size="text"
      class="max-w-[min(22rem,70vw)] text-border"
      :title="title"
      @click="$emit('openSource')"
    >
      <span class="truncate">
        {{ t('chat.forkedFrom', { session: title }) }}
      </span>
    </Button>
    <span
      aria-hidden="true"
      class="h-px flex-1 bg-border"
    />
  </div>
</template>

<script setup lang="ts">
import { Button } from '@memohai/ui'
import { useI18n } from 'vue-i18n'

defineProps<{
  title: string
}>()

defineEmits<{
  openSource: []
}>()

const { t } = useI18n()
</script>
