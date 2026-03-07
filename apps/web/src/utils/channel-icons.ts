import type { Component } from 'vue'
import { channelIconMap } from '@memoh/icon'

export function getChannelIconComponent(platformKey: string): Component | null {
  if (!platformKey) return null
  return channelIconMap[platformKey] ?? null
}

export { channelIconMap } from '@memoh/icon'
