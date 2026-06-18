import { computed } from 'vue'

// Single source for the "first letters as an avatar fallback" rule. Use the pure
// avatarInitials() in lists or one-off calls; useAvatarInitials() wraps it for a
// reactive label and returns a computed.
export function avatarInitials(label: string | null | undefined, fallback = '') {
  const text = label?.trim() ?? ''
  if (!text) {
    return fallback
  }
  return text.slice(0, 2).toUpperCase() || fallback
}

export function useAvatarInitials(getLabel: () => string | null | undefined, fallback = '') {
  return computed(() => avatarInitials(getLabel(), fallback))
}
