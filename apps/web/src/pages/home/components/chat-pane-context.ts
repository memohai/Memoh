import type { ComputedRef, InjectionKey } from 'vue'

// The session a ChatPane subtree belongs to. Provided by chat-pane.vue and
// injected by descendants (e.g. tool-call-inline) that answer the store with a
// session-scoped action, so a pinned tab's approval/input targets its own chat
// rather than whatever session happens to be globally active.
export const CHAT_PANE_SESSION_ID: InjectionKey<ComputedRef<string | null>> = Symbol('chat-pane-session-id')
