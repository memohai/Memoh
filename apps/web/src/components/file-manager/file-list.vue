<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArchiveRestore, LoaderCircle, FolderOpen, Folder, File, Download, SquarePen, Trash2 } from 'lucide-vue-next'
import {
  Checkbox,
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@memohai/ui'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import { formatFileSize, formatRelativeTime, isArchiveFile } from './utils'

const props = defineProps<{
  entries: HandlersFsFileInfo[]
  loading?: boolean
  selectedPaths?: Set<string>
  selectionMode?: boolean
  selectionDisabled?: boolean
  canWrite?: boolean
}>()

const emit = defineEmits<{
  navigate: [path: string]
  open: [entry: HandlersFsFileInfo]
  download: [entry: HandlersFsFileInfo]
  extract: [entry: HandlersFsFileInfo]
  rename: [entry: HandlersFsFileInfo]
  delete: [entry: HandlersFsFileInfo]
  toggleSelect: [entry: HandlersFsFileInfo, selected: boolean]
  selectAll: [selected: boolean]
}>()

const { t } = useI18n()

type CheckboxState = boolean | 'indeterminate'

const sortedEntries = computed(() => {
  const dirs = props.entries
    .filter(e => e.isDir)
    .sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
  const files = props.entries
    .filter(e => !e.isDir)
    .sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
  return [...dirs, ...files]
})

const selectableEntries = computed(() => sortedEntries.value.filter(entry => !!entry.path))

const selectedCount = computed(() => selectableEntries.value.filter(entry => isSelected(entry)).length)

const allSelectedState = computed(() => {
  if (selectableEntries.value.length === 0 || selectedCount.value === 0) return false
  if (selectedCount.value === selectableEntries.value.length) return true
  return 'indeterminate'
})

function handleClick(entry: HandlersFsFileInfo) {
  if (props.selectionMode) {
    toggleEntry(entry)
    return
  }

  if (entry.isDir) {
    emit('navigate', entry.path ?? '')
  } else {
    emit('open', entry)
  }
}

function isSelected(entry: HandlersFsFileInfo) {
  return props.selectedPaths?.has(entry.path ?? '') ?? false
}

function setEntrySelected(entry: HandlersFsFileInfo, selected: boolean) {
  if (props.selectionDisabled || !entry.path) return
  emit('toggleSelect', entry, selected)
}

function toggleEntry(entry: HandlersFsFileInfo) {
  setEntrySelected(entry, !isSelected(entry))
}

function toggleAll(checked: CheckboxState) {
  emit('selectAll', checked === true)
}

function handleCheckboxUpdate(entry: HandlersFsFileInfo, checked: CheckboxState) {
  setEntrySelected(entry, checked === true)
}
</script>

<template>
  <div class="w-full">
    <div
      v-if="loading"
      class="flex items-center justify-center py-16 text-muted-foreground"
    >
      <LoaderCircle
        class="mr-2 size-4 animate-spin"
      />
      {{ t('common.loading') }}
    </div>

    <div
      v-else-if="sortedEntries.length === 0"
      class="flex flex-col items-center justify-center py-16 text-muted-foreground"
    >
      <FolderOpen
        class="mb-2 size-8 opacity-40"
      />
      <span>{{ t('bots.files.empty') }}</span>
    </div>

    <div v-else>
      <!-- Header row -->
      <div class="flex items-center border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground">
        <div
          v-if="selectionMode"
          class="mr-2 flex w-5 shrink-0 items-center justify-center"
          @click.stop
        >
          <Checkbox
            :model-value="allSelectedState"
            :disabled="selectionDisabled || selectableEntries.length === 0"
            :aria-label="t('bots.files.selectAll')"
            @update:model-value="toggleAll"
          />
        </div>
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
            class="group flex cursor-pointer items-center border-b border-border/50 px-3 py-2 text-xs transition-colors hover:bg-muted/50"
            @click="handleClick(entry)"
          >
            <div
              v-if="selectionMode"
              class="mr-2 flex w-5 shrink-0 items-center justify-center"
              @click.stop
            >
              <Checkbox
                :model-value="isSelected(entry)"
                :disabled="selectionDisabled || !entry.path"
                :aria-label="t('bots.files.selectItem', { name: entry.name ?? '' })"
                @update:model-value="checked => handleCheckboxUpdate(entry, checked)"
              />
            </div>
            <div class="flex flex-1 items-center gap-2 min-w-0">
              <component
                :is="entry.isDir ? Folder : File"
                :class="entry.isDir ? 'text-info' : 'text-muted-foreground'"
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
          <ContextMenuItem @select="emit('download', entry)">
            <Download
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.download') }}
          </ContextMenuItem>
          <ContextMenuItem
            v-if="canWrite && !entry.isDir && isArchiveFile(entry.name)"
            @select="emit('extract', entry)"
          >
            <ArchiveRestore
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.extract') }}
          </ContextMenuItem>
          <ContextMenuItem
            v-if="canWrite"
            @select="emit('rename', entry)"
          >
            <SquarePen
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.rename') }}
          </ContextMenuItem>
          <ContextMenuSeparator v-if="canWrite" />
          <ContextMenuItem
            v-if="canWrite"
            class="text-destructive focus:text-destructive"
            @select="emit('delete', entry)"
          >
            <Trash2
              class="mr-2 size-3.5"
            />
            {{ t('bots.files.delete') }}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  </div>
</template>
