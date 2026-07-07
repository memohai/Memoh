import { describe, expect, it } from 'vitest'
import { computed } from 'vue'
import {
  MEMORY_LAYERS,
  memoryConfidence,
  memoryLayerOf,
  memoryTags,
  useMemoryFilter,
} from './use-memory-filter'
import type { AdaptersMemoryItem } from '@memohai/sdk'

function item(over: Partial<AdaptersMemoryItem> = {}): AdaptersMemoryItem {
  return { id: '1', memory: 'm', ...over }
}

describe('memoryLayerOf', () => {
  it('returns the metadata layer when valid', () => {
    expect(memoryLayerOf(item({ metadata: { layer: 'preference' } }))).toBe('preference')
    expect(memoryLayerOf(item({ metadata: { layer: 'identity' } }))).toBe('identity')
  })

  it('falls back to note when layer is missing or unknown', () => {
    expect(memoryLayerOf(item({ metadata: {} }))).toBe('note')
    expect(memoryLayerOf(item({ metadata: { layer: 'bogus' } }))).toBe('note')
    expect(memoryLayerOf(item({}))).toBe('note')
  })
})

describe('memoryConfidence', () => {
  it('reads numeric confidence', () => {
    expect(memoryConfidence(item({ metadata: { confidence: 0.9 } }))).toBe(0.9)
  })

  it('parses string confidence', () => {
    expect(memoryConfidence(item({ metadata: { confidence: '0.8' } }))).toBe(0.8)
  })

  it('returns null when missing or unparseable', () => {
    expect(memoryConfidence(item({ metadata: {} }))).toBeNull()
    expect(memoryConfidence(item({ metadata: { confidence: 'high' } }))).toBeNull()
  })
})

describe('memoryTags', () => {
  it('returns string tags, filtering non-strings', () => {
    expect(memoryTags(item({ metadata: { tags: ['tea', 4, '', 'oolong'] } }))).toEqual(['tea', 'oolong'])
  })

  it('returns empty array when tags missing or not array', () => {
    expect(memoryTags(item({ metadata: {} }))).toEqual([])
    expect(memoryTags(item({ metadata: { tags: 'tea' } }))).toEqual([])
  })
})

describe('useMemoryFilter', () => {
  const list: AdaptersMemoryItem[] = [
    item({ id: '1', metadata: { layer: 'preference' } }),
    item({ id: '2', metadata: { layer: 'preference' } }),
    item({ id: '3', metadata: { layer: 'identity' } }),
    item({ id: '4', metadata: { layer: 'note' } }),
    item({ id: '5', metadata: {} }), // defaults to note
  ]

  it('counts items per layer including all', () => {
    const { layerCounts } = useMemoryFilter(computed(() => list))
    expect(layerCounts.value.all).toBe(5)
    expect(layerCounts.value.preference).toBe(2)
    expect(layerCounts.value.identity).toBe(1)
    expect(layerCounts.value.note).toBe(2)
    expect(layerCounts.value.context).toBe(0)
  })

  it('filters by the active layer', () => {
    const { activeLayer, filtered } = useMemoryFilter(computed(() => list))
    expect(filtered.value).toHaveLength(5)
    activeLayer.value = 'preference'
    expect(filtered.value).toHaveLength(2)
    expect(filtered.value.every((m) => m.id === '1' || m.id === '2')).toBe(true)
    activeLayer.value = 'identity'
    expect(filtered.value).toHaveLength(1)
    activeLayer.value = 'all'
    expect(filtered.value).toHaveLength(5)
  })

  it('searchActive tracks isSearching and query text', () => {
    const { searchQuery, isSearching, searchActive } = useMemoryFilter(computed(() => list))
    expect(searchActive.value).toBe(false)
    searchQuery.value = 'tea'
    expect(searchActive.value).toBe(true)
    searchQuery.value = ''
    isSearching.value = true
    expect(searchActive.value).toBe(true)
  })

  it('exposes the canonical layer order', () => {
    expect(MEMORY_LAYERS[0]).toBe('all')
    expect(MEMORY_LAYERS).toContain('preference')
    expect(MEMORY_LAYERS).toContain('note')
  })
})
