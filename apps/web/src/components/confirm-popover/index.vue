<template>
  <Popover>
    <template #default="{ close }">
      <PopoverTrigger as-child>
        <slot name="trigger" />
      </PopoverTrigger>
      <!-- Inherit the shared popover chrome (menu-shell radius, --border-menu
           hairline, dropdown shadow, p-4) instead of overriding it — this is the
           same surface as DropdownMenu / Select, so a confirm reads as part of
           the one menu language. -->
      <PopoverContent class="w-72">
        <div class="space-y-3">
          <div
            v-if="title"
            class="flex items-center gap-2"
          >
            <span
              v-if="$slots.icon"
              class="shrink-0"
            >
              <slot name="icon" />
            </span>
            <h5 class="min-w-0 truncate text-sm font-medium text-foreground">
              {{ title }}
            </h5>
          </div>

          <div class="text-xs leading-relaxed text-muted-foreground">
            <slot>
              {{ message }}
            </slot>
          </div>

          <div class="flex items-center justify-end gap-2 pt-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              @click="close"
            >
              {{ cancelText || $t('common.cancel') }}
            </Button>
            <Button
              type="button"
              size="sm"
              :variant="variant"
              :disabled="loading"
              @click="$emit('confirm'); close()"
            >
              <Spinner
                v-if="loading"
                class="size-3.5"
              />
              {{ confirmText || $t('common.confirm') }}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </template>
  </Popover>
</template>

<script setup lang="ts">
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Spinner,
} from '@memohai/ui'

withDefaults(defineProps<{
  title?: string
  message?: string
  confirmText?: string
  cancelText?: string
  loading?: boolean
  variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link'
}>(), {
  title: '',
  message: '',
  confirmText: '',
  cancelText: '',
  loading: false,
  variant: 'default',
})

defineEmits<{
  confirm: []
}>()
</script>
