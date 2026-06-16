<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="isOpen"
        ref="dialogRef"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/90"
        role="dialog"
        aria-modal="true"
        :aria-label="currentItem?.name ? `Media preview: ${currentItem.name}` : 'Media preview'"
        tabindex="-1"
        @click.self="close"
      >
        <!-- Close -->
        <button
          ref="closeButtonRef"
          type="button"
          class="absolute right-4 top-4 z-10 rounded-full p-2 text-white/80 hover:bg-white/10 hover:text-white transition-colors"
          aria-label="Close"
          @click="close"
        >
          <X
            class="size-6"
          />
        </button>

        <!-- Prev -->
        <button
          v-if="items.length > 1"
          type="button"
          class="absolute left-4 top-1/2 -translate-y-1/2 z-10 rounded-full p-3 text-white/80 hover:bg-white/10 hover:text-white transition-colors"
          aria-label="Previous"
          @click.stop="prev"
        >
          <ChevronLeft
            class="size-6"
          />
        </button>

        <!-- Next -->
        <button
          v-if="items.length > 1"
          type="button"
          class="absolute right-4 top-1/2 -translate-y-1/2 z-10 rounded-full p-3 text-white/80 hover:bg-white/10 hover:text-white transition-colors"
          aria-label="Next"
          @click.stop="next"
        >
          <ChevronRight
            class="size-6"
          />
        </button>

        <!-- Media content -->
        <div class="max-w-[90vw] max-h-[90vh] flex items-center justify-center">
          <img
            v-if="currentItem?.type === 'image'"
            :src="currentItem.src"
            :alt="currentItem.name || 'image'"
            class="max-w-full max-h-[90vh] object-contain select-none"
            draggable="false"
            @click.stop
          >
          <video
            v-else-if="currentItem?.type === 'video'"
            :src="currentItem.src"
            controls
            class="max-w-full max-h-[90vh] object-contain"
            @click.stop
          />
        </div>

        <!-- Counter -->
        <div
          v-if="items.length > 1"
          class="absolute bottom-4 left-1/2 -translate-x-1/2 px-3 py-1 rounded-full bg-black/50 text-white/90 text-xs"
        >
          {{ (openIndex ?? 0) + 1 }} / {{ items.length }}
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watchEffect, nextTick, ref } from 'vue'
import { X, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { appKeyboardCommands } from '@/lib/keyboard-commands'

export interface MediaGalleryItem {
  src: string
  type: 'image' | 'video'
  name?: string
}

const props = defineProps<{
  items: MediaGalleryItem[]
  openIndex: number | null
}>()

const emit = defineEmits<{
  'update:openIndex': [value: number | null]
}>()

const isOpen = computed(() => props.openIndex !== null && props.items.length > 0)
const closeButtonRef = ref<HTMLButtonElement | null>(null)
const dialogRef = ref<HTMLElement | null>(null)
let previousFocusedElement: HTMLElement | null = null

const currentItem = computed(() => {
  const idx = props.openIndex
  if (idx === null || idx < 0 || idx >= props.items.length) return null
  return props.items[idx] ?? null
})

function close() {
  emit('update:openIndex', null)
}

function prev() {
  if (props.openIndex === null) return
  const idx = props.openIndex <= 0 ? props.items.length - 1 : props.openIndex - 1
  emit('update:openIndex', idx)
}

function next() {
  if (props.openIndex === null) return
  const idx = props.openIndex >= props.items.length - 1 ? 0 : props.openIndex + 1
  emit('update:openIndex', idx)
}

watchEffect(() => {
  if (isOpen.value) {
    previousFocusedElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
    nextTick(() => {
      closeButtonRef.value?.focus()
    })
  } else {
    previousFocusedElement?.focus()
    previousFocusedElement = null
  }
})

// Scoped to "lightbox open": each handler returns false while closed so the
// dispatcher keeps iterating (e.g. lets Escape fall through to a global). When
// the lightbox is open we claim the key, fire the action, and preventDefault.
function claim(action: () => void): boolean {
  if (!isOpen.value) return false
  action()
  return true
}

useKeyboardCommand(appKeyboardCommands.closeMediaLightbox, () => claim(close))
useKeyboardCommand(appKeyboardCommands.mediaLightboxPrev, () => claim(prev))
useKeyboardCommand(appKeyboardCommands.mediaLightboxNext, () => claim(next))
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
