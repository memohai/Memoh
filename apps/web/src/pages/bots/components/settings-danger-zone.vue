<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <!-- Danger Zone: High-visibility title without background highlight -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div class="space-y-0.5">
        <h4 class="text-xs font-medium text-destructive">
          {{ $t('bots.settings.dangerZone') }}
        </h4>
        <p class="text-[11px] text-muted-foreground">
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
              size="sm"
              :disabled="deleteLoading"
              class="min-w-28 h-8 text-xs font-medium shadow-none"
            >
              <Spinner
                v-if="deleteLoading"
                class="mr-1.5 size-3"
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
