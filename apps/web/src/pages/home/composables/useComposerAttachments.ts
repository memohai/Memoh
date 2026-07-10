import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { ChatAttachment } from '@/composables/api/useChat'
import type { MediaGalleryItem } from '../components/media-gallery-lightbox.vue'

// Composer attachment tray: pending File list, paste-to-attachment capture,
// object-URL previews, async line counts, the reveal/collapse animation
// window, and File <-> ChatAttachment conversion. Owns its object-URL and
// timer lifecycles via its own scope-dispose hooks, so the composer that
// mounts it never has to remember to clean up.

// Pasting a large block of text floods the composer and buries the controls, so
// past a threshold we capture it as a "pasted content" attachment card instead
// (the raw text still rides along as a .txt file on send). The trigger is set
// deliberately high so ordinary multi-line snippets keep landing in the input.
const PASTE_LINE_THRESHOLD = 50
const PASTE_CHAR_THRESHOLD = 2000
const PASTED_FILE_NAME = 'pasted-text.txt'

// Text-like files get a line count on their card (e.g. a pasted snippet or a
// .yml config), mirroring how a code block reads. Binary blobs are skipped so
// we never surface a meaningless newline tally for a PDF or archive.
const TEXT_EXTENSIONS = new Set([
  'txt', 'md', 'markdown', 'json', 'jsonc', 'yaml', 'yml', 'xml', 'csv', 'tsv',
  'log', 'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'vue', 'py', 'go', 'rs', 'java',
  'c', 'cc', 'cpp', 'h', 'hpp', 'css', 'scss', 'less', 'html', 'svg', 'sh', 'bash',
  'zsh', 'toml', 'ini', 'conf', 'env', 'sql', 'rb', 'php', 'swift', 'kt', 'gradle',
])
const LINE_COUNT_MAX_BYTES = 2 * 1024 * 1024

// Attachment row reveal/collapse timing (the grid 0fr↔1fr transition).
// Exported: the composer's radius morph syncs its duration to this.
export const ATTACHMENT_ANIM_MS = 230

export async function fileToAttachment(file: File): Promise<ChatAttachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      resolve({
        type: file.type.startsWith('image/') ? 'image' : 'file',
        base64: reader.result as string,
        mime: file.type || 'application/octet-stream',
        name: file.name,
      })
    }
    reader.onerror = () => reject(new Error('Failed to read file'))
    reader.readAsDataURL(file)
  })
}

export function attachmentToFile(attachment: ChatAttachment): File | null {
  const source = attachment.base64?.trim()
  if (!source) return null
  try {
    const commaIndex = source.indexOf(',')
    const payload = commaIndex >= 0 ? source.slice(commaIndex + 1) : source
    const meta = commaIndex >= 0 ? source.slice(0, commaIndex) : ''
    const bytes = atob(payload)
    const buffer = new Uint8Array(bytes.length)
    for (let i = 0; i < bytes.length; i += 1) {
      buffer[i] = bytes.charCodeAt(i)
    }
    const inferredMime = meta.match(/^data:([^;,]+)/)?.[1]
    return new File([buffer], attachment.name?.trim() || 'attachment', {
      type: attachment.mime?.trim() || inferredMime || 'application/octet-stream',
    })
  } catch {
    return null
  }
}

export function useComposerAttachments() {
  const fileInput = ref<HTMLInputElement | null>(null)
  const pendingFiles = ref<File[]>([])

  // Original text for each pasted-content file, so its card can preview the body
  // and the viewer can show it in full without re-reading the synthetic File.
  const pastedTexts = new WeakMap<File, string>()
  function makePastedFile(text: string): File {
    const file = new File([text], PASTED_FILE_NAME, { type: 'text/plain' })
    pastedTexts.set(file, text)
    return file
  }

  function isMediaFile(file: File): boolean {
    return file.type.startsWith('image/') || file.type.startsWith('video/')
  }

  // A stable, collision-free key per File object (two byte-identical files are
  // still distinct instances) so a card keeps its identity across reorders and
  // never replays its entry animation when a sibling is removed.
  const fileKeys = new WeakMap<File, string>()
  let fileKeySeq = 0
  function keyForFile(file: File): string {
    let key = fileKeys.get(file)
    if (!key) {
      key = `f${++fileKeySeq}`
      fileKeys.set(file, key)
    }
    return key
  }

  function isTextLikeFile(file: File): boolean {
    if (isMediaFile(file)) return false
    if (file.size > LINE_COUNT_MAX_BYTES) return false
    const mime = file.type.toLowerCase()
    if (mime.startsWith('text/')) return true
    if (mime === 'application/json' || mime === 'application/xml' || mime.includes('yaml')) return true
    const dot = file.name.lastIndexOf('.')
    const ext = dot > 0 ? file.name.slice(dot + 1).toLowerCase() : ''
    if (ext && TEXT_EXTENSIONS.has(ext)) return true
    // Pasted content arrives without a mime/extension — treat it as text.
    return mime === '' && ext === ''
  }

  // Object-URL previews for pending image/video attachments, keyed by File so a
  // URL is created once and revoked the moment its file leaves the tray (or the
  // composer unmounts) — no leaks across sends or session switches.
  const pendingPreviewUrls = ref(new Map<File, string>())
  // Line counts for text-like pending files, resolved asynchronously via FileReader.
  // A `-1` sentinel marks "reading in progress" so a file is read at most once.
  const pendingLineCounts = ref(new Map<File, number>())
  function syncPendingAttachmentMeta(files: File[]) {
    const urls = pendingPreviewUrls.value
    for (const [file, url] of urls) {
      if (!files.includes(file)) {
        URL.revokeObjectURL(url)
        urls.delete(file)
      }
    }
    for (const file of files) {
      if (!urls.has(file) && isMediaFile(file)) urls.set(file, URL.createObjectURL(file))
    }

    const counts = pendingLineCounts.value
    for (const file of [...counts.keys()]) {
      if (!files.includes(file)) counts.delete(file)
    }
    for (const file of files) {
      if (counts.has(file) || !isTextLikeFile(file)) continue
      counts.set(file, -1)
      const reader = new FileReader()
      reader.onload = (e) => {
        if (!pendingFiles.value.includes(file)) return
        counts.set(file, String(e.target?.result ?? '').split('\n').length)
      }
      // -2 marks "read failed": no count to show, but the card must still reveal.
      reader.onerror = () => { if (pendingFiles.value.includes(file)) counts.set(file, -2) }
      reader.readAsText(file)
    }
  }
  watch(pendingFiles, files => syncPendingAttachmentMeta(files), { deep: true, immediate: true })
  onBeforeUnmount(() => {
    for (const url of pendingPreviewUrls.value.values()) URL.revokeObjectURL(url)
    pendingPreviewUrls.value.clear()
  })

  const pendingPreviews = computed(() =>
    pendingFiles.value.map((file, i) => {
      const isImage = file.type.startsWith('image/')
      const isVideo = file.type.startsWith('video/')
      const isMedia = isImage || isVideo
      const dot = file.name.lastIndexOf('.')
      const url = pendingPreviewUrls.value.get(file) ?? ''
      const lc = pendingLineCounts.value.get(file)
      const pastedText = pastedTexts.get(file)
      const isPasted = pastedText !== undefined
      return {
        i,
        file,
        key: keyForFile(file),
        isMedia,
        isVideo,
        isPasted,
        pastedText: pastedText ?? '',
        size: file.size,
        url,
        ext: dot > 0 ? file.name.slice(dot + 1).toUpperCase() : '',
        lines: lc != null && lc >= 0 ? lc : null,
        // A text-like file is still loading until its line count resolves (sentinel
        // `undefined`/`-1`); the card shimmers until then, like the media skeleton.
        // Pasted content is held in memory already, so it reveals immediately.
        loading: !isPasted && !isMedia && isTextLikeFile(file) && (lc === undefined || lc === -1),
      }
    }),
  )

  // Lightbox for pending composer media so attachments can be verified at full
  // size before sending. Driven separately from the message gallery since these
  // object URLs are not part of the sent history yet.
  const composerPreviewItems = computed<MediaGalleryItem[]>(() =>
    pendingPreviews.value
      .filter(p => p.isMedia && p.url)
      .map(p => ({ src: p.url, type: p.isVideo ? 'video' : 'image', name: p.file.name })),
  )
  const composerPreviewIndex = ref<number | null>(null)
  function openComposerPreview(url: string) {
    const idx = composerPreviewItems.value.findIndex(item => item.src === url)
    if (idx >= 0) composerPreviewIndex.value = idx
  }

  // Full-text viewer for a pending pasted-content card, opened from its preview.
  const pastedViewerText = ref<string | null>(null)
  const pastedViewerOpen = computed({
    get: () => pastedViewerText.value !== null,
    set: (open: boolean) => { if (!open) pastedViewerText.value = null },
  })

  // While the last card is collapsing the row stays mounted (the card holds its
  // place) until the animation ends; the grid is open whenever there are cards and
  // we're not in that closing window.
  const collapsingAttachments = ref(false)
  const showAttachmentGrid = computed(() => pendingPreviews.value.length > 0 && !collapsingAttachments.value)
  let attachmentCollapseTimer: ReturnType<typeof setTimeout> | null = null
  function removeAttachment(index: number) {
    const file = pendingFiles.value[index]
    if (!file) return
    // Removing one of several cards just reflows the open row; removing the last
    // one collapses the row first, then drops the card so it doesn't pop out.
    if (pendingFiles.value.length > 1) {
      pendingFiles.value.splice(index, 1)
      return
    }
    collapsingAttachments.value = true
    if (attachmentCollapseTimer) clearTimeout(attachmentCollapseTimer)
    attachmentCollapseTimer = setTimeout(() => {
      const i = pendingFiles.value.indexOf(file)
      if (i >= 0) pendingFiles.value.splice(i, 1)
      collapsingAttachments.value = false
      attachmentCollapseTimer = null
    }, ATTACHMENT_ANIM_MS)
  }
  // A new file arriving mid-collapse cancels the close so it can reveal instead.
  watch(() => pendingFiles.value.length, (n, o) => {
    if (n > o && collapsingAttachments.value) {
      if (attachmentCollapseTimer) {
        clearTimeout(attachmentCollapseTimer)
        attachmentCollapseTimer = null
      }
      collapsingAttachments.value = false
    }
  })
  onBeforeUnmount(() => {
    if (attachmentCollapseTimer) clearTimeout(attachmentCollapseTimer)
  })

  function handleFileInputChange(e: Event) {
    const input = e.target as HTMLInputElement
    if (input.files) {
      for (const file of Array.from(input.files)) {
        pendingFiles.value.push(file)
      }
    }
    input.value = ''
  }

  function handlePaste(e: ClipboardEvent) {
    const data = e.clipboardData
    if (!data) return
    let handledFile = false
    for (const item of Array.from(data.items ?? [])) {
      if (item.kind === 'file') {
        const file = item.getAsFile()
        if (file) {
          pendingFiles.value.push(file)
          handledFile = true
        }
      }
    }
    // A file paste from the OS also carries a text item (its name); without this
    // the textarea would insert that filename alongside the attachment card.
    if (handledFile) {
      e.preventDefault()
      return
    }
    // A large text paste becomes a pasted-content card so it doesn't bury the
    // composer; anything below the threshold drops into the textarea as usual.
    const text = data.getData('text/plain')
    if (!text) return
    const lineCount = text.split('\n').length
    if (lineCount >= PASTE_LINE_THRESHOLD || text.length >= PASTE_CHAR_THRESHOLD) {
      e.preventDefault()
      pendingFiles.value.push(makePastedFile(text))
    }
  }

  return {
    fileInput,
    pendingFiles,
    pendingPreviews,
    composerPreviewItems,
    composerPreviewIndex,
    openComposerPreview,
    pastedViewerText,
    pastedViewerOpen,
    showAttachmentGrid,
    removeAttachment,
    handleFileInputChange,
    handlePaste,
  }
}
