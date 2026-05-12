<template>
  <div class="flex flex-wrap gap-2">
    <template
      v-for="(att, i) in block.attachments"
      :key="i"
    >
      <!-- Image / video thumbnail -->
      <button
        v-if="(isImage(att) || isVideo(att)) && getUrl(att)"
        type="button"
        class="inline-flex max-w-64 max-h-72 rounded-lg overflow-hidden hover:ring-2 ring-primary/40 transition-all cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary/40"
        @click="handleMediaClick(att)"
      >
        <img
          v-if="isImage(att)"
          :src="getUrl(att)"
          :alt="String(att.name ?? 'image')"
          class="block max-w-64 max-h-72 w-auto h-auto object-contain pointer-events-none"
          loading="eager"
        >
        <video
          v-else
          :src="getUrl(att)"
          class="block max-w-64 max-h-72 w-auto h-auto object-contain pointer-events-none"
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

      <!-- Container file attachment — open in file manager -->
      <button
        v-else-if="getContainerPath(att)"
        type="button"
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 hover:bg-muted/60 transition-colors text-xs cursor-pointer"
        :title="getContainerPath(att)"
        @click="handleOpenContainerFile(att)"
      >
        <component
          :is="fileIconComponent(att)"
          class="size-4 text-muted-foreground"
        />
        <span class="truncate max-w-[200px] font-mono text-xs">
          {{ getDisplayName(att) }}
        </span>
        <ExternalLink class="size-3 text-muted-foreground/60 shrink-0" />
      </button>

      <!-- Downloadable file -->
      <a
        v-else-if="getUrl(att)"
        :href="getUrl(att)"
        target="_blank"
        rel="noopener noreferrer"
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 hover:bg-muted/60 transition-colors text-xs"
      >
        <component
          :is="fileIconComponent(att)"
          class="size-4 text-muted-foreground"
        />
        <span class="truncate max-w-[200px]">
          {{ String(att.name ?? 'file') }}
        </span>
      </a>

      <!-- Non-accessible attachment -->
      <div
        v-else
        class="flex items-center gap-2 px-3 py-2 rounded-lg border bg-muted/30 text-xs text-muted-foreground"
      >
        <component
          :is="fileIconComponent(att)"
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
import { Music, Video, File as FileIcon, ExternalLink } from 'lucide-vue-next'
import type { Component } from 'vue'
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
  const direct = String(att.path ?? '').trim()
  if (direct) return direct
  const meta = att.metadata as Record<string, unknown> | undefined
  return String(meta?.source_path ?? '').trim()
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

function fileIconComponent(att: AttachmentItem): Component {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'audio' || type === 'voice') return Music
  if (type === 'video') return Video
  return FileIcon
}
</script>
