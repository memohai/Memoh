<template>
  <Button
    v-bind="$attrs"
    :disabled="loading || disabled"
    :variant="variant"
    :size="size"
  >
    <Spinner
      v-if="loading"
      class="size-4"
    />
    <slot />
  </Button>
</template>

<script setup lang="ts">
import { Button, Spinner } from '@memohai/ui'
import type { ButtonVariants } from '@memohai/ui'

defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{
  loading?: boolean
  disabled?: boolean
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
}>(), {
  loading: false,
  disabled: false,
  variant: 'default',
  size: 'default',
})
</script>
