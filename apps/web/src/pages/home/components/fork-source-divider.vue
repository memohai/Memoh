<template>
  <div class="flex items-center gap-3 py-1">
    <span
      aria-hidden="true"
      class="h-px flex-1 bg-border"
    />
    <TextButton
      type="button"
      class="max-w-[min(22rem,70vw)] text-caption font-normal text-muted-foreground"
      :title="title"
      @click="$emit('openSource')"
    >
      <span class="truncate">
        {{ t('chat.forkedFrom', { session: title }) }}
      </span>
    </TextButton>
    <span
      aria-hidden="true"
      class="h-px flex-1 bg-border"
    />
  </div>
</template>

<script setup lang="ts">
import { TextButton } from '@memohai/ui'
import { useI18n } from 'vue-i18n'

defineProps<{
  title: string
}>()

defineEmits<{
  openSource: []
}>()

const { t } = useI18n()
</script>
