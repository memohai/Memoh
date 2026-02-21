<template>
  <span
    v-if="showBadge"
    class="absolute -right-0.5 -bottom-0.5 flex size-4 items-center justify-center overflow-hidden rounded-full bg-muted border border-background text-muted-foreground"
    :title="channelLabel"
    role="img"
    :aria-label="channelLabel"
  >
    <img
      v-if="channelImage"
      :src="channelImage"
      alt=""
      class="size-full object-contain"
    >
    <FontAwesomeIcon
      v-else
      :icon="channelIcon!"
      class="size-2.5"
      aria-hidden="true"
    />
  </span>
</template>

<script setup lang="ts">
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getChannelIcon, getChannelImage } from '@/utils/channel-icons'

const props = defineProps<{
  platform: string
}>()

const { t } = useI18n()
const platformKey = computed(() => (props.platform ?? '').trim().toLowerCase())
const isWebChannel = computed(() => {
  const k = platformKey.value
  return k === 'web' || k === ''
})
const channelImage = computed(() => getChannelImage(platformKey.value))
const channelIcon = computed(() => getChannelIcon(platformKey.value))
const channelLabel = computed(() => {
  if (!platformKey.value) return ''
  const key = `bots.channels.types.${platformKey.value}`
  const out = t(key)
  return out !== key ? out : platformKey.value.charAt(0).toUpperCase() + platformKey.value.slice(1)
})
const showBadge = computed(() => !isWebChannel.value && (channelImage.value || channelIcon.value))
</script>
