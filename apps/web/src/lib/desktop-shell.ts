import type { InjectionKey } from 'vue'

// Provided by the Electron desktop shell to enable a macOS-style top inset
// (traffic-light reserve + custom TopBar) inside reusable web sidebars.
// Web (browser) does not provide this key, so consumers fall back to false.
export const DesktopShellKey: InjectionKey<boolean> = Symbol('memohai:desktop-shell')

// The desktop runtime mode, provided by the Electron desktop shell so the
// shared web pages can branch on whether they talk to the managed local
// server (auto-login via [admin], no login screen to return to) or a remote
// server (real user accounts, sign-out is meaningful). Web (browser) does not
// provide this key, so consumers fall back to 'local'.
export type DesktopRuntimeMode = 'local' | 'remote'
export const DesktopRuntimeModeKey: InjectionKey<DesktopRuntimeMode> = Symbol('memohai:desktop-runtime-mode')
