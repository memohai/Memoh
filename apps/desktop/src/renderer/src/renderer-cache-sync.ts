// Pinia Colada query-cache synchronization for desktop renderer hosts.
//
// We wrap `queryCache.invalidateQueries` so every local invalidation also
// asks the main process to broadcast the (serializable) filter to every
// other renderer host. Sibling renderers replay the invalidation against
// their own caches via the un-wrapped original method, which avoids
// re-broadcasting and prevents echo loops.

import type { useQueryCache } from '@pinia/colada'
import type { RendererInvalidatePayload } from '../../preload'

type QueryCache = ReturnType<typeof useQueryCache>
type InvalidateQueries = QueryCache['invalidateQueries']
type InvalidateFilters = Parameters<InvalidateQueries>[0]
type InvalidateRefetch = Parameters<InvalidateQueries>[1]

// Pull only structured-clone-safe fields off the filter. If the caller
// passed a `predicate` function we can't ship it across the IPC boundary;
// in that case we skip the broadcast (returning `null`) — the local
// invalidation still happens, only the broadcast mirror is dropped.
function toSerializableFilter(
  filters: InvalidateFilters,
): RendererInvalidatePayload['filters'] | null {
  if (filters == null) return undefined
  const raw = filters as Record<string, unknown>
  if (typeof raw.predicate === 'function') return null

  const out: NonNullable<RendererInvalidatePayload['filters']> = {}
  if ('key' in raw && raw.key !== undefined) {
    try {
      out.key = JSON.parse(JSON.stringify(raw.key)) as unknown
    }
    catch {
      return null
    }
  }
  if (typeof raw.exact === 'boolean') out.exact = raw.exact
  if (raw.stale === null || typeof raw.stale === 'boolean') out.stale = raw.stale as boolean | null
  if (raw.status !== undefined) out.status = raw.status
  return out
}

function toSerializableRefetch(refetch: InvalidateRefetch): RendererInvalidatePayload['refetchActive'] {
  if (refetch === true || refetch === false || refetch === 'all') return refetch
  return undefined
}

export function setupRendererCacheSync(queryCache: QueryCache): void {
  const desktop = window.api?.desktop
  if (!desktop || typeof desktop.broadcastInvalidate !== 'function' || typeof desktop.onInvalidate !== 'function') {
    return
  }

  const original = queryCache.invalidateQueries.bind(queryCache) as InvalidateQueries

  const wrapped: InvalidateQueries = (filters, refetchActive) => {
    const result = original(filters, refetchActive)
    const serializableFilters = toSerializableFilter(filters)
    if (serializableFilters !== null) {
      void desktop.broadcastInvalidate({
        filters: serializableFilters,
        refetchActive: toSerializableRefetch(refetchActive),
      })
    }
    return result
  }

  queryCache.invalidateQueries = wrapped

  desktop.onInvalidate((payload) => {
    void original(payload?.filters as InvalidateFilters, payload?.refetchActive)
  })
}
