/**
 * Localized channel platform title for UI.
 * Prefer bots.channels.types.{type}; fall back to server display_name, then raw type.
 */
export function channelTypeDisplayName(
  t: (key: string, ...args: unknown[]) => string,
  channelType: string | undefined | null,
  serverDisplayName?: string | null,
): string {
  const raw = (channelType ?? '').trim().toLowerCase()
  if (!raw) {
    return (serverDisplayName ?? '').trim() || ''
  }
  const i18nKey = `bots.channels.types.${raw}`
  const out = t(i18nKey)
  if (out !== i18nKey) return out
  const fb = (serverDisplayName ?? '').trim()
  if (fb) return fb
  return raw.charAt(0).toUpperCase() + raw.slice(1)
}
