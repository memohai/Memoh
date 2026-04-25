// Type stubs for @memohai/web subpath imports consumed by the renderer.
// We route typechecking through these stubs (via tsconfig `paths`) so vue-tsc
// does not recursively typecheck @memohai/web's source tree — @memohai/web
// owns its own types/CI. Vite ignores `paths` and resolves the real `exports`
// entries at bundle time, so runtime behavior is unchanged.

// Marker to make this file a module (not an ambient script). Required so
// that paths-mapped dynamic `import('@memohai/web/...')` calls succeed —
// TS otherwise complains that the resolved file "is not a module".
export {}

declare module '@memohai/web/router' {
  import type { Router } from 'vue-router'
  const router: Router
  export default router
}

declare module '@memohai/web/i18n' {
  import type { I18n } from 'vue-i18n'
  const i18n: I18n
  export default i18n
}

declare module '@memohai/web/api-client' {
  export interface SetupApiClientOptions {
    onUnauthorized?: () => void
  }
  export function setupApiClient(options?: SetupApiClientOptions): void
}

declare module '@memohai/web/store/settings' {
  // We don't need the concrete Pinia store type here — desktop just calls the
  // composable for its registration side-effect.
  export function useSettingsStore(): unknown
}

declare module '@memohai/web/lib/desktop-shell' {
  import type { InjectionKey } from 'vue'
  export const DesktopShellKey: InjectionKey<boolean>
}

declare module '@memohai/web/style.css'

// Fallback for every Vue SFC reachable through the @memohai/web/* wildcard
// export. The TS ambient-module `*` token matches multi-segment paths
// (slashes included), so this single declaration covers `pages/.../*.vue`,
// `components/.../*.vue`, `layout/.../*.vue`, etc.
declare module '@memohai/web/*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, never>, Record<string, never>, unknown>
  export default component
}
