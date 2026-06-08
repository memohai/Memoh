import { computed, reactive, ref } from 'vue'
import { reconcileById, sortByRecency, upsertById } from '@/store/chat-list.utils'
import type { DemoBlock, DemoSession, DemoTurn } from './demo-types'

export type DemoMode = 'new' | 'old'

// Pre-refactor `upsertById`, reconstructed from git history (commit ace83305^).
// Allocates a new array and replaces the matched element with the incoming
// object on every call, so the block's identity churns once per streamed token.
function upsertByIdOld<T extends { id: number }>(items: T[], incoming: T): T[] {
  const next = [...items]
  const index = next.findIndex(item => item.id === incoming.id)
  if (index >= 0) next[index] = incoming
  else {
    next.push(incoming)
    next.sort((a, b) => a.id - b.id)
  }
  return next
}

const THINKING = 'Reading the request, planning the build, and getting ready to stream the logs.'
const TEXT_INTRO = 'Sure — kicking off the build now and streaming the logs as they arrive.\n'
const LOG_LINES = [
  '$ pnpm build',
  'vite v7 building for production...',
  'transforming modules...',
  '✓ 1423 modules transformed',
  'rendering chunks...',
  'computing gzip size...',
  'dist/assets/index.js   estimated',
  '✓ built in 4.21s',
]
const TEXT_SUMMARY = '\nBuild finished successfully — no new oversized chunks, the bundle looks healthy.'

type Phase = 'idle' | 'thinking' | 'intro' | 'tool' | 'summary'

const BASE_TS = Date.parse('2026-06-08T10:00:00Z')

export function useChatRefactorDemo() {
  const mode = ref<DemoMode>('new')
  const messages = reactive<DemoTurn[]>([])
  const stats = reactive({ refetches: 0, blockChurn: 0, cyclesCompleted: 0 })

  const sessions = reactive<DemoSession[]>([
    { id: 's1', title: 'Chat · onboarding flow', updated_at: '2026-06-08T10:00:00Z' },
    { id: 's2', title: 'Background task · nightly build', updated_at: '2026-06-08T09:00:00Z' },
    { id: 's3', title: 'Chat · bug triage', updated_at: '2026-06-08T08:00:00Z' },
  ])
  const sidebarStats = reactive({ reorders: 0 })
  const orderedSessions = computed(() => (mode.value === 'new' ? sortByRecency(sessions) : [...sessions]))

  let clock = 0
  const stamp = () => new Date(BASE_TS + clock++ * 1000).toISOString()

  let cycle = 0
  let assistant: DemoTurn | null = null
  let phase: Phase = 'idle'
  let prog = 0

  // Write one block snapshot into a turn. NEW mutates in place (identity stable,
  // markstream-vue keeps its smoothing); OLD swaps in a fresh array + object.
  function putBlock(turn: DemoTurn, incoming: DemoBlock) {
    if (!turn.blocks) turn.blocks = []
    const before = turn.blocks.find(block => block.id === incoming.id)
    if (mode.value === 'new') upsertById(turn.blocks, incoming)
    else turn.blocks = upsertByIdOld(turn.blocks, incoming)
    const after = turn.blocks.find(block => block.id === incoming.id)
    if (before && after && before !== after) stats.blockChurn++
  }

  const serverIdOf = (turn: DemoTurn) =>
    turn.serverId ?? (turn.id.startsWith('srv-') ? turn.id : turn.id.replace('opt-', 'srv-'))

  function serverize(turn: DemoTurn): DemoTurn {
    return {
      id: serverIdOf(turn),
      role: turn.role,
      text: turn.text,
      ts: turn.ts,
      blocks: turn.blocks?.map(block => ({ ...block })),
    }
  }

  function mergeTurnInPlace(current: DemoTurn, incoming: DemoTurn) {
    if (current.role === 'assistant' && incoming.role === 'assistant' && current.blocks && incoming.blocks) {
      reconcileById(current.blocks, incoming.blocks)
    }
    current.text = incoming.text
    current.ts = incoming.ts
  }

  // Mirrors the store's adoptTailOptimisticTurns: link a just-sent optimistic
  // turn to its server twin so reconcile updates it in place under its original
  // id (the v-for key), instead of dropping + re-inserting it (a remount).
  function adoptTail(incoming: DemoTurn[]) {
    const incomingIds = new Set(incoming.map(turn => turn.id))
    const existingKeys = new Set(messages.map(turn => turn.serverId ?? turn.id))
    let ei = messages.length - 1
    let ii = incoming.length - 1
    while (ei >= 0 && ii >= 0) {
      const existing = messages[ei]
      const candidate = incoming[ii]
      if (!existing || !candidate) break
      if (incomingIds.has(existing.serverId ?? existing.id)) break
      if (existingKeys.has(candidate.id)) break
      if (existing.role !== candidate.role) break
      existing.serverId = candidate.id
      ei -= 1
      ii -= 1
    }
  }

  function doRefetch() {
    if (!messages.length) return
    stats.refetches++
    const serverTurns = messages.map(serverize)
    if (mode.value === 'new') {
      adoptTail(serverTurns)
      reconcileById(messages, serverTurns, {
        keyOfExisting: turn => turn.serverId ?? turn.id,
        merge: mergeTurnInPlace,
      })
    } else {
      // Pre-refactor setMessages: wholesale replace. The just-sent optimistic
      // turn's id flips client -> server, so its v-for key changes -> remount.
      messages.splice(0, messages.length, ...serverTurns)
    }
  }

  function startSend() {
    cycle += 1
    assistant = { id: `opt-a${cycle}`, role: 'assistant', blocks: [], ts: stamp() }
    messages.push(
      {
        id: `opt-u${cycle}`,
        role: 'user',
        text: cycle === 1 ? 'Run the build and watch the logs.' : `Re-run the build (#${cycle}).`,
        ts: stamp(),
      },
      assistant,
    )
  }

  function tick() {
    if (phase === 'idle') {
      startSend()
      phase = 'thinking'
      prog = 0
      return
    }
    if (!assistant) return

    if (phase === 'thinking') {
      prog += 6
      putBlock(assistant, { id: 0, kind: 'thinking', content: THINKING.slice(0, prog) })
      if (prog >= THINKING.length) {
        phase = 'intro'
        prog = 0
      }
      return
    }
    if (phase === 'intro') {
      prog += 4
      putBlock(assistant, { id: 1, kind: 'text', content: TEXT_INTRO.slice(0, prog) })
      if (prog >= TEXT_INTRO.length) {
        putBlock(assistant, { id: 2, kind: 'tool', toolName: 'exec', status: 'running', output: '' })
        phase = 'tool'
        prog = 0
      }
      return
    }
    if (phase === 'tool') {
      putBlock(assistant, {
        id: 2,
        kind: 'tool',
        toolName: 'exec',
        status: 'running',
        output: LOG_LINES.slice(0, prog + 1).join('\n'),
      })
      // A server refetch lands WHILE the background task is still running: the
      // pre-refactor path remounts the turn, flashing the popup mid-run.
      if (prog === 2) doRefetch()
      prog += 1
      if (prog >= LOG_LINES.length) {
        phase = 'summary'
        prog = 0
      }
      return
    }
    // phase === 'summary'
    prog += 4
    putBlock(assistant, { id: 3, kind: 'text', content: TEXT_SUMMARY.slice(0, prog) })
    if (prog >= TEXT_SUMMARY.length) {
      putBlock(assistant, {
        id: 2,
        kind: 'tool',
        toolName: 'exec',
        status: 'completed',
        output: LOG_LINES.join('\n'),
      })
      doRefetch()
      stats.cyclesCompleted += 1
      phase = 'idle'
    }
  }

  function fireBackgroundEvent() {
    const serverTime = new Date(Date.parse('2026-06-08T11:00:00Z') + sidebarStats.reorders * 60000 + clock++ * 1000).toISOString()
    if (mode.value === 'new') {
      const target = sessions.find(session => session.id === 's2')
      if (target && (!target.updated_at || serverTime > target.updated_at)) target.updated_at = serverTime
    } else {
      const index = sessions.findIndex(session => session.id === 's2')
      if (index >= 0) {
        const [target] = sessions.splice(index, 1)
        if (target) {
          target.updated_at = new Date().toISOString()
          sessions.unshift(target)
          sidebarStats.reorders += 1
        }
      }
    }
  }

  function reset() {
    messages.splice(0, messages.length)
    sessions.splice(0, sessions.length,
      { id: 's1', title: 'Chat · onboarding flow', updated_at: '2026-06-08T10:00:00Z' },
      { id: 's2', title: 'Background task · nightly build', updated_at: '2026-06-08T09:00:00Z' },
      { id: 's3', title: 'Chat · bug triage', updated_at: '2026-06-08T08:00:00Z' },
    )
    stats.refetches = 0
    stats.blockChurn = 0
    stats.cyclesCompleted = 0
    sidebarStats.reorders = 0
    clock = 0
    cycle = 0
    assistant = null
    phase = 'idle'
    prog = 0
  }

  return { mode, messages, stats, sessions, sidebarStats, orderedSessions, tick, fireBackgroundEvent, reset }
}
