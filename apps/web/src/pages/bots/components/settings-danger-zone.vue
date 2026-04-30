<template>
  <div class="space-y-4 rounded-md border border-border bg-transparent p-5 shadow-none">
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div class="space-y-1">
        <h4 class="text-sm font-semibold text-destructive">
          {{ $t('bots.settings.dangerZone') }}
        </h4>
        <p class="text-xs text-muted-foreground leading-relaxed">
          {{ $t('bots.settings.deleteBotDescription') }}
        </p>
      </div>
      <div class="flex justify-end shrink-0">
        <ConfirmPopover
          :message="$t('bots.deleteConfirm')"
          :loading="deleteLoading"
          :confirm-text="$t('common.delete')"
          @confirm="$emit('delete')"
        >
          <template #trigger>
            <Button
              variant="destructive"
              :disabled="deleteLoading"
              class="min-w-32"
            >
              <Spinner
                v-if="deleteLoading"
                class="mr-1.5 size-3.5"
              />
              {{ $t('bots.settings.deleteBot') }}
            </Button>
          </template>
        </ConfirmPopover>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Button, Spinner } from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'

defineProps<{
  deleteLoading: boolean
}>()

defineEmits<{
  'delete': []
}>()
</script>
