import type { Component } from 'vue'

export { default as BraveIcon } from './BraveIcon.vue'
export { default as BingIcon } from './BingIcon.vue'
export { default as GoogleIcon } from './GoogleIcon.vue'
export { default as TavilyIcon } from './TavilyIcon.vue'
export { default as SogouIcon } from './SogouIcon.vue'
export { default as SerperIcon } from './SerperIcon.vue'
export { default as SearxngIcon } from './SearxngIcon.vue'
export { default as JinaIcon } from './JinaIcon.vue'
export { default as ExaIcon } from './ExaIcon.vue'
export { default as BochaIcon } from './BochaIcon.vue'
export { default as DuckduckgoIcon } from './DuckduckgoIcon.vue'
export { default as YandexIcon } from './YandexIcon.vue'

import BraveIcon from './BraveIcon.vue'
import BingIcon from './BingIcon.vue'
import GoogleIcon from './GoogleIcon.vue'
import TavilyIcon from './TavilyIcon.vue'
import SogouIcon from './SogouIcon.vue'
import SerperIcon from './SerperIcon.vue'
import SearxngIcon from './SearxngIcon.vue'
import JinaIcon from './JinaIcon.vue'
import ExaIcon from './ExaIcon.vue'
import BochaIcon from './BochaIcon.vue'
import DuckduckgoIcon from './DuckduckgoIcon.vue'
import YandexIcon from './YandexIcon.vue'

export const searchProviderIconMap: Record<string, Component> = {
  brave: BraveIcon,
  bing: BingIcon,
  google: GoogleIcon,
  tavily: TavilyIcon,
  sogou: SogouIcon,
  serper: SerperIcon,
  searxng: SearxngIcon,
  jina: JinaIcon,
  exa: ExaIcon,
  bocha: BochaIcon,
  duckduckgo: DuckduckgoIcon,
  yandex: YandexIcon,
}
