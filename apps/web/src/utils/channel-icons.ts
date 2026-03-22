/**
 * Local channel icons under public/channels/.
 * getChannelImage: URL to local icon when available.
 * getChannelIcon: FontAwesome fallback when no local image.
 */

const LOCAL_CHANNEL_IMAGES: Record<string, string> = {
  feishu: '/channels/feishu.png',
  telegram: '/channels/telegram.webp',
  weixin: '/channels/weixin.png',
}

const CHANNEL_ICONS: Record<string, [string, string]> = {
  qq: ['fab', 'qq'],
  telegram: ['fab', 'telegram'],
  feishu: ['fas', 'comment-dots'],
  weixin: ['fab', 'weixin'],
  web: ['fas', 'globe'],
  slack: ['fab', 'slack'],
  discord: ['fab', 'discord'],
  email: ['fas', 'envelope'],
}

const DEFAULT_ICON: [string, string] = ['far', 'comment']

export function getChannelIcon(platformKey: string): [string, string] {
  if (!platformKey) return DEFAULT_ICON
  return CHANNEL_ICONS[platformKey] ?? DEFAULT_ICON
}

export function getChannelImage(platformKey: string): string | null {
  if (!platformKey) return null
  return LOCAL_CHANNEL_IMAGES[platformKey] ?? null
}
