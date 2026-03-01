<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@memoh/ui'
import type { HandlersFsFileInfo } from '@memoh/sdk'
import { formatFileSize, formatRelativeTime } from './utils'

const props = defineProps<{
  entries: HandlersFsFileInfo[]
  loading?: boolean
}>()

const emit = defineEmits<{
  navigate: [path: string]
  open: [entry: HandlersFsFileInfo]
  download: [entry: HandlersFsFileInfo]
  rename: [entry: HandlersFsFileInfo]
  delete: [entry: HandlersFsFileInfo]
}>()

const { t } = useI18n()

const sortedEntries = computed(() => {
  const dirs = props.entries
    .filter(e => e.isDir)
    .sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
  const files = props.entries
    .filter(e => !e.isDir)
    .sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
  return [...dirs, ...files]
})

function handleClick(entry: HandlersFsFileInfo) {
  if (entry.isDir) {
    emit('navigate', entry.path ?? '')
  } else {
    emit('open', entry)
  }
}
</script>

<template>
  <div class="w-full">
    <div
      v-if="loading"
      class="flex items-center justify-center py-16 text-muted-foreground"
    >
      <FontAwesomeIcon
        :icon="['fas', 'spinner']"
        class="mr-2 size-4 animate-spin"
      />
      {{ t('common.loading') }}
    </div>

    <div
      v-else-if="sortedEntries.length === 0"
      class="flex flex-col items-center justify-center py-16 text-muted-foreground"
    >
      <FontAwesomeIcon
        :icon="['fas', 'folder-open']"
        class="mb-2 size-8 opacity-40"
      />
      <span>{{ t('bots.files.empty') }}</span>
    </div>

    <div v-else>
      <!-- Header row -->
      <div class="flex items-center border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground">
        <div class="flex-1">
          {{ t('bots.files.name') }}
        </div>
        <div class="hidden w-20 text-right sm:block">
          {{ t('bots.files.size') }}
        </div>
        <div class="hidden w-28 text-right md:block">
          {{ t('bots.files.modified') }}
        </div>
      </div>

      <!-- File rows -->
      <ContextMenu
        v-for="entry in sortedEntries"
        :key="entry.path"
      >
        <ContextMenuTrigger as-child>
          <div
            class="flex items-center border-b border-border/50 cursor-pointer px-3 py-2 text-sm transition-colors hover:bg-muted/50"
            @click="handleClick(entry)"
          >
            <div class="flex flex-1 items-center gap-2 min-w-0">
              <FontAwesomeIcon
                :icon="entry.isDir ? ['fas', 'folder'] : ['fas', 'file']"
                :class="entry.isDir ? 'text-blue-500' : 'text-muted-foreground'"
                class="size-4 shrink-0"
              />
              <span class="truncate">{{ entry.name }}</span>
            </div>
            <div class="hidden w-20 shrink-0 text-right text-muted-foreground sm:block">
              {{ entry.isDir ? '' : formatFileSize(entry.size) }}
            </div>
            <div class="hidden w-28 shrink-0 text-right text-muted-foreground md:block">
              {{ formatRelativeTime(entry.modTime) }}
            </div>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem
            v-if="!entry.isDir"
            @select="emit('download', entry)"
          >
            <FontAwesomeIcon
              :icon="['fas', 'download']"
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.download') }}
          </ContextMenuItem>
          <ContextMenuItem @select="emit('rename', entry)">
            <FontAwesomeIcon
              :icon="['fas', 'pen']"
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.rename') }}
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem
            class="text-destructive focus:text-destructive"
            @select="emit('delete', entry)"
          >
            <FontAwesomeIcon
              :icon="['fas', 'trash']"
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.delete') }}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  </div>
</template>
