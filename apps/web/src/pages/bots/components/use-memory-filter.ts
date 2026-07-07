import { computed, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import type { AdaptersMemoryItem } from '@memohai/sdk'

export type MemoryLayer = 'all' | 'identity' | 'preference' | 'context' | 'experience' | 'activity' | 'persona' | 'note'

export const MEMORY_LAYERS: MemoryLayer[] = ['all', 'identity', 'preference', 'context', 'experience', 'activity', 'persona', 'note']

/**
 * Read the layer from a memory item's metadata. The DB node carries `layer` as a
 * metadata key (see graph_runtime / migrate.Plan classification). Falls back to
 * 'note' when unset, matching the backend default.
 */
export function memoryLayerOf(item: AdaptersMemoryItem): Exclude<MemoryLayer, 'all'> {
  const raw = item?.metadata?.layer
  return typeof raw === 'string' && MEMORY_LAYERS.includes(raw as MemoryLayer)
    ? raw as Exclude<MemoryLayer, 'all'>
    : 'note'
}

export function memoryConfidence(item: AdaptersMemoryItem): number | null {
  const raw = item?.metadata?.confidence
  if (typeof raw === 'number') return raw
  if (typeof raw === 'string') {
    const parsed = Number.parseFloat(raw)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

export function memoryTags(item: AdaptersMemoryItem): string[] {
  const raw = item?.metadata?.tags
  if (Array.isArray(raw)) {
    return raw.filter((t): t is string => typeof t === 'string' && t.length > 0)
  }
  return []
}

export interface UseMemoryFilterResult {
  activeLayer: Ref<MemoryLayer>
  searchQuery: Ref<string>
  isSearching: Ref<boolean>
  filtered: ComputedRef<AdaptersMemoryItem[]>
  layerCounts: ComputedRef<Record<MemoryLayer, number>>
  /** True when the user is actively typing a search (gates the layer chips off). */
  searchActive: ComputedRef<boolean>
}

/**
 * Local layer filtering + search-query state for the memory list. Search itself
 * is server-side (postBotsByBotIdMemorySearch); this only owns the input state
 * and the client-side layer filter applied to whichever list the page holds.
 */
export function useMemoryFilter(memories: ComputedRef<AdaptersMemoryItem[]>): UseMemoryFilterResult {
  const activeLayer = ref<MemoryLayer>('all')
  const searchQuery = ref('')
  const isSearching = ref(false)

  const layerCounts = computed<Record<MemoryLayer, number>>(() => {
    const counts: Record<MemoryLayer, number> = {
      all: memories.value.length,
      identity: 0,
      preference: 0,
      context: 0,
      experience: 0,
      activity: 0,
      persona: 0,
      note: 0,
    }
    for (const item of memories.value) {
      counts[memoryLayerOf(item)]++
    }
    return counts
  })

  const filtered = computed<AdaptersMemoryItem[]>(() => {
    if (activeLayer.value === 'all') return memories.value
    return memories.value.filter((item) => memoryLayerOf(item) === activeLayer.value)
  })

  const searchActive = computed(() => isSearching.value || searchQuery.value.trim().length > 0)

  return {
    activeLayer,
    searchQuery,
    isSearching,
    filtered,
    layerCounts,
    searchActive,
  }
}
