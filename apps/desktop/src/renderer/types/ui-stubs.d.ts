// Minimal type stub for @memohai/ui consumed directly by the desktop
// renderer's own files. @memohai/ui ships a barrel `index.ts` that imports
// every component; pulling those into desktop's typecheck program surfaces
// pre-existing strict-template warnings unrelated to desktop. Routing the
// typecheck through this stub keeps the desktop's surface small.
//
// Vite ignores `paths` and resolves the real `@memohai/ui` package at bundle
// time, so runtime behavior is unchanged.

declare module '@memohai/ui' {
  import type { DefineComponent } from 'vue'

  type LooseComponent = DefineComponent<
    Record<string, unknown>,
    Record<string, unknown>,
    unknown
  >

  export const Toaster: LooseComponent
  export const SidebarInset: LooseComponent
}
