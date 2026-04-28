<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="flex items-center gap-1 border-b border-border px-2 py-1.5 shrink-0">
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0"
        :disabled="loading"
        :title="t('bots.files.upload')"
        @click="triggerUpload"
      >
        <Upload class="size-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0"
        :disabled="loading"
        :title="t('bots.files.newFolder')"
        @click="openMkdirDialog"
      >
        <FolderPlus class="size-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0 ml-auto"
        :disabled="loading"
        :title="t('common.refresh')"
        @click="reload"
      >
        <RefreshCw
          class="size-3.5"
          :class="{ 'animate-spin': loading }"
        />
      </Button>
    </div>

    <div class="flex items-center px-2 py-1.5 shrink-0 overflow-x-auto">
      <nav class="flex min-w-0 items-center gap-0.5 text-[11px]">
        <template
          v-for="(seg, idx) in pathSegments(currentPath)"
          :key="seg.path"
        >
          <ChevronRight
            v-if="idx > 0"
            class="size-2.5 shrink-0 text-muted-foreground"
          />
          <button
            type="button"
            class="inline-flex items-center truncate rounded px-1 py-0.5 hover:bg-muted/60 transition-colors"
            :class="idx === pathSegments(currentPath).length - 1 ? 'font-medium text-foreground' : 'text-muted-foreground'"
            @click="navigateTo(seg.path)"
          >
            <Folder
              v-if="idx === 0"
              class="mr-1 size-3 shrink-0"
            />
            {{ seg.name }}
          </button>
        </template>
      </nav>
    </div>

    <input
      ref="uploadInputRef"
      type="file"
      class="hidden"
      @change="handleUpload"
    >

    <div class="flex-1 min-h-0 relative">
      <ScrollArea class="absolute inset-0">
        <FileList
          :entries="entries"
          :loading="loading"
          @navigate="navigateTo"
          @open="handleOpenFile"
          @download="handleDownload"
          @rename="openRenameDialog"
          @delete="openDeleteDialog"
        />
      </ScrollArea>
    </div>

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

    <Dialog v-model:open="deleteDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('bots.files.confirmDelete') }}</DialogTitle>
        </DialogHeader>
        <p class="text-xs text-muted-foreground">
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

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ChevronRight, Folder, Upload, FolderPlus, RefreshCw } from 'lucide-vue-next'
import {
  Button,
  Input,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Spinner,
  ScrollArea,
} from '@memohai/ui'
import {
  getBotsByBotIdContainerFsList,
  postBotsByBotIdContainerFsUpload,
  postBotsByBotIdContainerFsMkdir,
  postBotsByBotIdContainerFsDelete,
  postBotsByBotIdContainerFsRename,
} from '@memohai/sdk'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { pathSegments, joinPath } from '@/components/file-manager/utils'
import FileList from '@/components/file-manager/file-list.vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const workspaceTabs = useWorkspaceTabsStore()

const currentPath = ref('/data')
const entries = ref<HandlersFsFileInfo[]>([])
const loading = ref(false)
const uploadInputRef = ref<HTMLInputElement>()

async function loadDirectory(path: string) {
  if (!props.botId) return
  loading.value = true
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
    loading.value = false
  }
}

function navigateTo(path: string) {
  void loadDirectory(path)
}

function reload() {
  void loadDirectory(currentPath.value)
}

function handleOpenFile(entry: HandlersFsFileInfo) {
  if (!entry.path) return
  workspaceTabs.openFile(entry.path)
}

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

const mkdirDialogOpen = ref(false)
const mkdirName = ref('')
const mkdirLoading = ref(false)

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

const renameDialogOpen = ref(false)
const renameTarget = ref<HandlersFsFileInfo | null>(null)
const renameNewName = ref('')
const renameLoading = ref(false)

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
    toast.success(t('bots.files.renameSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.renameFailed')))
  } finally {
    renameLoading.value = false
  }
}

const deleteDialogOpen = ref(false)
const deleteTarget = ref<HandlersFsFileInfo | null>(null)
const deleteLoading = ref(false)

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
    toast.success(t('bots.files.deleteSuccess'))
    void loadDirectory(currentPath.value)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.files.deleteFailed')))
  } finally {
    deleteLoading.value = false
  }
}

function handleDownload(entry: HandlersFsFileInfo) {
  const token = localStorage.getItem('token') ?? ''
  const url = `/api/bots/${props.botId}/container/fs/download?path=${encodeURIComponent(entry.path ?? '')}&token=${encodeURIComponent(token)}`
  const a = document.createElement('a')
  a.href = url
  a.download = entry.name ?? 'file'
  a.click()
}

watch(() => props.botId, () => {
  void loadDirectory(currentPath.value)
}, { immediate: true })

defineExpose({
  navigateTo,
  reload,
})
</script>
