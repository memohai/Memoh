<template>
  <Popover>
    <template #default="{ close }">
      <PopoverTrigger as-child>
        <slot name="trigger" />
      </PopoverTrigger>
      <!-- Inherit the shared popover chrome (menu-shell radius, --border-menu
           hairline, dropdown shadow, p-4) instead of overriding it — this is the
           same surface as DropdownMenu / Select, so a confirm reads as part of
           the one menu language. This is an anchored popover, NOT a modal dialog:
           keep it compact (the question is allowed to wrap to ~2 lines) and keep
           the inherited p-4 edge inset so text never touches the border. -->
      <PopoverContent class="w-72 max-w-[calc(100vw-2rem)]">
        <div class="space-y-3">
          <!-- The core question is the strongest line in a confirm: it reads as a
               title (text-sm / medium / foreground), never as muted caption text.
               With only a message, the message *is* the question; pass a title too
               and the message drops to the supporting line beneath it. -->
          <div class="flex items-start gap-2">
            <span
              v-if="$slots.icon"
              class="mt-0.5 shrink-0"
            >
              <slot name="icon" />
            </span>
            <p class="min-w-0 text-sm font-medium text-foreground">
              <template v-if="title">
                {{ title }}
              </template>
              <slot v-else>
                {{ message }}
              </slot>
            </p>
          </div>

          <p
            v-if="title && (message || !!$slots.default)"
            class="text-xs leading-relaxed text-muted-foreground"
          >
            <slot>{{ message }}</slot>
          </p>

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
              :loading="loading"
              @click="$emit('confirm'); close()"
            >
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
