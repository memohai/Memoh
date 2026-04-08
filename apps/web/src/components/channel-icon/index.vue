<template>
  <component
    :is="iconComponent"
    v-if="iconComponent"
    :size="size"
    v-bind="$attrs"
  />
  <span
    v-else
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
  Wecom,
  Matrix,
} from '@memohai/icon'

const channelIcons: Record<string, Component> = {
  qq: Qq,
  telegram: Telegram,
  discord: Discord,
  slack: Slack,
  feishu: Feishu,
  wechat: Wechat,
  weixin: Wechat,
  wecom: Wecom,
  matrix: Matrix,
  dingtalk: Dingtalk,
}

const props = withDefaults(defineProps<{
  channel: string
  size?: string | number
}>(), {
  size: '1em',
})

defineOptions({ inheritAttrs: false })

const iconComponent = computed<Component | undefined>(() =>
  channelIcons[props.channel],
)

const fallback = computed(() =>
  props.channel.slice(0, 2).toUpperCase(),
)
</script>
