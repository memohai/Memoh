<template>
  <span
    v-if="showBadge"
    class="absolute -right-0.5 -bottom-0.5 flex size-4 items-center justify-center rounded-full bg-muted ring-[1.5px] ring-background text-muted-foreground"
    :title="channelLabel"
    role="img"
    :aria-label="channelLabel"
  >
    <ChannelIcon
      :channel="platformKey"
      size="1em"
      aria-hidden="true"
    />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ChannelIcon from '@/components/channel-icon/index.vue'

const props = defineProps<{
  platform: string
}>()

const { t } = useI18n()
const platformKey = computed(() => (props.platform ?? '').trim().toLowerCase())
const isWebChannel = computed(() => {
  const k = platformKey.value
  return k === 'web' || k === ''
})
const channelLabel = computed(() => {
  if (!platformKey.value) return ''
  const key = `bots.channels.types.${platformKey.value}`
  const out = t(key)
  return out !== key ? out : platformKey.value.charAt(0).toUpperCase() + platformKey.value.slice(1)
})
const showBadge = computed(() => !isWebChannel.value)
</script>
