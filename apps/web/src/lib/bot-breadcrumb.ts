import { reactive } from 'vue'

/**
 * Maps a bot route identifier (whatever sits in the URL — a slug or a raw UUID)
 * to its human display name. The bot detail page registers the name once the bot
 * loads; the router's bot-detail breadcrumb reads it so the breadcrumb and the
 * "back" affordance show a real name instead of a raw `bot-<uuid>`.
 *
 * When a name isn't known yet, lookups return '' so the back affordance falls
 * back to its generic label — a raw id never reaches the UI.
 */
const names = reactive<Record<string, string>>({})

export function registerBotBreadcrumbName(
  identifier: string | undefined,
  name: string | undefined,
): void {
  const key = (identifier ?? '').trim()
  const value = (name ?? '').trim()
  if (!key || !value) return
  names[key] = value
}

export function getBotBreadcrumbName(identifier: string | undefined): string {
  const key = (identifier ?? '').trim()
  if (!key) return ''
  return names[key] ?? ''
}
