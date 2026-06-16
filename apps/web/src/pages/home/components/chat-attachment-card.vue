<template>
  <div class="group/att relative shrink-0">
    <div
      class="relative size-24 overflow-hidden rounded-lg border border-border bg-surface-composer"
      :class="interactive ? 'transition-colors group-hover/att:border-ring' : ''"
    >
      <!-- Image / video cover -->
      <template v-if="kind === 'media'">
        <Skeleton
          v-if="src && !mediaLoaded"
          class="absolute inset-0 size-full rounded-none"
        />
        <video
          v-if="src && video"
          :src="src"
          class="size-full object-cover transition-opacity"
          :class="mediaLoaded ? 'opacity-100' : 'opacity-0'"
          preload="metadata"
          muted
          playsinline
          @loadeddata="mediaLoaded = true"
        />
        <img
          v-else-if="src"
          :src="src"
          :alt="name || ''"
          class="size-full object-cover transition-opacity"
          :class="mediaLoaded ? 'opacity-100' : 'opacity-0'"
          loading="eager"
          decoding="async"
          @load="mediaLoaded = true"
          @error="mediaLoaded = true"
        >
        <div
          v-else
          class="flex size-full items-center justify-center text-muted-foreground"
        >
          <ImageIcon class="size-5" />
        </div>
      </template>

      <!-- File card -->
      <div
        v-else
        class="flex size-full flex-col gap-1 p-2.5"
      >
        <span class="line-clamp-3 break-all text-caption leading-snug text-foreground">
          {{ name }}
        </span>
        <div class="flex-1" />
        <span
          v-if="ext"
          class="self-start rounded-sm border border-border px-1.5 py-0.5 text-caption font-medium uppercase tracking-wide text-muted-foreground"
        >
          {{ ext }}
        </span>
        <FileIcon
          v-else
          class="size-4 text-muted-foreground"
        />
      </div>
    </div>

    <!-- Remove (composer only) -->
    <button
      v-if="removable"
      type="button"
      :aria-label="resolvedRemoveLabel"
      class="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full border border-border bg-card text-muted-foreground opacity-0 transition-[opacity,color] hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none group-hover/att:opacity-100"
      @click.stop="emit('remove')"
    >
      <X class="size-3" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { X, Image as ImageIcon, File as FileIcon } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Skeleton } from '@memohai/ui'

const props = defineProps<{
  kind: 'media' | 'file'
  src?: string
  video?: boolean
  name?: string
  ext?: string
  removable?: boolean
  interactive?: boolean
  removeLabel?: string
}>()

const emit = defineEmits<{ remove: [] }>()
const { t } = useI18n()

const resolvedRemoveLabel = computed(() =>
  props.removeLabel
  || (props.name ? `${t('common.delete')}: ${props.name}` : t('common.delete')),
)

const mediaLoaded = ref(false)
watch(() => props.src, () => { mediaLoaded.value = false })
</script>
