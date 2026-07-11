export interface PostConnectNavigationInput {
  changed: boolean
  hasToken: boolean
  returnTo: unknown
}

export function isCurrentServerProbe(probedBaseUrl: string, currentBaseUrl: string): boolean {
  return probedBaseUrl === currentBaseUrl
}

export type PostConnectNavigation =
  | { clearAuth: true, animateLogin: true, destination: { name: 'Login' } }
  | { clearAuth: false, animateLogin: true, destination: { name: 'Login' } }
  | { clearAuth: false, animateLogin: false, destination: string }

export function decidePostConnectNavigation(
  input: PostConnectNavigationInput,
): PostConnectNavigation {
  if (input.changed) {
    return { clearAuth: true, animateLogin: true, destination: { name: 'Login' } }
  }
  if (!input.hasToken) {
    return { clearAuth: false, animateLogin: true, destination: { name: 'Login' } }
  }
  const returnTo = typeof input.returnTo === 'string'
    && input.returnTo.startsWith('/')
    && !input.returnTo.startsWith('//')
    ? input.returnTo
    : '/'
  return { clearAuth: false, animateLogin: false, destination: returnTo }
}
