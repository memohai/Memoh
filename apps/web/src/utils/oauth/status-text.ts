interface OAuthStatusTextStatus {
  configured: boolean
  has_token: boolean
}

interface OAuthStatusTextOptions {
  loading: boolean
  authorizing?: boolean
  status: OAuthStatusTextStatus | null
  unavailableKey: string
}

export function oauthStatusTextKey(options: OAuthStatusTextOptions): string {
  if (options.status?.has_token) return 'provider.oauth.status.authorized'
  if (options.status && !options.status.configured) return options.unavailableKey
  if (options.authorizing) return 'provider.oauth.status.oauthing'
  if (options.loading) return 'provider.oauth.status.checking'
  if (!options.status?.configured) return options.unavailableKey
  return 'provider.oauth.status.missing'
}
