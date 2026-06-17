import {
  getCurrentInstance,
  inject,
  onActivated,
  onDeactivated,
  onScopeDispose,
  type InjectionKey,
} from 'vue'
import {
  createScopedKeyboardBinding,
  type AppKeyboardCommand,
  type KeyboardCommandHandler,
  type KeyboardCommandRegistry,
} from '@/lib/keyboard-commands'

/** Provided once at app bootstrap so any component can register scoped shortcuts. */
export const KEYBOARD_REGISTRY: InjectionKey<KeyboardCommandRegistry> = Symbol('keyboard-registry')

/**
 * Register a keyboard command handler that is live only while the calling
 * component is mounted AND active. A handler that returns `false` declines the
 * command, letting it fall through (in the browser, to native behavior).
 *
 * KeepAlive matters here: a cached-but-deactivated component stays *mounted*, so
 * relying on unmount alone would leave its handler claiming the command from
 * other tabs. We bind on setup, rebind on `onActivated`, and unbind on both
   * `onDeactivated` and scope disposal, so an inactive file viewer can't swallow
 * Cmd/Ctrl+S meant for whatever tab is actually focused.
 *
 * No-ops when no registry is provided, so components stay safe to mount in
 * isolation (tests, storybook, non-app contexts).
 */
export function useKeyboardCommand(
  command: AppKeyboardCommand,
  handler: KeyboardCommandHandler,
): void {
  const registry = inject(KEYBOARD_REGISTRY, null)
  if (!registry) return

  const binding = createScopedKeyboardBinding(registry, command, handler)
  binding.bind()
  onScopeDispose(binding.unbind)

  // Activate/deactivate only fire for components inside <KeepAlive>; guard on an
  // instance so the composable stays usable (and warning-free) outside one.
  if (getCurrentInstance()) {
    onActivated(binding.bind)
    onDeactivated(binding.unbind)
  }
}
