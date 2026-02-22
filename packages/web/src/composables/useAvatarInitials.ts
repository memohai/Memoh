import { computed } from 'vue'

export function useAvatarInitials(getLabel: () => string | null | undefined, fallback = '') {
  return computed(() => {
    const label = getLabel()?.trim() ?? ''
    if (!label) {
      return fallback
    }
    return label.slice(0, 2).toUpperCase() || fallback
  })
}
