<template>
  <Popover>
    <template #default="{ close }">
      <PopoverTrigger as-child>
        <slot name="trigger" />
      </PopoverTrigger>
      <PopoverContent class="w-80 p-0 overflow-hidden border-border shadow-xl">
        <div class="p-4 space-y-3">
          <div
            v-if="title"
            class="flex items-center gap-2 mb-1"
          >
            <div
              v-if="$slots.icon"
              class="shrink-0"
            >
              <slot name="icon" />
            </div>
            <h5 class="text-xs font-bold text-foreground">
              {{ title }}
            </h5>
          </div>
          
          <div class="text-[11px] text-muted-foreground leading-relaxed">
            <slot>
              {{ message }}
            </slot>
          </div>
        </div>

        <div class="flex items-center justify-end gap-2 px-4 py-3 bg-muted/30 border-t border-border/50">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            class="h-8 text-[11px] font-medium"
            @click="close"
          >
            {{ cancelText || $t('common.cancel') }}
          </Button>
          <Button
            type="button"
            size="sm"
            class="h-8 text-[11px] font-bold shadow-sm"
            :variant="variant"
            :disabled="loading"
            @click="$emit('confirm'); close()"
          >
            <Spinner
              v-if="loading"
              class="size-3 mr-1"
            />
            {{ confirmText || $t('common.confirm') }}
          </Button>
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
  variant: 'default'
})

defineEmits<{
  confirm: []
}>()
</script>
