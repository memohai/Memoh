// Memoh's stream doesn't carry reasoning duration, and a finished turn is
// re-fetched (which replaces the live components that measured it). To keep
// "Thought for Ns" after that refetch, the duration is measured live while
// streaming and stashed here keyed by the reasoning text; the re-mounted (done)
// block then recovers it by the same key. Cross-reload it's gone (no backend
// timing) — that's acceptable: the turn you just watched keeps its timer.
//
// Measurement is driven centrally from message-item so it covers *every*
// reasoning block, not only the last (tail) one: `markReasoningSeen` stamps a
// start the first time a block appears mid-stream, and `finalizeReasoning`
// closes it out once a later block appears or the turn ends. This is why a
// reasoning step that's immediately followed by a tool call still gets a real
// "Thought for Ns" instead of a bare "Thought".
const durations = new Map<string, number>()
const starts = new Map<string, number>()

function keyFor(content: string): string {
  let hash = 0
  for (let i = 0; i < content.length; i += 1) {
    hash = (hash * 31 + content.charCodeAt(i)) | 0
  }
  return `${content.length}:${hash}`
}

// Stamp the moment a reasoning block first appears while streaming. No-op once
// it already has a start or a finalized duration.
export function markReasoningSeen(content: string): void {
  const trimmed = content.trim()
  if (!trimmed) return
  const key = keyFor(trimmed)
  if (durations.has(key) || starts.has(key)) return
  starts.set(key, Date.now())
}

// Close out a reasoning block (a later block appeared, or the turn ended),
// converting its start stamp into a final duration. Floored at 1ms so the label
// rounds to at least 1s rather than showing nothing.
export function finalizeReasoning(content: string): void {
  const trimmed = content.trim()
  if (!trimmed) return
  const key = keyFor(trimmed)
  const start = starts.get(key)
  if (start === undefined) return
  starts.delete(key)
  if (!durations.has(key)) {
    durations.set(key, Math.max(1, Date.now() - start))
  }
}

export function getReasoningDuration(content: string): number {
  const trimmed = content.trim()
  return trimmed ? durations.get(keyFor(trimmed)) ?? 0 : 0
}
