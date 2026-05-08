<template>
  <img
    v-if="isMemohWeb"
    src="/logo.svg"
    alt="Memoh"
    :style="imageStyle"
    v-bind="$attrs"
  >
  <component
    :is="iconComponent"
    v-else-if="iconComponent"
    :size="size"
    v-bind="$attrs"
  />
  <span
    v-else
    class="inline-flex items-center justify-center font-medium leading-none"
    :style="fallbackStyle"
    v-bind="$attrs"
  >{{ fallback }}</span>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import {
  Dingtalk,
  Qq,
  Telegram,
  Discord,
  Slack,
  Feishu,
  Wechat,
  Wechatoa,
  Wecom,
  Matrix,
  Misskey,
} from '@memohai/icon'
import { channelIconFallbackText } from '@/utils/channel-icon-fallback'

const channelIcons: Record<string, Component> = {
  qq: Qq,
  telegram: Telegram,
  discord: Discord,
  slack: Slack,
  feishu: Feishu,
  wechat: Wechat,
  weixin: Wechat,
  wechatoa: Wechatoa,
  wecom: Wecom,
  matrix: Matrix,
  misskey: Misskey,
  dingtalk: Dingtalk,
}

const props = withDefaults(defineProps<{
  channel: string
  size?: string | number
}>(), {
  size: '1em',
})

defineOptions({ inheritAttrs: false })

const normalizedChannel = computed(() =>
  props.channel.trim().toLowerCase(),
)

const iconComponent = computed<Component | undefined>(() =>
  channelIcons[normalizedChannel.value],
)

const isMemohWeb = computed(() =>
  normalizedChannel.value === 'web',
)

const fallback = computed(() =>
  channelIconFallbackText(props.channel),
)

const normalizedSize = computed(() =>
  typeof props.size === 'number' ? `${props.size}px` : props.size,
)

const imageStyle = computed(() => ({
  width: normalizedSize.value,
  height: normalizedSize.value,
}))

const fallbackStyle = computed(() => ({
  fontSize: normalizedSize.value,
}))
</script>
