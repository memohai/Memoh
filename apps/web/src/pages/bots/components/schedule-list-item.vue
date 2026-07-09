<template>
  <div
    class="group/card relative flex cursor-pointer items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card transition-colors hover:bg-accent/30 dark:hover:bg-accent focus-visible:outline-none"
    :class="variant === 'sidebar' ? '' : 'px-4 py-3.5'"
    role="button"
    tabindex="0"
    @click="$emit('open')"
    @keydown.enter="$emit('open')"
    @keydown.space.prevent="$emit('open')"
  >
    <div
      class="min-w-0 flex-1"
      :class="variant === 'sidebar' ? 'px-3 py-2.5' : ''"
    >
      <div class="flex min-w-0 items-center gap-2">
        <p
          class="truncate text-foreground"
          :class="variant === 'sidebar' ? 'text-control font-normal leading-snug' : 'text-sm font-medium'"
        >
          {{ item.name }}
        </p>
        <span
          v-if="timeLabel"
          class="shrink-0 text-caption tabular-nums text-muted-foreground"
        >
          {{ timeLabel }}
        </span>
      </div>
      <p
        class="truncate text-muted-foreground"
        :class="variant === 'sidebar' ? 'mt-0.5 text-caption leading-snug' : 'text-xs'"
      >
        {{ description }}
      </p>
    </div>

    <div
      class="flex shrink-0 items-center gap-2"
      :class="variant === 'sidebar' ? 'pr-1.5' : ''"
    >
      <DropdownMenu
        @update:open="(open: boolean) => { menuOpen = open }"
      >
        <DropdownMenuTrigger as-child>
          <Button
            variant="ghost"
            :size="variant === 'sidebar' ? 'icon-sm' : 'icon'"
            class="transition-opacity"
            :class="[
              variant === 'sidebar' ? 'size-6' : 'size-7',
              menuOpen ? 'opacity-100' : 'opacity-0 group-hover/card:opacity-100',
            ]"
            :aria-label="t('common.actions')"
            @click.stop
          >
            <MoreHorizontal class="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            class="gap-2"
            @select="$emit('edit')"
          >
            <Pencil class="size-3.5" />
            {{ t('bots.schedule.edit') }}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            class="gap-2"
            @select="$emit('delete')"
          >
            <Trash2 class="size-3.5" />
            {{ t('bots.schedule.delete') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Switch
        :model-value="!!item.enabled"
        :disabled="busy"
        :aria-label="t('bots.schedule.form.enabled')"
        @click.stop
        @update:model-value="(value: boolean) => $emit('toggle', !!value)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MoreHorizontal, Pencil, Trash2 } from 'lucide-vue-next'
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Switch,
} from '@felinic/ui'
import type { ScheduleSchedule } from '@memohai/sdk'

withDefaults(defineProps<{
  item: ScheduleSchedule
  description: string
  timeLabel?: string
  busy?: boolean
  variant?: 'card' | 'sidebar'
}>(), {
  timeLabel: '',
  busy: false,
  variant: 'card',
})

defineEmits<{
  open: []
  edit: []
  delete: []
  toggle: [enabled: boolean]
}>()

const { t } = useI18n()
const menuOpen = ref(false)
</script>
