<template>
  <SettingsSection>
    <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3">
      <div class="min-w-0">
        <div class="text-sm font-medium text-destructive">
          {{ $t('bots.settings.dangerZone') }}
        </div>
        <p class="mt-0.5 text-xs text-muted-foreground">
          {{ $t('bots.settings.deleteBotDescription') }}
        </p>
      </div>
      <div class="shrink-0">
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
  </SettingsSection>
</template>

<script setup lang="ts">
import { Button, Spinner } from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SettingsSection from '@/components/settings/section.vue'

defineProps<{
  deleteLoading: boolean
}>()

defineEmits<{
  'delete': []
}>()
</script>
