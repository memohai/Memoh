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
          loading="lazy"
        />
        <video
          v-else
          :src="getUrl(att)"
          class="w-full h-full object-contain pointer-events-none"
          preload="metadata"
          muted
          playsinline
        />
      </button>

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
import type { AttachmentBlock } from '@/store/chat-list'
import { resolveUrl, isMediaType } from '../composables/useMediaGallery'

const props = defineProps<{
  block: AttachmentBlock
  onOpenMedia?: (src: string) => void
}>()

function getUrl(att: Record<string, unknown>): string {
  return resolveUrl(att)
}

function isImage(att: Record<string, unknown>): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'image' || type === 'gif') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('image/')
}

function isVideo(att: Record<string, unknown>): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'video') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('video/')
}

function handleMediaClick(att: Record<string, unknown>) {
  const src = getUrl(att)
  if (src && props.onOpenMedia) {
    props.onOpenMedia(src)
  }
}

function fileIcon(att: Record<string, unknown>): string {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'audio' || type === 'voice') return 'music'
  if (type === 'video') return 'video'
  return 'file'
}
</script>
