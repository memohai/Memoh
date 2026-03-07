import type { Component } from 'vue'

export { default as TelegramIcon } from './TelegramIcon.vue'
export { default as FeishuIcon } from './FeishuIcon.vue'
export { default as DiscordIcon } from './DiscordIcon.vue'
export { default as QQIcon } from './QQIcon.vue'
export { default as WebIcon } from './WebIcon.vue'
export { default as CliIcon } from './CliIcon.vue'
export { default as SlackIcon } from './SlackIcon.vue'
export { default as EmailIcon } from './EmailIcon.vue'

import TelegramIcon from './TelegramIcon.vue'
import FeishuIcon from './FeishuIcon.vue'
import DiscordIcon from './DiscordIcon.vue'
import QQIcon from './QQIcon.vue'
import WebIcon from './WebIcon.vue'
import CliIcon from './CliIcon.vue'
import SlackIcon from './SlackIcon.vue'
import EmailIcon from './EmailIcon.vue'

export const channelIconMap: Record<string, Component> = {
  telegram: TelegramIcon,
  feishu: FeishuIcon,
  discord: DiscordIcon,
  qq: QQIcon,
  web: WebIcon,
  cli: CliIcon,
  slack: SlackIcon,
  email: EmailIcon,
}
