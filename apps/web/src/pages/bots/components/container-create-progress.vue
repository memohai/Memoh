<script setup lang="ts">
import { Spinner } from '@memoh/ui'

defineProps<{
  phase: 'preserving' | 'pulling' | 'creating' | 'restoring' | 'complete' | 'error'
  percent: number
  error?: string
}>()
</script>

<template>
  <div class="flex items-center gap-2 text-sm text-muted-foreground">
    <Spinner class="size-3.5" />
    <span v-if="phase === 'preserving'">
      {{ $t('bots.container.preservingData') }}
    </span>
    <span v-else-if="phase === 'pulling'">
      {{ $t('bots.container.pullingImage') }}
      <span
        v-if="percent > 0"
        class="tabular-nums"
      >{{ percent }}%</span>
    </span>
    <span v-else-if="phase === 'creating'">
      {{ $t('bots.container.creatingContainer') }}
    </span>
    <span v-else-if="phase === 'restoring'">
      {{ $t('bots.container.restoringData') }}
    </span>
    <span
      v-else-if="phase === 'error'"
      class="text-destructive"
    >
      {{ error }}
    </span>
  </div>
  <div class="h-2 w-full overflow-hidden rounded-full bg-muted">
    <div
      v-if="phase === 'pulling'"
      class="h-full rounded-full bg-primary transition-all duration-300 ease-out"
      :style="{ width: `${percent}%` }"
    />
    <div
      v-else-if="phase === 'creating' || phase === 'restoring' || phase === 'preserving'"
      class="h-full w-full animate-pulse rounded-full bg-primary/60"
    />
  </div>
</template>
