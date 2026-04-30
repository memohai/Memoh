<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { NodeProps } from '@vue-flow/core'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@memohai/ui'
import {
  statusClass,
  statusShortLabel,
} from '../model'

const { t } = useI18n()

interface TaskNodeData {
  title: string
  label: string
  worker: string
  status: string
  goal: string
  resultSummary: string
  attemptLabel: string
  isRoot: boolean
  checkpointLabel: string
}

const props = defineProps<NodeProps<TaskNodeData>>()
</script>

<template>
  <Tooltip :delay-duration="120">
    <TooltipTrigger as-child>
      <div
        class="rounded-lg border bg-card/96 shadow-sm transition-all"
        :class="[
          props.selected
            ? 'border-foreground/20 ring-2 ring-foreground/10'
            : 'border-border/60',
        ]"
      >
        <div class="flex min-w-[160px] max-w-[168px] flex-col gap-1.5 px-2.5 py-2">
          <div class="flex items-start justify-between gap-1.5">
            <div class="min-w-0">
              <p
                v-if="props.data.isRoot"
                class="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground"
              >
                {{ t('orchestration.root') }}
              </p>
              <p class="truncate text-xs font-medium leading-tight text-foreground">
                {{ props.data.label }}
              </p>
            </div>
            <span
              class="shrink-0 rounded border px-1 py-0.5 text-[9px] font-medium leading-none"
              :class="statusClass(props.data.status)"
            >
              {{ statusShortLabel(props.data.status) }}
            </span>
          </div>

          <div class="flex items-center justify-between gap-1.5 text-[10px] text-muted-foreground">
            <span class="min-w-0 truncate">{{ props.data.worker }}</span>
            <span class="shrink-0 tabular-nums">{{ props.data.attemptLabel }}</span>
          </div>
        </div>
      </div>
    </TooltipTrigger>

    <TooltipContent
      side="top"
      :side-offset="8"
      class="max-w-80 border border-border/60 bg-popover/96 p-0 text-xs shadow-lg"
    >
      <div class="space-y-2 px-3 py-2.5">
        <div>
          <p class="font-medium text-foreground">
            {{ props.data.title }}
          </p>
          <p class="mt-1 line-clamp-4 break-words text-[11px] text-muted-foreground">
            {{ props.data.goal }}
          </p>
        </div>

        <div class="grid grid-cols-[4.5rem_minmax(0,1fr)] gap-x-2 gap-y-0.5 text-[11px]">
          <span class="text-muted-foreground">{{ t('orchestration.status') }}</span>
          <span class="text-foreground">{{ props.data.status }}</span>
          <span class="text-muted-foreground">{{ t('orchestration.workerProfile') }}</span>
          <span class="break-all text-foreground">{{ props.data.worker }}</span>
          <span class="text-muted-foreground">{{ t('orchestration.attempts') }}</span>
          <span>{{ props.data.attemptLabel }}</span>
          <span class="text-muted-foreground">{{ t('orchestration.checkpoints') }}</span>
          <span>{{ props.data.checkpointLabel }}</span>
        </div>
      </div>
    </TooltipContent>
  </Tooltip>
</template>
