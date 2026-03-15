<template>
  <div class="flex flex-wrap gap-2">
    <template
      v-for="(att, i) in block.attachments"
      :key="i"
    >
      <!-- Image / video thumbnail -->
      <button
        v-if="isImage(att) || isVideo(att)"
        type="button"
        class="block w-48 h-48 rounded-lg overflow-hidden border bg-muted/20 hover:ring-2 ring-primary/40 transition-all cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary/40"
        @click="handleMediaClick(att)"
      >
        <img
          v-if="isImage(att)"
          :src="getUrl(att)"
          :alt="String(att.name ?? 'image')"
          class="w-full h-full object-contain pointer-events-none"
          loading="eager"
          width="192"
          height="192"
        >
        <video
          v-else
          :src="getUrl(att)"
          class="w-full h-full object-contain pointer-events-none"
          preload="metadata"
          muted
          playsinline
        />
      </button>

      <!-- Audio player -->
      <div
        v-else-if="isAudio(att) && getUrl(att)"
        class="rounded-lg border bg-muted/30 px-3 py-2 min-w-[280px] max-w-[400px]"
      >
        <audio
          controls
          preload="metadata"
          class="w-full"
          :src="getUrl(att)"
        />
      </div>

      <!-- Downloadable file -->
      <a
        v-else-if="getUrl(att)"
        :href="getUrl(att)"
        target="_blank"
        rel="noopener noreferrer"
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 hover:bg-muted/60 transition-colors text-sm"
      >
        <FontAwesomeIcon
          :icon="['fas', fileIcon(att)]"
          class="size-4 text-muted-foreground"
        />
        <span class="truncate max-w-[200px]">
          {{ String(att.name ?? 'file') }}
        </span>
      </a>

      <!-- Container file attachment — open in file manager -->
      <button
        v-else-if="getContainerPath(att)"
        type="button"
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 hover:bg-muted/60 transition-colors text-sm cursor-pointer"
        :title="getContainerPath(att)"
        @click="handleOpenContainerFile(att)"
      >
        <FontAwesomeIcon
          :icon="['fas', fileIcon(att)]"
          class="size-4 text-muted-foreground"
        />
        <span class="truncate max-w-[200px] font-mono text-xs">
          {{ getDisplayName(att) }}
        </span>
        <FontAwesomeIcon
          :icon="['fas', 'arrow-up-right-from-square']"
          class="size-3 text-muted-foreground/60 shrink-0"
        />
      </button>

      <!-- Non-accessible attachment -->
      <div
        v-else
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 text-sm text-muted-foreground"
      >
        <FontAwesomeIcon
          :icon="['fas', fileIcon(att)]"
          class="size-4"
        />
        <span class="truncate max-w-[200px]">
          {{ String(att.name ?? att.storage_key ?? 'attachment') }}
        </span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import type { AttachmentBlock, AttachmentItem } from '@/store/chat-list'
import { resolveUrl } from '../composables/useMediaGallery'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'

const props = defineProps<{
  block: AttachmentBlock
  onOpenMedia?: (src: string) => void
}>()

const openInFileManager = inject(openInFileManagerKey, undefined)

function getUrl(att: AttachmentItem): string {
  return resolveUrl(att)
}

function isImage(att: AttachmentItem): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'image' || type === 'gif') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('image/')
}

function isVideo(att: AttachmentItem): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'video') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('video/')
}

function isAudio(att: Record<string, unknown>): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'audio' || type === 'voice') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('audio/')
}

function getContainerPath(att: AttachmentItem): string {
  return String(att.path ?? '').trim()
}

function getDisplayName(att: AttachmentItem): string {
  if (att.name) return String(att.name)
  const p = getContainerPath(att)
  if (p) return p.split('/').pop() || p
  return 'file'
}

function handleMediaClick(att: AttachmentItem) {
  const src = getUrl(att)
  if (src && props.onOpenMedia) {
    props.onOpenMedia(src)
  }
}

function handleOpenContainerFile(att: AttachmentItem) {
  const path = getContainerPath(att)
  if (path && openInFileManager) {
    openInFileManager(path, false)
  }
}

function fileIcon(att: AttachmentItem): string {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'audio' || type === 'voice') return 'music'
  if (type === 'video') return 'video'
  return 'file'
}
</script>
