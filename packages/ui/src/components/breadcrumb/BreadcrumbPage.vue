<script lang="ts" setup>
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <span
    data-slot="breadcrumb-page"
    role="link"
    aria-disabled="true"
    aria-current="page"
    :class="cn('text-foreground font-normal inline-flex items-center gap-1.5 rounded-sm px-1.5 py-[0.3125rem] leading-none [&>svg]:size-3 [&>svg]:shrink-0 [&>svg]:text-muted-foreground', props.class)"
  >
    <slot />
  </span>
</template>
