<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import {
  Button,
  Input,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Spinner,
} from '@memoh/ui'
import {
  getBotsByBotIdContainerFsList,
  postBotsByBotIdContainerFsUpload,
  postBotsByBotIdContainerFsMkdir,
  postBotsByBotIdContainerFsDelete,
  postBotsByBotIdContainerFsRename,
} from '@memoh/sdk'
import type { HandlersFsFileInfo } from '@memoh/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { pathSegments, joinPath } from './utils'
import FileList from './file-list.vue'
import FileViewer from './file-viewer.vue'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const currentPath = ref('/data')
const entries = ref<HandlersFsFileInfo[]>([])
const listLoading = ref(false)
const openFile = ref<HandlersFsFileInfo | null>(null)

const mkdirDialogOpen = ref(false)
const mkdirName = ref('')
const mkdirLoading = ref(false)

const renameDialogOpen = ref(false)
const renameTarget = ref<HandlersFsFileInfo | null>(null)
const renameNewName = ref('')
const renameLoading = ref(false)

const deleteDialogOpen = ref(false)
const deleteTarget = ref<HandlersFsFileInfo | null>(null)
const deleteLoading = ref(false)

const uploadInputRef = ref<HTMLInputElement>()

async function loadDirectory(path: string) {
  listLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerFsList({
      path: { bot_id: props.botId },
      query: { path },
      throwOnError: true,
    })
    entries.value = data.entries ?? []
    currentPath.value = data.path ?? path
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.loadFailed')))
  } finally {
    listLoading.value = false
  }
}

function handleNavigate(path: string) {
  openFile.value = null
  void loadDirectory(path)
}

function handleOpenFile(entry: HandlersFsFileInfo) {
  openFile.value = entry
}

function handleCloseViewer() {
  openFile.value = null
}

function handleFileSaved() {
  void loadDirectory(currentPath.value)
}

// Upload
function triggerUpload() {
  uploadInputRef.value?.click()
}

async function handleUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  const destPath = joinPath(currentPath.value, file.name)
  try {
    await postBotsByBotIdContainerFsUpload({
      path: { bot_id: props.botId },
      body: { path: destPath, file } as never,
      throwOnError: true,
    })
    toast.success(t('bots.files.uploadSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.uploadFailed')))
  } finally {
    input.value = ''
  }
}

// Mkdir
function openMkdirDialog() {
  mkdirName.value = ''
  mkdirDialogOpen.value = true
}

async function handleMkdir() {
  const name = mkdirName.value.trim()
  if (!name || mkdirLoading.value) return

  mkdirLoading.value = true
  try {
    await postBotsByBotIdContainerFsMkdir({
      path: { bot_id: props.botId },
      body: { path: joinPath(currentPath.value, name) },
      throwOnError: true,
    })
    mkdirDialogOpen.value = false
    toast.success(t('bots.files.mkdirSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.mkdirFailed')))
  } finally {
    mkdirLoading.value = false
  }
}

// Rename
function openRenameDialog(entry: HandlersFsFileInfo) {
  renameTarget.value = entry
  renameNewName.value = entry.name ?? ''
  renameDialogOpen.value = true
}

async function handleRename() {
  const target = renameTarget.value
  const newName = renameNewName.value.trim()
  if (!target || !newName || renameLoading.value) return

  renameLoading.value = true
  try {
    await postBotsByBotIdContainerFsRename({
      path: { bot_id: props.botId },
      body: {
        oldPath: target.path,
        newPath: joinPath(currentPath.value, newName),
      },
      throwOnError: true,
    })
    renameDialogOpen.value = false
    if (openFile.value?.path === target.path) {
      openFile.value = null
    }
    toast.success(t('bots.files.renameSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.renameFailed')))
  } finally {
    renameLoading.value = false
  }
}

// Delete
function openDeleteDialog(entry: HandlersFsFileInfo) {
  deleteTarget.value = entry
  deleteDialogOpen.value = true
}

async function handleDelete() {
  const target = deleteTarget.value
  if (!target || deleteLoading.value) return

  deleteLoading.value = true
  try {
    await postBotsByBotIdContainerFsDelete({
      path: { bot_id: props.botId },
      body: { path: target.path, recursive: target.isDir },
      throwOnError: true,
    })
    deleteDialogOpen.value = false
    if (openFile.value?.path === target.path) {
      openFile.value = null
    }
    toast.success(t('bots.files.deleteSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.deleteFailed')))
  } finally {
    deleteLoading.value = false
  }
}

// Download
function handleDownload(entry: HandlersFsFileInfo) {
  const url = `/api/bots/${props.botId}/container/fs/download?path=${encodeURIComponent(entry.path ?? '')}`
  const a = document.createElement('a')
  a.href = url
  a.download = entry.name ?? 'file'
  a.click()
}

watch(() => props.botId, () => {
  openFile.value = null
  currentPath.value = '/data'
  void loadDirectory('/data')
}, { immediate: true })
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden">
    <!-- Toolbar -->
    <div class="flex items-center gap-2 border-b border-border px-4 py-2">
      <!-- Breadcrumb -->
      <nav class="flex min-w-0 flex-1 items-center gap-1 text-sm">
        <template
          v-for="(seg, idx) in pathSegments(currentPath)"
          :key="seg.path"
        >
          <FontAwesomeIcon
            v-if="idx > 0"
            :icon="['fas', 'chevron-right']"
            class="size-2.5 shrink-0 text-muted-foreground"
          />
          <button
            class="truncate rounded px-1.5 py-0.5 hover:bg-muted transition-colors"
            :class="idx === pathSegments(currentPath).length - 1 ? 'font-medium text-foreground' : 'text-muted-foreground'"
            @click="handleNavigate(seg.path)"
          >
            <FontAwesomeIcon
              v-if="idx === 0"
              :icon="['fas', 'folder']"
              class="mr-1 size-3"
            />
            {{ seg.name }}
          </button>
        </template>
      </nav>

      <!-- Actions -->
      <div class="flex items-center gap-1.5">
        <input
          ref="uploadInputRef"
          type="file"
          class="hidden"
          @change="handleUpload"
        >
        <Button
          variant="outline"
          size="sm"
          @click="triggerUpload"
        >
          <FontAwesomeIcon
            :icon="['fas', 'upload']"
            class="mr-1.5 size-3"
          />
          {{ t('bots.files.upload') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          @click="openMkdirDialog"
        >
          <FontAwesomeIcon
            :icon="['fas', 'folder-plus']"
            class="mr-1.5 size-3"
          />
          {{ t('bots.files.newFolder') }}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          class="size-8 p-0"
          :disabled="listLoading"
          @click="() => loadDirectory(currentPath)"
        >
          <FontAwesomeIcon
            :icon="['fas', 'rotate']"
            class="size-3.5"
            :class="{ 'animate-spin': listLoading }"
          />
        </Button>
      </div>
    </div>

    <!-- Main content area -->
    <div class="flex flex-1 min-h-0 overflow-hidden">
      <!-- File list -->
      <div
        class="overflow-auto border-border transition-all"
        :class="openFile ? 'w-80 shrink-0 border-r' : 'w-full'"
      >
        <FileList
          :entries="entries"
          :loading="listLoading"
          @navigate="handleNavigate"
          @open="handleOpenFile"
          @download="handleDownload"
          @rename="openRenameDialog"
          @delete="openDeleteDialog"
        />
      </div>

      <!-- File viewer -->
      <div
        v-if="openFile"
        class="flex-1 overflow-hidden"
      >
        <FileViewer
          :bot-id="botId"
          :file="openFile"
          @close="handleCloseViewer"
          @saved="handleFileSaved"
        />
      </div>
    </div>

    <!-- Mkdir dialog -->
    <Dialog v-model:open="mkdirDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('bots.files.newFolder') }}</DialogTitle>
        </DialogHeader>
        <Input
          v-model="mkdirName"
          :placeholder="t('bots.files.folderNamePlaceholder')"
          :disabled="mkdirLoading"
          @keydown.enter.prevent="handleMkdir"
        />
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="mkdirLoading"
            @click="mkdirDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            :disabled="!mkdirName.trim() || mkdirLoading"
            @click="handleMkdir"
          >
            <Spinner
              v-if="mkdirLoading"
              class="mr-1"
            />
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Rename dialog -->
    <Dialog v-model:open="renameDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('bots.files.rename') }}</DialogTitle>
        </DialogHeader>
        <Input
          v-model="renameNewName"
          :placeholder="t('bots.files.newNamePlaceholder')"
          :disabled="renameLoading"
          @keydown.enter.prevent="handleRename"
        />
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="renameLoading"
            @click="renameDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            :disabled="!renameNewName.trim() || renameLoading"
            @click="handleRename"
          >
            <Spinner
              v-if="renameLoading"
              class="mr-1"
            />
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete dialog -->
    <Dialog v-model:open="deleteDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('bots.files.confirmDelete') }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm text-muted-foreground">
          {{ t('bots.files.confirmDeleteMessage', { name: deleteTarget?.name ?? '' }) }}
        </p>
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="deleteLoading"
            @click="deleteDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deleteLoading"
            @click="handleDelete"
          >
            <Spinner
              v-if="deleteLoading"
              class="mr-1"
            />
            {{ t('bots.files.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
