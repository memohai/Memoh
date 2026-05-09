<script setup lang="ts">
import type { VariantProps } from 'class-variance-authority'
import type { HTMLAttributes } from 'vue'
import { cva } from 'class-variance-authority'
import { computed } from 'vue'
import { cn } from '#/lib/utils'

const badgeCountVariants = cva(
  'inline-flex items-center justify-center rounded-full h-[18px] min-w-[18px] px-1 text-[11px] font-medium font-sans tabular-nums',
  {
    variants: {
      variant: {
        default: 'border border-border bg-secondary text-secondary-foreground',
        destructive: 'bg-destructive text-destructive-foreground',
        secondary: 'bg-accent text-foreground',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

type BadgeCountVariants = VariantProps<typeof badgeCountVariants>

const props = defineProps<{
  count: number
  max?: number
  variant?: BadgeCountVariants['variant']
  class?: HTMLAttributes['class']
}>()

const displayCount = computed(() => {
  const max = props.max ?? 99
  return props.count > max ? `${max}+` : `${props.count}`
})
</script>

<template>
  <span
    data-slot="badge-count"
    :class="cn(badgeCountVariants({ variant }), props.class)"
  >
    {{ displayCount }}
  </span>
</template>
