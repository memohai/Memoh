import type { InjectionKey } from 'vue'

// Provided by the Electron desktop shell to enable a macOS-style top inset
// (traffic-light reserve + custom TopBar) inside reusable web sidebars.
// Web (browser) does not provide this key, so consumers fall back to false.
export const DesktopShellKey: InjectionKey<boolean> = Symbol('memohai:desktop-shell')
