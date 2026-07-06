<template>
  <div class="flex items-center gap-3 py-1">
    <span
      aria-hidden="true"
      class="h-px flex-1 bg-border"
    />
    <TextButton
      type="button"
      class="max-w-[min(22rem,70vw)] text-caption font-[400] text-cop-title"
      :title="title"
      :disabled="disabled"
      @click="handleOpenSource"
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

const props = defineProps<{
  title: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  openSource: []
}>()

const { t } = useI18n()

function handleOpenSource() {
  if (props.disabled) return
  emit('openSource')
}
</script>
