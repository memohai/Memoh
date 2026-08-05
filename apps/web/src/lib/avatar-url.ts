import { sdkApiUrl, sdkAuthQuery } from '@/lib/api-client'

const storedAvatarPath = /^\/avatars\/[a-f0-9]{64}$/i

// Stored avatar URLs are deployment-neutral database values. Resolve them
// through the configured SDK base and current auth token so Web and Desktop
// both load them from the private server-side media proxy.
export function resolveAvatarUrl(value: string | null | undefined): string {
  const trimmed = value?.trim() ?? ''
  if (!storedAvatarPath.test(trimmed)) return trimmed
  return sdkApiUrl({ url: trimmed, query: sdkAuthQuery() })
}
