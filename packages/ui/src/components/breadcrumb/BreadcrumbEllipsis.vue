<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { MoreHorizontal } from 'lucide-vue-next'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <span
    data-slot="breadcrumb-ellipsis"
    role="presentation"
    aria-hidden="true"
    :class="cn('inline-flex size-6 items-center justify-center rounded-sm text-muted-foreground [&>svg]:size-3', props.class)"
  >
    <slot>
      <MoreHorizontal />
    </slot>
    <span class="sr-only">More</span>
  </span>
</template>
