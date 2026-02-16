import { computed, ref, type Ref } from 'vue'
import type { ChatMessage } from '@/store/chat-list'
import type { MediaGalleryItem } from '../components/media-gallery-lightbox.vue'

function isMediaType(att: Record<string, unknown>): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'image' || type === 'gif' || type === 'video') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('image/') || mime.startsWith('video/')
}

function resolveUrl(att: Record<string, unknown>): string {
  const url = String(att.url ?? '').trim()
  if (url) return url
  const assetId = String(att.asset_id ?? '').trim()
  if (!assetId) return ''
  const botId = String(att.bot_id ?? '').trim()
  if (!botId) return ''
  const token = localStorage.getItem('token') || ''
  return `/api/bots/${botId}/media/${assetId}?token=${encodeURIComponent(token)}`
}

function normalizeSrc(src: string): string {
  if (!src || src.startsWith('data:')) return src
  try {
    const u = new URL(src, window.location.origin)
    return u.pathname + u.search
  } catch {
    return src
  }
}

export function useMediaGallery(messages: Ref<ChatMessage[]>) {
  const openIndex = ref<number | null>(null)

  const items = computed((): MediaGalleryItem[] => {
    const result: MediaGalleryItem[] = []
    for (const msg of messages.value) {
      for (const block of msg.blocks) {
        if (block.type !== 'attachment') continue
        for (const att of block.attachments) {
          if (!isMediaType(att)) continue
          const src = resolveUrl(att)
          if (!src) continue
          const type = String(att.type ?? '').toLowerCase()
          result.push({
            src,
            type: type === 'video' ? 'video' : 'image',
            name: String(att.name ?? '').trim() || undefined,
          })
        }
      }
    }
    return result
  })

  function findIndexBySrc(src: string): number {
    const norm = normalizeSrc(src)
    if (!norm) return -1
    return items.value.findIndex((item) => normalizeSrc(item.src) === norm)
  }

  function openBySrc(src: string) {
    const idx = findIndexBySrc(src)
    if (idx >= 0) {
      openIndex.value = idx
    }
  }

  function setOpenIndex(v: number | null) {
    openIndex.value = v
  }

  return {
    items,
    openIndex,
    setOpenIndex,
    openBySrc,
    findIndexBySrc,
  }
}

export { resolveUrl, isMediaType }
